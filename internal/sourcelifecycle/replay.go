package sourcelifecycle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/c360studio/semsource/internal/sourceintent"
)

// ReconcileOnce scans all durable current intents, acquiring the common gate for
// one bounded replay unit at a time. A failed unit remains eligible next scan.
func (c *Coordinator) ReconcileOnce(ctx context.Context) error {
	records, err := c.journal.List(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, record := range records {
		if (record.Phase == sourceintent.Complete && record.Operation != sourceintent.Reactivate) || record.Phase == sourceintent.Superseded {
			continue
		}
		unitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		release, err := c.Acquire(unitCtx)
		if err == nil {
			err = c.reconcile(unitCtx, record.Binding.Handle)
			release()
		}
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	return errors.Join(failures...)
}
func (c *Coordinator) reconcile(ctx context.Context, handle string) error {
	r, rev, err := c.journal.Get(ctx, handle)
	if err != nil {
		return err
	}
	if (r.Phase == sourceintent.Complete && r.Operation != sourceintent.Reactivate) || r.Phase == sourceintent.Superseded {
		return nil
	}
	cc, exists, err := c.desired(ctx, handle)
	if err != nil {
		return c.recordOutcome(ctx, r, rev, sourceintent.ProjectionResult{}, err)
	}
	if !exists {
		return c.recordOutcome(ctx, r, rev, sourceintent.ProjectionResult{}, blocker(sourceintent.CodeDesiredAmbiguous, "desired envelope missing"))
	}
	if r.Operation == sourceintent.Remove && cc.Enabled {
		if r.Phase != sourceintent.Prepared || !sameConfig(cc, r.RetiredConfig) {
			return c.RecordReadd(ctx, handle)
		}
		return c.recordOutcome(ctx, r, rev, sourceintent.ProjectionResult{}, blocker(sourceintent.CodeDesiredAmbiguous, "prepared removal has not committed disable"))
	}
	if r.Operation == sourceintent.Remove && r.Phase == sourceintent.Prepared {
		if err := c.PersistRemoval(ctx, r); err != nil {
			return err
		}
		r, rev, err = c.journal.Get(ctx, handle)
		if err != nil {
			return err
		}
	}
	var result sourceintent.ProjectionResult
	if r.Operation == sourceintent.Remove {
		result, err = c.replayRemoval(ctx, r)
	} else {
		result, err = c.replayReactivation(ctx, r)
	}
	return c.recordOutcome(ctx, r, rev, result, err)
}
func (c *Coordinator) replayRemoval(ctx context.Context, r sourceintent.Record) (sourceintent.ProjectionResult, error) {
	var zero sourceintent.ProjectionResult
	if err := c.CheckAdmission(ctx); err != nil {
		return zero, err
	}
	if cc, ok := c.boot[r.Binding.Handle]; ok && cc.Enabled {
		return zero, blocker(sourceintent.CodeRetirement, "requesting boot still admits producer; application replacement required")
	}
	cc, exists, err := c.desired(ctx, r.Binding.Handle)
	if err != nil {
		return zero, err
	}
	if !exists || cc.Enabled || !sameConfig(cc, r.RetiredConfig) {
		return zero, blocker(sourceintent.CodeDesiredAmbiguous, "removal no longer matches disabled desired source")
	}
	if err := CheckExclusive(r.Scope, c.boot); err != nil {
		return zero, err
	}
	if err := CheckExclusive(r.Scope, c.store.GetConfig().Get().Components); err != nil {
		return zero, err
	}
	tail, err := c.tail.Settled(ctx, r.Scope)
	if err != nil {
		return zero, err
	}
	if err := sameTail(r.Tail, tail); err != nil {
		return zero, err
	}
	if tail.Proof != sourceintent.TailProofCurrentRetained && tail.Proof != sourceintent.TailProofAppliedComplete {
		return zero, blocker(sourceintent.CodeTailUnavailable, "tail observer returned no admitted proof")
	}
	result, err := c.projector.ApplyRemoval(ctx, sourceintent.RemovalProjection{Binding: r.Binding, Scope: r.Scope, Effects: c.effectFence(r.Binding)})
	if err != nil {
		return result, err
	}
	if !result.Complete {
		return result, blocker(sourceintent.CodeEnumeration, "retained projection returned incomplete without error")
	}
	if tail.Proof == sourceintent.TailProofCurrentRetained {
		return result, blocker(sourceintent.CodeAppliedTailUnproven, "retained scope converged; pinned graph-ingest cannot prove every accepted input applied (semstreams#1444)")
	}
	return result, nil
}
func sameTail(old, current sourceintent.TailEvidence) error {
	if len(old.Inputs) == 0 || len(old.Inputs) != len(current.Inputs) {
		return blocker(sourceintent.CodeTailChanged, "graph input set changed")
	}
	for i, want := range old.Inputs {
		got := current.Inputs[i]
		if !reflect.DeepEqual(want.Input, got.Input) || !want.StreamCreated.Equal(got.StreamCreated) || !want.ConsumerCreated.Equal(got.ConsumerCreated) || want.AckPolicy != got.AckPolicy || want.DeliverPolicy != got.DeliverPolicy || want.Storage != got.Storage || want.Replicas != got.Replicas {
			return blocker(sourceintent.CodeTailChanged, "captured stream or consumer identity/policy changed")
		}
	}
	return nil
}
func (c *Coordinator) replayReactivation(ctx context.Context, r sourceintent.Record) (sourceintent.ProjectionResult, error) {
	var zero sourceintent.ProjectionResult
	binding, admitted := c.bindings[r.Binding.Handle]
	if !admitted || binding != r.Binding {
		return zero, blocker(sourceintent.CodeRetirement, "reactivation requires current boot publication binding")
	}
	cc, exists, err := c.desired(ctx, r.Binding.Handle)
	if err != nil {
		return zero, err
	}
	if !exists || !cc.Enabled || !sameDigest(cc, r.Binding.ConfigDigest) {
		return zero, blocker(sourceintent.CodeDesiredAmbiguous, "reactivation desired source changed")
	}
	if r.Seed == nil || !r.Seed.Initial || !r.Seed.Successful {
		return zero, blocker(sourceintent.CodeSeedIncomplete, "current initial enumeration has not sealed successfully")
	}

	initial, err := c.journal.LoadSeed(ctx, r.Binding, r.Seed.BatchID)
	if err != nil {
		return zero, err
	}
	if !initial.Initial {
		return zero, blocker(sourceintent.CodeSeedIncomplete, "initial seal does not describe initial enumeration")
	}
	initialReceipts, err := c.journal.ListReceipts(ctx, r.Binding, initial.BatchID)
	if err != nil {
		return zero, err
	}
	if err := validateReceipts(initial, initialReceipts); err != nil {
		return zero, err
	}
	manifests, err := c.journal.ListSeeds(ctx, r.Binding)
	if err != nil {
		return zero, err
	}
	result := sourceintent.ProjectionResult{Complete: true}
	var failures []error
	for _, manifest := range manifests {
		receipts, err := c.journal.ListReceipts(ctx, r.Binding, manifest.BatchID)
		if err == nil {
			err = validateReceipts(manifest, receipts)
		}
		if err != nil {
			result.Complete = false
			failures = append(failures, err)
			continue
		}
		request := sourceintent.ReactivationProjection{Binding: r.Binding, Manifest: manifest, Receipts: receipts}
		if !manifest.Initial {
			request.InitialManifest = &initial
			request.InitialReceipts = initialReceipts
		}
		projected, err := c.projector.ApplyReactivation(ctx, request)
		result.Enumerated += projected.Enumerated
		result.Cleared += projected.Cleared
		result.Failures = append(result.Failures, projected.Failures...)
		result.Complete = result.Complete && projected.Complete && err == nil
		if err != nil {
			failures = append(failures, err)
		}
	}
	return result, errors.Join(failures...)
}

func validateReceipts(m sourceintent.SeedManifest, receipts []sourceintent.Receipt) error {
	if !m.Successful || len(m.Entries) != len(receipts) {
		return blocker(sourceintent.CodeSeedIncomplete, "sealed current seed and receipts differ")
	}
	seen := map[string]string{}
	for _, r := range receipts {
		if r.Binding != m.Binding || r.BatchID != m.BatchID || !r.Acknowledged {
			return blocker(sourceintent.CodeReceipt, "receipt binding/ack mismatch")
		}
		if _, ok := seen[r.EntityID]; ok {
			return blocker(sourceintent.CodeReceipt, "duplicate receipt")
		}
		seen[r.EntityID] = r.Fingerprint
	}
	for _, expected := range m.Entries {
		if got, ok := seen[expected.EntityID]; !ok || got != expected.Fingerprint {
			return blocker(sourceintent.CodeReceipt, "expected source receipt missing or changed")
		}
	}
	return nil
}
func (c *Coordinator) recordOutcome(ctx context.Context, r sourceintent.Record, _ uint64, result sourceintent.ProjectionResult, err error) error {
	current, rev, readErr := c.journal.Get(ctx, r.Binding.Handle)
	if readErr != nil {
		return errors.Join(err, readErr)
	}
	if current.Binding != r.Binding || current.Operation != r.Operation {
		return errors.Join(err, sourceintent.ErrConflict)
	}
	r = current
	if unresolvedEffect(r) {
		err = errors.Join(effectBlocked(), err)
	}
	r.UpdatedAt = time.Now()
	r.Progress.RetryCount++
	r.Progress.CompletedCount = result.Marked + result.Cleared
	// A complete removal pass verifies already-marked entities as well as new
	// mutations. Count that retained set, while preserving partial-pass receipts
	// when enumeration, mutation, or cancellation prevented convergence.
	retainedConverged := err == nil || asBlocker(err).Code == sourceintent.CodeAppliedTailUnproven
	if r.Operation == sourceintent.Remove && result.Complete && retainedConverged {
		r.Progress.CompletedCount = result.Enumerated
	}
	if result.Enumerated > 0 || result.Complete {
		r.Progress.LastProgress = r.UpdatedAt
	}
	if err != nil {
		r.Phase = sourceintent.Pending
		r.Progress.Blocker = asBlocker(err)
	} else if !result.Complete {
		r.Phase = sourceintent.Pending
		err = blocker(sourceintent.CodeEnumeration, "projection returned incomplete without error")
		r.Progress.Blocker = asBlocker(err)
	} else {
		r.Phase = sourceintent.Complete
		r.Progress.Blocker = nil
	}
	if _, writeErr := c.journal.Update(ctx, r, rev); writeErr != nil {
		return errors.Join(err, fmt.Errorf("persist replay progress: %w", writeErr))
	}
	return err
}
