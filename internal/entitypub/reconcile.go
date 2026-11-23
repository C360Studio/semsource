package entitypub

import (
	"context"
	"errors"
	"fmt"
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
	ReadAuthoritative(context.Context, string) (*gtypes.ExactEntity, error)
	Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error)
}

// WithReconcile makes every successful stream publish not marked
// SkipReconcile followed by one reconcile of contract group `source`, so
// predicates the source stopped emitting are removed (#336). A handler passes
// it only once a test proves each of its publishes carries the entity's
// complete semsource statement set.
func WithReconcile(r Reconciler) Option {
	return func(c *publisherConfig) { c.reconciler = r }
}

const (
	// requestTimeout bounds each request the mutation client sends; one
	// reconcile is a few of them (reads, then the conditional replace).
	requestTimeout = 5 * time.Second
	// reconcileBudget bounds one entity's whole reconcile, waiting for its
	// stream write to apply included. Reconciles run off the drain loop, so
	// the budget never delays delivery.
	reconcileBudget = 30 * time.Second
	// reconcileQueueSize bounds queued reconciles; a full queue drops the
	// reconcile (counted) rather than stall the drain.
	reconcileQueueSize = 4096
	appliedPollStart   = 20 * time.Millisecond
	appliedPollMax     = 500 * time.Millisecond
)

// NewSourceReconciler returns the projection mutation client a handler passes
// to WithReconcile, bound to the source-entity contract.
func NewSourceReconciler(nc *natsclient.Client) (Reconciler, error) {
	return projection.NewMutationClient(projection.MutationClientConfig{
		NATS:      nc,
		Contracts: []projection.Contract{graph.SourceEntityContract()},
		Timeout:   requestTimeout,
	})
}

var (
	groupOnce  sync.Once
	groupPreds map[string]bool
)

// groupPredicates is the predicate set of the contract group the reconcile is
// sent against, computed on first use: the vocabulary is registered at
// bootstrap, after package init.
func groupPredicates() map[string]bool {
	groupOnce.Do(func() {
		groupPreds = map[string]bool{}
		for _, g := range graph.SourceEntityContract().Groups {
			if g.Name != graph.GroupSource {
				continue
			}
			for _, p := range g.Predicates {
				groupPreds[p] = true
			}
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

// reconcileJob is one delivered publish whose group the worker reconciles.
type reconcileJob struct {
	id      string
	seq     uint64 // enqueue order; two publishes can share an envelope millisecond
	desired []message.Triple
}

// enqueueReconcile hands a delivered publish to the reconcile worker. It runs
// on the drain loop before the delivery counters move, so the job is counted
// in flight and WaitSettled covers it. It never blocks: a full queue drops
// the reconcile, counted, and the next edit of the entity repairs it.
func (p *Publisher) enqueueReconcile(payload *graph.EntityPayload, at time.Time) {
	if p.reconciler == nil || payload.SkipReconcile || p.stopping() {
		return
	}
	p.latestMu.Lock()
	p.reconcileSeq++
	job := reconcileJob{id: payload.ID, seq: p.reconcileSeq, desired: desiredGroup(payload.TripleData, at)}
	p.latest[job.id] = job.seq
	p.latestMu.Unlock()
	p.reconcilePending.Add(1)
	select {
	case p.reconcileQ <- job:
	default:
		p.reconcilePending.Add(-1)
		p.reconcileFailure(job.id, errors.New("reconcile queue full"))
	}
}

// reconcileWorker runs queued reconciles one at a time until the queue is
// closed. Once Stop has begun, queued jobs are discarded uncounted: a
// shutdown is not a reconcile failure, and the next edit repairs it.
func (p *Publisher) reconcileWorker(ctx context.Context) {
	defer close(p.reconcileDone)
	for job := range p.reconcileQ {
		if !p.stopping() && !p.superseded(job) {
			p.reconcile(ctx, job)
		}
		p.latestMu.Lock()
		if p.latest[job.id] == job.seq {
			delete(p.latest, job.id)
		}
		p.latestMu.Unlock()
		p.reconcilePending.Add(-1)
		p.noteOutcome()
	}
}

// superseded reports whether a later publish of the same entity was queued:
// its own reconcile covers this one, and this one's statements may never be
// stored as such (the later write can apply first).
func (p *Publisher) superseded(job reconcileJob) bool {
	p.latestMu.Lock()
	defer p.latestMu.Unlock()
	return p.latest[job.id] != job.seq
}

// reconcile sets the entity's semsource statements in group `source` to
// exactly the ones this publish carried, once graph-ingest has applied this
// publish's stream write. A stream write is acknowledged when stored, not
// when applied, and graph-ingest applies one entity's writes in arrival
// order (keyed_ingest.go, SemStreams ADR-072). So waiting until the stored
// state shows this write means every older write of the entity has applied
// too, and none can land afterwards to restore a predicate this reconcile
// removed. The replace is fenced at the revision that showed it.
func (p *Publisher) reconcile(ctx context.Context, job reconcileJob) {
	ctx, cancel := context.WithTimeout(ctx, reconcileBudget)
	defer cancel()
	request := projection.ReconcileMutation{
		Contract: graph.SourceEntityContract().Name,
		Group:    graph.GroupSource,
		EntityID: job.id,
		Desired:  job.desired,
		// No metadata timestamp: the client refuses one that differs from a
		// statement's own, and statements such as the lifecycle owner fact
		// carry their own. desiredGroup stamps every other statement.
		Metadata: projection.MutationMetadata{Source: envelopeSource},
	}
	wait := appliedPollStart
	for {
		exact, err := p.reconciler.ReadAuthoritative(ctx, job.id)
		switch {
		case err != nil && !isNotFound(err):
			p.reconcileFailure(job.id, err)
			return
		case err == nil && applied(exact.Entity, job.desired):
			request.ExpectedRevision = exact.KVRevision
			_, err = p.reconciler.Reconcile(ctx, request)
			if err == nil {
				p.reconciled.Add(1)
				p.metrics.incReconciled()
				return
			}
			if !isRevisionConflict(err) {
				p.reconcileFailure(job.id, err)
				return
			}
		}
		// Not applied yet, absent (a birth whose write is still queued), or
		// moved under the replace: read again after a short backoff, unless
		// a later publish of the entity has been queued meanwhile.
		if p.superseded(job) {
			return
		}
		p.metrics.incReconcileRetries()
		select {
		case <-ctx.Done():
			p.reconcileFailure(job.id, fmt.Errorf("stream write not applied within %s: %w", reconcileBudget, ctx.Err()))
			return
		case <-p.stop:
			return // shutdown: not a failure; the next edit repairs it
		case <-time.After(wait):
		}
		wait = min(2*wait, appliedPollMax)
	}
}

// applied reports whether stored holds every statement of desired, the
// group statements this publish carried, stamped as the stream arrival is.
func applied(stored *gtypes.EntityState, desired []message.Triple) bool {
	if stored == nil {
		return false
	}
	have := make(map[string]bool, len(stored.Triples))
	for _, t := range stored.Triples {
		have[statementKey(t)] = true
	}
	for _, t := range desired {
		if !have[statementKey(t)] {
			return false
		}
	}
	return true
}

// statementKey identifies a statement by the fields a publish stamps. The
// object goes through fmt so a JSON round trip's number types compare equal.
func statementKey(t message.Triple) string {
	return fmt.Sprintf("%s\x00%s\x00%v\x00%s\x00%d", t.Subject, t.Predicate, t.Object, t.Source, t.Timestamp.UnixMilli())
}

func isNotFound(err error) bool {
	var ce *errs.ClassifiedError
	return errors.As(err, &ce) && ce.Code == gtypes.ErrorCodeEntityNotFound
}

func isRevisionConflict(err error) bool {
	var me *projection.MutationError
	return errors.As(err, &me) && me.Kind == projection.MutationRevisionConflict
}

// reconcileFailure counts and logs one reconcile that will not complete. A
// cancellation during shutdown is not a failure and is not counted.
func (p *Publisher) reconcileFailure(id string, err error) {
	if p.stopping() && errors.Is(err, context.Canceled) {
		return
	}
	p.reconcileFailed.Add(1)
	p.metrics.incReconcileFailed()
	p.logger.Debug("entity reconcile failed; the next edit repairs it", "id", id, "error", err)
}

// stopping reports whether Stop has begun.
func (p *Publisher) stopping() bool {
	select {
	case <-p.stop:
		return true
	default:
		return false
	}
}

// Reconciled returns how many reconciles committed (applied or unchanged).
func (p *Publisher) Reconciled() int64 { return p.reconciled.Load() }

// ReconcileFailed returns how many reconciles will not complete: an error, a
// stream write not applied within the budget, or a full queue.
func (p *Publisher) ReconcileFailed() int64 { return p.reconcileFailed.Load() }
