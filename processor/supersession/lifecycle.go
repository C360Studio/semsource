package supersession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"errors"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// lifecycleMutationTimeout bounds each mutation-lane round trip. Generous:
// the pass runs off a background trigger, never a caller's synchronous
// request path.
const lifecycleMutationTimeout = 15 * time.Second

// lifecycleEdgeSource tags the emitted marker triple's provenance.
const lifecycleEdgeSource = "lifecycle"

// This file implements the lifecycle pass (entity-staleness spec, design
// D2/D4): it converges the entity.lifecycle.stale marker against reality for
// a caller-announced scope. Housed alongside the correspondence/supersession
// pass because it reuses the same enumeration machinery (QueryPrefixAll) and
// trigger shape (NATS subject, request/reply). The pass never infers scope on
// its own — ordinary file/passage callers state org+systems and, for a filesystem check, a
// root path explicitly in a graph.LifecycleRunRequest. Whole-source removal is unavailable.

// handleLifecycleRun is the NATS request handler for graph.LifecycleTriggerSubject.
func (c *Component) handleLifecycleRun(ctx context.Context, data []byte) ([]byte, error) {
	var req graph.LifecycleRunRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("decode lifecycle run request: %w", err)
	}
	resp, err := c.runLifecyclePass(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

// runLifecyclePass enumerates every entity in req.Org+req.Systems' scope and
// converges each entity's entity.lifecycle.stale marker against reality. How
// reality is determined depends on which liveness oracle the request carries —
// a filesystem root, an explicit absent set, or neither, in which case every
// in-scope entity is unconditionally marked. See LifecycleRunRequest. Passes are
// serialized against the correspondence pass via runGate. Read-then-write-only-
// the-delta, so re-running converges rather than duplicating markers.
func (c *Component) runLifecyclePass(ctx context.Context, req graph.LifecycleRunRequest) (graph.LifecycleRunResponse, error) {
	if req.Reason == graph.LifecycleReasonSourceRemoved {
		return graph.LifecycleRunResponse{}, fmt.Errorf("SOURCE_LIFECYCLE_UNAVAILABLE: source lifecycle projection is unavailable")
	}
	release, err := c.runGate.acquire(ctx)
	if err != nil {
		return graph.LifecycleRunResponse{}, err
	}
	defer release()

	if req.Org == "" || len(req.Systems) == 0 {
		return graph.LifecycleRunResponse{}, fmt.Errorf("lifecycle run: org and systems are required")
	}
	if req.Reason == "" {
		return graph.LifecycleRunResponse{}, fmt.Errorf("lifecycle run: reason is required")
	}

	c.mu.RLock()
	q := c.queryClient
	mut := c.mutClient
	c.mu.RUnlock()
	if q == nil || mut == nil {
		return graph.LifecycleRunResponse{}, fmt.Errorf("supersession not started")
	}

	if req.Org != c.authority.Org {
		return graph.LifecycleRunResponse{}, fmt.Errorf("lifecycle request org %q differs from deployment authority", req.Org)
	}

	// A source's own republish can land between a mutation's authoritative
	// read and graph-ingest's compare, refusing the write with
	// revision_mismatch (#424). Only that refusal re-runs the pass, which
	// re-reads current state, and only lifecycleAttempts times; the gate stays
	// held across attempts (no attempt acquires it, so holding cannot deadlock,
	// and no other pass interleaves). Every other failure returns at once.
	var total graph.LifecycleRunResponse
	for attempt := 1; ; attempt++ {
		resp, conflictOnly, err := c.lifecycleAttempt(ctx, q, mut, req)
		total.Entities, total.Paths = resp.Entities, resp.Paths
		total.Marked += resp.Marked
		total.Cleared += resp.Cleared
		if err == nil || !conflictOnly || attempt == lifecycleAttempts || ctx.Err() != nil {
			return total, err
		}
		c.logger.Info("lifecycle pass hit revision_mismatch; re-running",
			"systems", req.Systems, "root_path", req.RootPath, "attempt", attempt, "error", err)
	}
}

// lifecycleAttempts bounds the passes one request runs when every failure is
// revision_mismatch (owner ruling on #424).
const lifecycleAttempts = 3

// lifecycleAttempt is one read-decide-write pass. conflictOnly reports that
// every failure it returns is a revision_mismatch refusal, which wrote nothing.
func (c *Component) lifecycleAttempt(ctx context.Context, q *prefixQuerier, mut lifecycleMutator, req graph.LifecycleRunRequest) (resp graph.LifecycleRunResponse, conflictOnly bool, err error) {
	prefix := c.authority.Org + "." + c.authority.Platform
	entities, truncated, err := q.queryPrefixAll(ctx, prefix, c.config.maxEntities())
	if err != nil {
		return graph.LifecycleRunResponse{}, false, fmt.Errorf("enumerate entities: %w", err)
	}

	if truncated {
		return graph.LifecycleRunResponse{}, false, fmt.Errorf("lifecycle enumeration truncated at max_entities")
	}
	systemSet := make(map[string]struct{}, len(req.Systems))
	for _, s := range req.Systems {
		systemSet[s] = struct{}{}
	}
	statFn, err := livenessOracle(req)
	if err != nil {
		return graph.LifecycleRunResponse{}, false, err
	}
	inScope := lifecycleScope(entities, systemSet, req.Owner, statFn != nil)
	toMark, toClear, pathCount := decideLifecycleActions(inScope, req.Reason, statFn)

	resp = graph.LifecycleRunResponse{Entities: len(inScope), Paths: pathCount}
	var failures error
	conflictOnly = true

	for _, tr := range toMark {
		if err := markStale(ctx, mut, tr); err != nil {
			failures = errors.Join(failures, err)
			conflictOnly = conflictOnly && isRevisionMismatch(err)
			continue
		}
		resp.Marked++
	}
	for _, id := range toClear {
		if err := clearStale(ctx, mut, id); err != nil {
			failures = errors.Join(failures, err)
			conflictOnly = conflictOnly && isRevisionMismatch(err)
			continue
		}
		resp.Cleared++
	}

	c.logger.Info("lifecycle pass complete",
		"org", req.Org, "systems", req.Systems, "reason", req.Reason, "root_path", req.RootPath,
		"entities", resp.Entities, "paths", resp.Paths, "marked", resp.Marked, "cleared", resp.Cleared)
	return resp, conflictOnly, failures
}

// isRevisionMismatch reports graph-ingest's classified revision_mismatch
// refusal, which the mutation client carries as MutationError.Code.
func isRevisionMismatch(err error) bool {
	var me *projection.MutationError
	return errors.As(err, &me) && me.Code == gtypes.ErrorCodeRevisionMismatch
}

// lifecycleScope selects the entities a pass judges: those in the request's
// systems and, when the request carries a liveness oracle, whose owner fact
// names the request's owner. A system slug is shared by every root configured
// with the same project, so the system alone would let one root's sweep judge
// another root's artifacts (#286). An entity with no owner fact is never
// judged by an oracle sweep: no root has shown it is that root's to judge.
func lifecycleScope(entities []gtypes.EntityState, systems map[string]struct{}, owner string, hasOracle bool) []gtypes.EntityState {
	var inScope []gtypes.EntityState
	for i := range entities {
		if _, ok := systems[entityIDSystem(entities[i].ID)]; !ok {
			continue
		}
		if hasOracle && tripleString(entities[i].Triples, source.EntityLifecycleOwner) != owner {
			continue
		}
		inScope = append(inScope, entities[i])
	}
	return inScope
}

// livenessOracle turns a request into the function that answers "is the
// artifact at this path still there?", or nil when the request carries no way
// to tell.
//
// RootPath and Absent are two different oracles and a request carrying both is
// rejected rather than resolved by precedence: whichever one lost would go on
// disagreeing silently, and the visible symptom is entities that will not
// retract or will not come back.
func livenessOracle(req graph.LifecycleRunRequest) (func(path string) bool, error) {
	if req.RootPath != "" && req.Absent != nil {
		return nil, fmt.Errorf("lifecycle run: root_path and absent are mutually exclusive")
	}
	if (req.RootPath != "" || req.Absent != nil) && req.Owner == "" {
		return nil, fmt.Errorf("lifecycle run: owner is required with root_path or absent")
	}

	if req.RootPath != "" {
		return func(path string) bool {
			// Code and doc paths are root-relative; a config file's path fact
			// is absolute (#393), and joining it to the root would never stat.
			if !filepath.IsAbs(path) {
				path = filepath.Join(req.RootPath, path)
			}
			_, err := os.Stat(path)
			return err == nil
		}, nil
	}

	if req.Absent != nil {
		// The caller enumerated its own source and completed; anything it did
		// not name is present. A path it never mentions is therefore live,
		// which is what lets an artifact that came back clear its marker on
		// this same pass rather than waiting for a re-seed.
		absent := make(map[string]struct{}, len(req.Absent))
		for _, path := range req.Absent {
			absent[path] = struct{}{}
		}
		return func(path string) bool {
			_, gone := absent[path]
			return !gone
		}, nil
	}

	return nil, nil
}

// decideLifecycleActions is the pure diff/converge core, isolated from all
// NATS/filesystem I/O so it is unit-testable with fakes. stat is injected and
// reports whether the given entity-relative path still has its artifact; it is
// nil when the request carried no way to tell. Idempotent: an already-marked
// entity whose artifact is still missing produces no action, and an unmarked
// entity whose artifact is present produces no action — only the boundary
// crossings do.
func decideLifecycleActions(inScope []gtypes.EntityState, reason string, stat func(path string) bool) (toMark []message.Triple, toClear []string, pathCount int) {
	if stat == nil {
		// Nothing to check liveness against — the source itself is gone, so
		// every in-scope entity is unconditionally stale.
		for i := range inScope {
			if isMarkedStale(inScope[i].Triples) {
				continue
			}
			toMark = append(toMark, staleTriple(inScope[i].ID, reason))
		}
		return toMark, toClear, 0
	}

	parentCounts := make(map[string]int)
	for _, entity := range inScope {
		if n, ok := tripleInt(entity.Triples, source.DocChunkCount); ok && n >= 0 {
			parentCounts[entity.ID] = n
		}
	}
	byPath := make(map[string][]gtypes.EntityState)
	for i := range inScope {
		p, ok := pathOf(inScope[i].Triples)
		if !ok {
			continue // no path predicate — can't determine liveness
		}
		byPath[p] = append(byPath[p], inScope[i])
	}
	pathCount = len(byPath)

	for path, group := range byPath {
		present := stat(path)
		if !present {
			for i := range group {
				if !isMarkedStale(group[i].Triples) {
					toMark = append(toMark, staleTriple(group[i].ID, reason))
				}
			}
			continue
		}
		// The file is present, but a document that SHRANK leaves passages
		// behind that no filesystem check can see: every passage of a
		// ten-passage document that is now seven passages long still carries
		// the path of a file that is very much still there. The parent's
		// DocChunkCount is the only evidence, so liveness for a passage is
		// index < count rather than stat().
		for i := range group {
			if isSourceRemoved(group[i].Triples) {
				continue
			}
			liveCount, haveCount := passageParentCount(group[i], parentCounts)
			marked := isMarkedStale(group[i].Triples)
			vanished := haveCount && isVanishedPassage(group[i].Triples, liveCount)
			switch {
			case vanished && !marked:
				toMark = append(toMark, staleTriple(group[i].ID, graph.LifecycleReasonPassageRemoved))
			case !vanished && marked:
				// Without the parent's count there is no evidence either way, so
				// clearing a passage's marker here would resurrect a genuinely
				// vanished passage whenever enumeration truncated before its
				// parent — and re-mark it on the next full pass. Absence of
				// evidence must not read as evidence of liveness.
				if !haveCount && isPassage(group[i].Triples) {
					continue
				}
				toClear = append(toClear, group[i].ID)
			}
		}
	}
	return toMark, toClear, pathCount
}

// passageParentCount uses the producer's exact typed parent edge. Sharing a path
// does not grant another source or revision authority over a passage's liveness.
func passageParentCount(entity gtypes.EntityState, parentCounts map[string]int) (int, bool) {
	parent := ""
	for _, tr := range entity.Triples {
		if tr.Predicate == source.CodeBelongs && tr.Datatype == message.EntityReferenceDatatype {
			if value, ok := tr.Object.(string); ok {
				if parent != "" && parent != value {
					return 0, false
				}
				parent = value
			}
		}
	}
	if parent == "" {
		return 0, false
	}
	count, ok := parentCounts[parent]
	return count, ok
}

func isSourceRemoved(triples []message.Triple) bool {
	for _, tr := range triples {
		if tr.Predicate == source.EntityLifecycleStale && lifecycleReason(tr.Object) == graph.LifecycleReasonSourceRemoved {
			return true
		}
	}
	return false
}

// isPassage reports whether triples describe a passage rather than a parent
// document or a code entity, identified by carrying a chunk index at all.
func isPassage(triples []message.Triple) bool {
	_, ok := tripleInt(triples, source.DocChunkIndex)
	return ok
}

// isVanishedPassage reports whether triples describe a passage whose ordinal is
// at or beyond its parent's current passage count — i.e. a passage the document
// no longer has. An entity with no chunk index is not a passage and is never
// vanished by this rule.
func isVanishedPassage(triples []message.Triple, liveCount int) bool {
	idx, ok := tripleInt(triples, source.DocChunkIndex)
	return ok && idx >= liveCount
}

// tripleInt reads a numeric triple object. The producer emits a Go int, but
// these entities arrive back through a JSON query round trip, where that int is
// a float64 — so both must be accepted or every passage judgement silently
// fails closed and no phantom is ever marked.
func tripleInt(triples []message.Triple, predicate string) (int, bool) {
	for i := range triples {
		if triples[i].Predicate != predicate {
			continue
		}
		switch v := triples[i].Object.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		case json.Number:
			n, err := v.Int64()
			return int(n), err == nil
		}
		return 0, false
	}
	return 0, false
}

// entityIDSystem extracts the system segment (index 3) from a 6-part entity
// ID: org.platform.domain.system.type.instance. Returns "" for a malformed ID.
func entityIDSystem(id string) string {
	parts := strings.SplitN(id, ".", 6)
	if len(parts) < 6 {
		return ""
	}
	return parts[2]
}

// isMarkedStale reports whether triples already carries the staleness marker.
func isMarkedStale(triples []message.Triple) bool {
	for i := range triples {
		if triples[i].Predicate == source.EntityLifecycleStale {
			return true
		}
	}
	return false
}

// pathOf returns an entity's backing artifact path, checked across the code,
// doc and config-file path predicates (an entity carries at most one).
func pathOf(triples []message.Triple) (string, bool) {
	for _, pred := range []string{semsourceast.CodePath, source.DocFilePath, source.ConfigFilePath} {
		if p := tripleString(triples, pred); p != "" {
			return p, true
		}
	}
	return "", false
}

// staleTriple builds the entity.lifecycle.stale marker triple.
func staleTriple(subject, reason string) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  source.EntityLifecycleStale,
		Object:     reason,
		Source:     lifecycleEdgeSource,
		Timestamp:  time.Now(),
		Confidence: 1.0,
	}
}

// markStale reconciles one entity's lifecycle predicate group to exactly the
// marker triple, through the typed CAS mutation client. Reconcile (not append)
// makes re-marking idempotent by construction — the old add_batch path needed
// decideLifecycleActions' dedup guard to avoid duplicating markers; this does
// not, though the guard still spares round trips. One transport attempt; the
// framework never retries a mutation for the caller.
func markStale(ctx context.Context, mut lifecycleMutator, marker message.Triple) error {
	_, err := reconcileStale(ctx, mut, marker, "")
	return err
}

func reconcileStale(ctx context.Context, mut lifecycleMutator, marker message.Triple, requestID string) (projection.MutationReceipt, error) {
	receipt, err := mut.Reconcile(ctx, projection.ReconcileMutation{
		Contract: graph.SourceEntityContract().Name,
		Group:    graph.GroupLifecycle,
		EntityID: marker.Subject,
		Desired:  []message.Triple{marker},
		Metadata: projection.MutationMetadata{
			RequestID: requestID,
			Source:    lifecycleEdgeSource,
			Timestamp: marker.Timestamp,
		},
	})
	if err != nil {
		return receipt, fmt.Errorf("reconcile lifecycle marker on %s: %w", marker.Subject, err)
	}
	return receipt, nil
}

// clearStale reconciles one entity's lifecycle predicate group to empty. An
// entity_not_found outcome is benign here — an entity that no longer exists
// has nothing to clear — and is skipped without error, mirroring the old
// update lane's silent no-op on absent predicates. Every other failure
// surfaces distinctly and nothing here is retried. The one bounded exception
// is a revision_mismatch refusal, on which runLifecyclePass re-runs the whole
// pass against re-read state (owner ruling on #424), never this call alone.
func clearStale(ctx context.Context, mut lifecycleMutator, id string) error {
	_, err := mut.Reconcile(ctx, projection.ReconcileMutation{
		Contract: graph.SourceEntityContract().Name,
		Group:    graph.GroupLifecycle,
		EntityID: id,
		Desired:  nil,
		Metadata: projection.MutationMetadata{
			Source:    lifecycleEdgeSource,
			Timestamp: time.Now(),
		},
	})
	if err != nil {
		var ce *errs.ClassifiedError
		if errors.As(err, &ce) && ce.Code == gtypes.ErrorCodeEntityNotFound {
			return nil
		}
		return fmt.Errorf("reconcile lifecycle clear on %s: %w", id, err)
	}
	return nil
}

func lifecycleReason(value any) string { reason, _ := value.(string); return reason }
