package supersession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// Mutation-lane subjects the lifecycle pass writes through. Hardcoded here
// (matching graph.query.* / graph.mutation.* elsewhere in this codebase)
// rather than importing semstreams/processor/graph-ingest, which no other
// semsource package depends on.
const (
	subjectTripleAddBatch          = "graph.mutation.triple.add_batch"
	subjectEntityUpdateWithTriples = "graph.mutation.entity.update_with_triples"
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
// its own — every caller (ast-source, doc-source, source-manifest's
// remove_source) states org+systems (and, when a filesystem check applies, a
// root path) explicitly in a graph.LifecycleRunRequest.

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
// converges each entity's entity.lifecycle.stale marker against reality: with
// RootPath set, entities whose path predicate resolves to a now-missing file
// get marked and entities whose file has reappeared get cleared; with
// RootPath empty (the remove_source shape) every in-scope entity is
// unconditionally marked — there is no filesystem left to check. Passes are
// serialized against the correspondence pass via runMu. Read-then-write-only-
// the-delta, so re-running converges rather than duplicating markers.
func (c *Component) runLifecyclePass(ctx context.Context, req graph.LifecycleRunRequest) (graph.LifecycleRunResponse, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	if req.Org == "" || len(req.Systems) == 0 {
		return graph.LifecycleRunResponse{}, fmt.Errorf("lifecycle run: org and systems are required")
	}
	if req.Reason == "" {
		return graph.LifecycleRunResponse{}, fmt.Errorf("lifecycle run: reason is required")
	}

	c.mu.RLock()
	q := c.queryClient
	client := c.client
	c.mu.RUnlock()
	if q == nil || client == nil {
		return graph.LifecycleRunResponse{}, fmt.Errorf("supersession not started")
	}

	prefix := req.Org + "." + entityid.PlatformSemsource
	entities, _, err := q.QueryPrefixAll(ctx, gtypes.PrefixQueryRequest{Prefix: prefix}, c.config.maxEntities())
	if err != nil {
		return graph.LifecycleRunResponse{}, fmt.Errorf("enumerate entities: %w", err)
	}

	systemSet := make(map[string]struct{}, len(req.Systems))
	for _, s := range req.Systems {
		systemSet[s] = struct{}{}
	}
	var inScope []gtypes.EntityState
	for i := range entities {
		if _, ok := systemSet[entityIDSystem(entities[i].ID)]; ok {
			inScope = append(inScope, entities[i])
		}
	}

	var statFn func(path string) bool
	if req.RootPath != "" {
		statFn = func(path string) bool {
			_, err := os.Stat(filepath.Join(req.RootPath, path))
			return err == nil
		}
	}
	toMark, toClear, pathCount := decideLifecycleActions(inScope, req.RootPath, req.Reason, statFn)

	resp := graph.LifecycleRunResponse{Entities: len(inScope), Paths: pathCount}

	if len(toMark) > 0 {
		if err := markStale(ctx, client, toMark); err != nil {
			c.logger.Warn("lifecycle pass: mark batch failed", "count", len(toMark), "error", err)
		} else {
			resp.Marked = len(toMark)
		}
	}
	for _, id := range toClear {
		if err := clearStale(ctx, client, id); err != nil {
			c.logger.Warn("lifecycle pass: clear failed", "id", id, "error", err)
			continue
		}
		resp.Cleared++
	}

	c.logger.Info("lifecycle pass complete",
		"org", req.Org, "systems", req.Systems, "reason", req.Reason, "root_path", req.RootPath,
		"entities", resp.Entities, "paths", resp.Paths, "marked", resp.Marked, "cleared", resp.Cleared)
	return resp, nil
}

// decideLifecycleActions is the pure diff/converge core, isolated from all
// NATS/filesystem I/O so it is unit-testable with fakes. stat is injected
// (nil when rootPath is empty) and reports whether the given entity-relative
// path is present on disk. Idempotent: an already-marked entity whose file is
// still missing produces no action, and an unmarked entity whose file is
// present produces no action — only the boundary crossings do.
func decideLifecycleActions(inScope []gtypes.EntityState, rootPath, reason string, stat func(path string) bool) (toMark []message.Triple, toClear []string, pathCount int) {
	if rootPath == "" {
		// No filesystem to check — the source itself is gone, so every
		// in-scope entity is unconditionally stale.
		for i := range inScope {
			if isMarkedStale(inScope[i].Triples) {
				continue
			}
			toMark = append(toMark, staleTriple(inScope[i].ID, reason))
		}
		return toMark, toClear, 0
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
		liveCount, haveCount := chunkCountOf(group)
		for i := range group {
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

// chunkCountOf finds the parent document in a path group and returns its
// current passage count. Reports false when no parent carries the predicate —
// a code entity, or a doc ingested before passages existed — in which case no
// passage judgement can be made and the group is left alone.
func chunkCountOf(group []gtypes.EntityState) (int, bool) {
	for i := range group {
		if n, ok := tripleInt(group[i].Triples, source.DocChunkCount); ok {
			return n, true
		}
	}
	return 0, false
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
	return parts[3]
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

// pathOf returns an entity's backing artifact path, checked across both the
// code and doc path predicates (an entity carries at most one).
func pathOf(triples []message.Triple) (string, bool) {
	if p := tripleString(triples, semsourceast.CodePath); p != "" {
		return p, true
	}
	if p := tripleString(triples, source.DocFilePath); p != "" {
		return p, true
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

// markStale batches every triple in one graph.mutation.triple.add_batch
// request. AddTriples is must-exist (ADR-055) and appends (not
// replace-by-predicate), which is why callers only include entities not
// already carrying the marker (decideLifecycleActions) — the pass's own
// idempotency guard, since a raw append would otherwise duplicate the
// triple on every re-run against an unchanged-missing file.
func markStale(ctx context.Context, client *natsclient.Client, triples []message.Triple) error {
	req := gtypes.AddTriplesBatchRequest{Triples: triples}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal add_triples_batch request: %w", err)
	}
	reply, err := client.RequestClassified(ctx, subjectTripleAddBatch, data, lifecycleMutationTimeout)
	if err != nil {
		return err
	}
	var resp gtypes.AddTriplesBatchResponse
	if err := json.Unmarshal(reply, &resp); err != nil {
		return fmt.Errorf("decode add_triples_batch response: %w", err)
	}
	if len(resp.FailedSubjects) > 0 {
		return fmt.Errorf("partial batch failure: %v", resp.FailedSubjects)
	}
	return nil
}

// clearStale removes the entity.lifecycle.stale predicate from one entity via
// the update lane's RemoveTriples (a pure per-predicate delete — verified
// against the substrate pre-design; see design.md). Unknown/absent predicates
// are a silent no-op on the substrate side, so this is safe to call
// unconditionally on any entity the caller believes is marked.
func clearStale(ctx context.Context, client *natsclient.Client, id string) error {
	req := gtypes.UpdateEntityWithTriplesRequest{
		Entity:        &gtypes.EntityState{ID: id},
		RemoveTriples: []string{source.EntityLifecycleStale},
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal update_with_triples request: %w", err)
	}
	_, err = client.RequestClassified(ctx, subjectEntityUpdateWithTriples, data, lifecycleMutationTimeout)
	return err
}
