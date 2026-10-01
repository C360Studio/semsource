package supersession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	source "github.com/c360studio/semsource/source/vocabulary"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/projection"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

// sourceProjectionTimeout bounds a synchronous replay unit. Unknown remote
// effects retain a durable fence; a deadline never proves their cancellation.
const sourceProjectionTimeout = 12 * time.Second

type lifecycleMutator interface {
	ReadAuthoritative(context.Context, string) (*gtypes.ExactEntity, error)
	Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error)
}

var _ sourceintent.Projector = (*Component)(nil)

// handleSourceProjection permanently refuses the old wire path. A NATS handler
// has an independent context and cannot borrow the caller's desired-state gate.
func (c *Component) handleSourceProjection(_ context.Context, _ []byte) ([]byte, error) {
	return json.Marshal(sourcelifecycle.ProjectionReply{Error: projectionBlocker(sourceintent.CodeOwnership,
		"source lifecycle projection requires the process-local authorized owner")})
}

// ApplyRemoval converges the complete currently retained selected set. Complete
// is deliberately not proof that all upstream accepted inputs have been applied;
// the coordinator retains that independent pending tail obligation.
func (c *Component) ApplyRemoval(ctx context.Context, request sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sourceProjectionTimeout)
	defer cancel()
	release, err := c.runGate.Acquire(ctx)
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	defer release()
	if err := c.validateRemoval(request); err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	c.mu.RLock()
	q, mut, ready := c.queryClient, c.mutClient, c.running && !c.stopping
	c.mu.RUnlock()
	if !ready || q == nil || mut == nil {
		return sourceintent.ProjectionResult{}, projectionBlocker(sourceintent.CodeMutation, "supersession not started")
	}
	if request.Effects == nil {
		return sourceintent.ProjectionResult{}, projectionBlocker(sourceintent.CodeEffectUnresolved, "source removal requires a durable effect fence")
	}
	entities, result, err := enumerateRemoval(ctx, q, request.Scope)
	if err != nil {
		return result, err
	}
	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		exact, err := mut.ReadAuthoritative(ctx, entity.ID)
		if err == nil && (exact == nil || exact.Entity == nil || exact.Entity.ID != entity.ID || exact.KVRevision == 0) {
			err = errors.New("invalid authoritative entity response")
		}
		if err == nil && !matchesAnySelector(*exact.Entity, request.Scope.Selectors) {
			err = errors.New("authoritative entity no longer matches exact selector")
		}
		if err == nil && exactSourceRemoved(exact.Entity.Triples) {
			continue
		}
		if err != nil {
			result.Failures = append(result.Failures, entityFailure(entity.ID, sourceintent.CodeMutation, err))
			continue
		}
		applied, effectErr := markSourceRemoved(ctx, mut, request, entity.ID)
		if applied {
			result.Marked++
		}
		if effectErr != nil {
			code := sourceintent.CodeMutation
			var blocked *sourceintent.Blocker
			if errors.As(effectErr, &blocked) {
				code = blocked.Code
			}
			result.Failures = append(result.Failures, entityFailure(entity.ID, code, effectErr))
			if code == sourceintent.CodeEffectUnresolved {
				return result, effectErr
			}
		}
	}
	if len(result.Failures) > 0 {
		return result, projectionBlocker(sourceintent.CodeMutation, "one or more retained entities could not be marked")
	}
	result.Complete = true
	return result, nil
}

func (c *Component) validateBinding(b sourceintent.Binding) error {
	if err := b.Authority.Validate(); err != nil {
		return projectionBlocker(sourceintent.CodeOwnership, err.Error())
	}
	if b.Authority != c.authority || b.Handle == "" || b.Namespace == "" || b.Generation == 0 || b.Factory == "" || b.ConfigDigest == "" {
		return projectionBlocker(sourceintent.CodeOwnership, "incomplete or foreign source binding")
	}
	return nil
}
func (c *Component) validateRemoval(r sourceintent.RemovalProjection) error {
	if err := c.validateBinding(r.Binding); err != nil {
		return err
	}
	s := r.Scope
	b := r.Binding
	if s.Authority != b.Authority || s.Handle != b.Handle || s.Factory != b.Factory || s.ConfigDigest != b.ConfigDigest || len(s.Selectors) == 0 {
		return projectionBlocker(sourceintent.CodeOwnership, "scope does not match source binding")
	}
	seen := map[string]bool{}
	for _, sel := range s.Selectors {
		prefix := b.Authority.Org + "." + b.Authority.Platform + "." + sel.System + "." + sel.Domain + "."
		if sel.System == "" || sel.Domain == "" || strings.ContainsAny(sel.System+sel.Domain, ".*>/\\") || sel.Prefix != prefix {
			return projectionBlocker(sourceintent.CodeUnsupportedScope, "selector is not an exact authority/system/taxonomy prefix")
		}
		if err := semtypes.ValidateEntityIDPrefix(strings.TrimSuffix(prefix, ".")); err != nil {
			return projectionBlocker(sourceintent.CodeUnsupportedScope, err.Error())
		}
		if seen[prefix] {
			return projectionBlocker(sourceintent.CodeAmbiguousScope, "duplicate selector")
		}
		seen[prefix] = true
		if (sel.ArtifactPredicate == "") != (len(sel.Artifacts) == 0) {
			return projectionBlocker(sourceintent.CodeUnsupportedScope, "artifact predicate and values must both be supplied")
		}
	}
	return nil
}
func enumerateRemoval(ctx context.Context, q *prefixQuerier, scope sourceintent.SourceScope) ([]gtypes.EntityState, sourceintent.ProjectionResult, error) {
	var all []gtypes.EntityState
	var result sourceintent.ProjectionResult
	for _, selector := range scope.Selectors {
		entities, truncated, err := q.queryPrefixAll(ctx, selector.Prefix, 0)
		for _, e := range entities {
			if matchesSelector(e, selector) {
				all = append(all, e)
			}
		}
		result.Enumerated = len(all)
		if err != nil {
			return nil, result, projectionBlocker(sourceintent.CodeEnumeration, err.Error())
		}
		if truncated {
			return nil, result, projectionBlocker(sourceintent.CodeEnumeration, "source enumeration unexpectedly truncated")
		}
	}
	return all, result, nil
}
func matchesAnySelector(e gtypes.EntityState, selectors []sourceintent.Selector) bool {
	for _, sel := range selectors {
		if matchesSelector(e, sel) {
			return true
		}
	}
	return false
}
func matchesSelector(e gtypes.EntityState, sel sourceintent.Selector) bool {
	if !strings.HasPrefix(e.ID, sel.Prefix) {
		return false
	}
	if len(sel.Artifacts) == 0 {
		return true
	}
	for _, tr := range e.Triples {
		if tr.Subject == e.ID && tr.Predicate == sel.ArtifactPredicate {
			value, ok := tr.Object.(string)
			if ok {
				for _, artifact := range sel.Artifacts {
					if value == artifact {
						return true
					}
				}
			}
		}
	}
	return false
}

// ApplyReactivation validates the current sealed epoch and source facts. The
// frozen client cannot condition Reconcile on this read's revision; until that
// public contract is admitted, source_removed remains pending rather than being
// cleared on an unfenced observation.
func (c *Component) ApplyReactivation(ctx context.Context, r sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sourceProjectionTimeout)
	defer cancel()
	release, err := c.runGate.Acquire(ctx)
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	defer release()
	if err := c.validateReactivation(r); err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	c.mu.RLock()
	mut, ready := c.mutClient, c.running && !c.stopping
	c.mu.RUnlock()
	if !ready || mut == nil {
		return sourceintent.ProjectionResult{}, projectionBlocker(sourceintent.CodeMutation, "supersession not started")
	}
	result := sourceintent.ProjectionResult{}
	for _, entry := range r.Manifest.Entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Enumerated++
		exact, err := mut.ReadAuthoritative(ctx, entry.EntityID)
		if err == nil && (exact == nil || exact.Entity == nil || exact.Entity.ID != entry.EntityID || exact.KVRevision == 0) {
			err = errors.New("invalid authoritative entity response")
		}
		if err != nil {
			result.Failures = append(result.Failures, entityFailure(entry.EntityID, sourceintent.CodeMutation, err))
			continue
		}
		if !isSourceRemoved(exact.Entity.Triples) {
			continue
		}
		fingerprint, err := sourceintent.RetainedFingerprint(entry.EntityID, exact.Entity.Triples, exact.Entity.StorageRef)
		if err != nil || fingerprint != entry.Fingerprint {
			result.Failures = append(result.Failures, entityFailure(entry.EntityID, sourceintent.CodeSourceFactsMismatch, errors.New("retained source facts do not match current publication")))
			continue
		}
		result.Failures = append(result.Failures, entityFailure(entry.EntityID, sourceintent.CodeConditionalReconcileUnavailable, errors.New("conditional source-fact revision reconciliation is not available at the frozen pin")))
	}
	if len(result.Failures) > 0 {
		return result, projectionBlocker(result.Failures[0].Failure.Code, "reactivation remains pending")
	}
	result.Complete = true
	return result, nil
}
func (c *Component) validateReactivation(r sourceintent.ReactivationProjection) error {
	if err := c.validateBinding(r.Binding); err != nil {
		return err
	}
	if err := c.validateSeedEvidence(r.Binding, r.Manifest, r.Receipts); err != nil {
		return err
	}
	if !r.Manifest.Initial {
		if r.InitialManifest == nil || !r.InitialManifest.Initial || r.InitialManifest.BatchID == r.Manifest.BatchID {
			return projectionBlocker(sourceintent.CodeSeedIncomplete, "live publication requires its separate completed current initial seed")
		}
		return c.validateSeedEvidence(r.Binding, *r.InitialManifest, r.InitialReceipts)
	}
	if r.InitialManifest != nil || len(r.InitialReceipts) > 0 {
		return projectionBlocker(sourceintent.CodeSeedIncomplete, "initial target must supply its proof as the target manifest")
	}
	return nil
}

func (c *Component) validateSeedEvidence(binding sourceintent.Binding, m sourceintent.SeedManifest, evidence []sourceintent.Receipt) error {
	if binding.BootEpoch == "" || m.Binding != binding || m.BatchID == "" || !m.Successful || m.Failure != nil {
		return projectionBlocker(sourceintent.CodeSeedIncomplete, "reactivation requires a completed current seed epoch")
	}
	digest, err := sourceintent.ManifestDigest(m.Entries)
	if err != nil || digest != m.Digest {
		return projectionBlocker(sourceintent.CodeSeedIncomplete, "current manifest digest is invalid")
	}
	receipts := map[string]sourceintent.Receipt{}
	for _, receipt := range evidence {
		if receipt.Binding != binding || receipt.BatchID != m.BatchID || !receipt.Acknowledged {
			return projectionBlocker(sourceintent.CodeReceipt, "receipt belongs to another epoch or is unacknowledged")
		}
		if _, ok := receipts[receipt.EntityID]; ok {
			return projectionBlocker(sourceintent.CodeReceipt, "duplicate publication receipt")
		}
		receipts[receipt.EntityID] = receipt
	}
	if len(receipts) != len(m.Entries) {
		return projectionBlocker(sourceintent.CodeReceipt, "receipt set differs from manifest")
	}
	prefix := c.authority.Org + "." + c.authority.Platform + "."
	for _, entry := range m.Entries {
		if err := semtypes.ValidateEntityID(entry.EntityID); err != nil || !strings.HasPrefix(entry.EntityID, prefix) {
			return projectionBlocker(sourceintent.CodeOwnership, "manifest contains foreign or invalid entity")
		}
		receipt, ok := receipts[entry.EntityID]
		if !ok || receipt.Fingerprint != entry.Fingerprint {
			return projectionBlocker(sourceintent.CodeReceipt, "receipt fingerprint differs from manifest")
		}
	}
	return nil
}
func projectionBlocker(code sourceintent.ErrorCode, message string) *sourceintent.Blocker {
	return &sourceintent.Blocker{Code: code, Message: message, Retryable: true}
}
func entityFailure(id string, code sourceintent.ErrorCode, err error) sourceintent.EntityFailure {
	return sourceintent.EntityFailure{EntityID: id, Failure: *projectionBlocker(code, err.Error())}
}

// A duplicate or conflicting retained marker still requires reconciliation.
func exactSourceRemoved(triples []message.Triple) bool {
	count := 0
	for _, tr := range triples {
		if tr.Predicate == source.EntityLifecycleStale {
			count++
			if value, ok := tr.Object.(string); !ok || value != graph.LifecycleReasonSourceRemoved {
				return false
			}
		}
	}
	return count == 1
}
