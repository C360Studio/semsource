package supersession

import (
	"context"
	"errors"
	"fmt"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/projection"
)

// markSourceRemoved never dispatches before the coordinator persists its exact
// attempt. A terminal journal failure still reports verified physical progress,
// but stops the pass until the owner reads authoritative resolution evidence.
func markSourceRemoved(ctx context.Context, mut lifecycleMutator, request sourceintent.RemovalProjection, id string) (bool, error) {
	attempt, err := request.Effects.Begin(ctx, id)
	if err != nil {
		return false, effectUnresolved("persist effect fence", err)
	}
	if attempt.ID == "" || attempt.Binding != request.Binding || attempt.EntityID != id || attempt.StartedAt.IsZero() || attempt.Outcome != sourceintent.EffectUnknown {
		return false, effectUnresolved("effect fence does not authorize this exact mutation", nil)
	}
	marker := staleTriple(id, graph.LifecycleReasonSourceRemoved)
	marker.Timestamp = attempt.StartedAt
	receipt, mutationErr := reconcileStale(ctx, mut, marker, attempt.ID)
	outcome := sourceRemovalOutcome(id, receipt, mutationErr)
	resolveErr := request.Effects.Resolve(ctx, attempt, outcome)
	applied := outcome == sourceintent.EffectVerified
	if resolveErr != nil {
		return applied, effectUnresolved("record terminal effect evidence", errors.Join(mutationErr, resolveErr))
	}
	if outcome == sourceintent.EffectUnknown {
		return false, effectUnresolved("graph mutation outcome remains unknown", mutationErr)
	}
	return applied, mutationErr
}

// sourceRemovalOutcome deliberately narrows the frozen public classifier. Its
// generic internal rejection can follow a KV update timeout (#1446), so nominal
// NotCommitted alone is insufficient to release a source-generation fence.
func sourceRemovalOutcome(id string, receipt projection.MutationReceipt, err error) sourceintent.EffectOutcome {
	if receipt.Commit == projection.CommitVerified && err == nil && receipt.Entity != nil && receipt.Entity.ID == id && receipt.KVRevision > 0 && exactSourceRemoved(receipt.Entity.Triples) {
		return sourceintent.EffectVerified
	}
	if receipt.Commit != projection.CommitNotCommitted || err == nil {
		return sourceintent.EffectUnknown
	}
	var mutation *projection.MutationError
	if !errors.As(err, &mutation) || mutation.Commit != projection.CommitNotCommitted {
		return sourceintent.EffectUnknown
	}
	switch mutation.Kind {
	case projection.MutationInvalid:
		if mutation.Code == "" || mutation.Code == gtypes.ErrorCodeInvalidRequest || mutation.Code == gtypes.ErrorCodeStructuralInvalid {
			return sourceintent.EffectNotCommitted
		}
	case projection.MutationNotFound:
		if mutation.Code == gtypes.ErrorCodeEntityNotFound {
			return sourceintent.EffectNotCommitted
		}
	case projection.MutationRevisionConflict:
		if mutation.Code == gtypes.ErrorCodeRevisionMismatch {
			return sourceintent.EffectNotCommitted
		}
	case projection.MutationUnavailable:
		if mutation.Code == "" && (natsclient.IsNoResponders(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return sourceintent.EffectNotCommitted
		}
	}
	return sourceintent.EffectUnknown
}

func effectUnresolved(message string, err error) *sourceintent.Blocker {
	if err != nil {
		message = fmt.Sprintf("%s: %v", message, err)
	}
	return projectionBlocker(sourceintent.CodeEffectUnresolved, message)
}
