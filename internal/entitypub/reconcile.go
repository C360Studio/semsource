package entitypub

import (
	"context"
	"errors"
	"sync"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"

	"github.com/c360studio/semsource/graph"
)

// envelopeSource is the source every handler publish is stamped with, both on
// the stream envelope and on the reconcile that follows it.
const envelopeSource = "semsource"

// Reconciler is the projection mutation-client surface the publisher needs.
type Reconciler interface {
	Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error)
}

// WithReconcile makes every successful stream publish followed by one
// reconcile of contract group `source`, so predicates the source stopped
// emitting are removed (#336). A handler passes it only once a test proves
// each of its publishes carries the entity's complete semsource statement set.
func WithReconcile(r Reconciler) Option {
	return func(c *publisherConfig) { c.reconciler = r }
}

// reconcileTimeout bounds one reconcile's read plus request/reply.
const reconcileTimeout = 15 * time.Second

// NewSourceReconciler returns the projection mutation client a handler passes
// to WithReconcile, bound to the source-entity contract.
func NewSourceReconciler(nc *natsclient.Client) (Reconciler, error) {
	return projection.NewMutationClient(projection.MutationClientConfig{
		NATS:      nc,
		Contracts: []projection.Contract{graph.SourceEntityContract()},
		Timeout:   reconcileTimeout,
	})
}

var (
	groupOnce  sync.Once
	groupPreds map[string]bool
)

// groupPredicates is graph.OwnedPredicates as a set, computed on first use:
// the vocabulary is registered at bootstrap, after package init.
func groupPredicates() map[string]bool {
	groupOnce.Do(func() {
		groupPreds = map[string]bool{}
		for _, p := range graph.OwnedPredicates() {
			groupPreds[p] = true
		}
	})
	return groupPreds
}

// desiredGroup returns the payload's group-`source` statements stamped as
// graph-ingest stamps the stream arrival: an empty source becomes semsource
// and a zero timestamp becomes the envelope's. Statements another source
// owns are left out, never re-stamped, so the reconcile cannot claim them.
func desiredGroup(triples []message.Triple, at time.Time) []message.Triple {
	group := groupPredicates()
	out := make([]message.Triple, 0, len(triples))
	for _, t := range triples {
		if !group[t.Predicate] || (t.Source != "" && t.Source != envelopeSource) {
			continue
		}
		t.Source = envelopeSource
		if t.Timestamp.IsZero() {
			t.Timestamp = at
		}
		out = append(out, t)
	}
	return out
}

// reconcileAttempts bounds re-reads after a revision conflict. The stream
// write this reconcile follows is acknowledged when stored in the stream, not
// when graph-ingest applies it, so the first attempt routinely reads the
// state before that write and loses the compare-and-set to it. Every
// attempt re-reads, so a retry never writes over state it has not seen.
const reconcileAttempts = 3

// reconcile sets the entity's semsource statements in group `source` to
// exactly the ones this publish carried. The stream write already delivered
// the statements; only the removal of dropped ones rides on this, so a
// failure is counted and logged but never fails the entity.
func (p *Publisher) reconcile(ctx context.Context, payload *graph.EntityPayload, at time.Time) {
	if p.reconciler == nil || payload.Partial {
		return
	}
	request := projection.ReconcileMutation{
		Contract: graph.SourceEntityContract().Name,
		Group:    graph.GroupSource,
		EntityID: payload.ID,
		Desired:  desiredGroup(payload.TripleData, at),
		// No metadata timestamp: the client refuses one that differs from a
		// statement's own, and statements such as the lifecycle owner fact
		// carry their own. desiredGroup stamps every other statement.
		Metadata: projection.MutationMetadata{Source: envelopeSource},
	}
	var err error
	for attempt := 0; attempt < reconcileAttempts; attempt++ {
		if _, err = p.reconciler.Reconcile(ctx, request); err == nil {
			p.reconciled.Add(1)
			p.metrics.incReconciled()
			return
		}
		var me *projection.MutationError
		if !errors.As(err, &me) || me.Kind != projection.MutationRevisionConflict {
			break
		}
		if attempt < reconcileAttempts-1 {
			p.metrics.incReconcileRetries()
		}
	}
	var ce *errs.ClassifiedError
	if errors.As(err, &ce) && ce.Code == gtypes.ErrorCodeEntityNotFound {
		// A birth whose stream write has not landed yet: nothing is stale.
		return
	}
	p.reconcileFailed.Add(1)
	p.metrics.incReconcileFailed()
	p.logger.Debug("entity reconcile failed; the next publish repairs it",
		"id", payload.ID, "error", err)
}

// Reconciled returns how many reconciles committed (applied or unchanged).
func (p *Publisher) Reconciled() int64 { return p.reconciled.Load() }

// ReconcileFailed returns how many reconciles failed for a reason other than
// the entity not existing yet.
func (p *Publisher) ReconcileFailed() int64 { return p.reconcileFailed.Load() }
