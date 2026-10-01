package entitypub

import (
	"context"
	"errors"
	"fmt"
	"github.com/c360studio/semsource/internal/seedproof"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
)

type receiptObserver struct {
	mu           sync.Mutex
	publications []sourceintent.Publication
	manifests    []sourceintent.SeedManifest
	attempts     int
	fail         int
	entered      chan struct{}
	release      chan struct{}
}

func (o *receiptObserver) Published(ctx context.Context, p sourceintent.Publication) error {
	o.mu.Lock()
	o.attempts++
	attempt := o.attempts
	o.mu.Unlock()
	if o.entered != nil {
		select {
		case o.entered <- struct{}{}:
		default:
		}
	}
	if o.release != nil {
		select {
		case <-o.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if attempt <= o.fail {
		return errors.New("receipt store unavailable")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.publications = append(o.publications, p)
	return nil
}
func (o *receiptObserver) SeedFinished(_ context.Context, m sourceintent.SeedManifest) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.manifests = append(o.manifests, m)
	return nil
}
func testBinding() sourceintent.Binding {
	return sourceintent.Binding{Authority: entityid.Authority{Org: "org", Platform: "semsource"}, Namespace: "test", Handle: "docs", Generation: 2, BootEpoch: "boot-new", Factory: "doc-source", ConfigDigest: "digest"}
}

func TestPublisherReceiptAfterAck(t *testing.T) {
	transport := &settlementPublisher{entered: make(chan context.Context, 1), release: make(chan struct{})}
	observer := &receiptObserver{fail: 2}
	p, _ := newTestPublisher(t, transport, WithRetryBackoff(time.Millisecond, time.Millisecond))
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	batch, err := p.BeginSeed("initial", true)
	if err != nil {
		t.Fatal(err)
	}
	payload := payloadN("before")
	if err := batch.Send(payload); err != nil {
		t.Fatal(err)
	}
	payload.ID = "mutated"
	p.Start(context.Background())
	<-transport.entered
	observer.mu.Lock()
	attempts := observer.attempts
	observer.mu.Unlock()
	if attempts != 0 {
		t.Fatal("receipt before PubAck")
	}
	finish := make(chan error, 1)
	go func() { finish <- batch.Finish(t.Context(), nil) }()
	select {
	case err := <-finish:
		t.Fatalf("finished in-flight batch: %v", err)
	default:
	}
	close(transport.release)
	if err := <-finish; err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.attempts != 3 || len(observer.publications) != 1 || p.Published() != 1 || p.Lost() != 0 {
		t.Fatalf("receipt retry republished/lost: attempts=%d published=%d lost=%d", observer.attempts, p.Published(), p.Lost())
	}
	if observer.publications[0].Payload.ID == "mutated" {
		t.Fatal("evidence was mutable")
	}
	if len(observer.manifests) != 1 || !observer.manifests[0].Successful || len(observer.manifests[0].Entries) != 1 {
		t.Fatalf("wrong manifest: %+v", observer.manifests)
	}
}

func TestPublisherSeedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"empty", nil}, {"failed", errors.New("enumeration failed")}} {
		t.Run(tc.name, func(t *testing.T) {
			observer := &receiptObserver{}
			p, _ := newTestPublisher(t, &capturingPublisher{})
			if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
				t.Fatal(err)
			}
			p.Start(context.Background())
			batch, err := p.BeginSeed("initial", true)
			if err != nil {
				t.Fatal(err)
			}
			err = batch.Finish(t.Context(), tc.err)
			if (err != nil) != (tc.err != nil) {
				t.Fatalf("finish=%v", err)
			}
			_ = p.Stop(t.Context())
			observer.mu.Lock()
			defer observer.mu.Unlock()
			if tc.err == nil {
				if len(observer.manifests) != 1 || len(observer.manifests[0].Entries) != 0 || !observer.manifests[0].Successful {
					t.Fatal("explicit empty seed not sealed")
				}
			} else {
				for _, m := range observer.manifests {
					if m.Successful {
						t.Fatal("failed seed sealed success")
					}
				}
			}
		})
	}
}

func TestPublisherReceiptShutdownDeadline(t *testing.T) {
	observer := &receiptObserver{entered: make(chan struct{}, 1), release: make(chan struct{})}
	p, _ := newTestPublisher(t, &capturingPublisher{}, WithRetryBackoff(time.Millisecond, time.Millisecond))
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	batch, _ := p.BeginSeed("initial", true)
	if err := batch.Send(payloadN("pending")); err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	<-observer.entered
	if p.ReceiptErrors() == 0 {
		t.Fatal("unsettled receipt invisible")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.Stop(ctx); err == nil {
		t.Fatal("stop hid unresolved receipt")
	}
	if p.Published() != 1 || p.Lost() != 0 {
		t.Fatal("receipt failure counted as transport loss")
	}
	close(observer.release)
	if err := batch.Finish(ctx, nil); err == nil {
		t.Fatal("canceled seed sealed success")
	}
}

func TestPublisherWatchBatchCannotCompleteFailedSeed(t *testing.T) {
	observer := &receiptObserver{}
	p, _ := newTestPublisher(t, &capturingPublisher{})
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	initial, _ := p.BeginSeed("initial", true)
	if err := initial.Send(payloadN("old")); err != nil {
		t.Fatal(err)
	}
	if err := initial.Finish(t.Context(), errors.New("truncated enumeration")); err == nil {
		t.Fatal("failed seed completed")
	}
	if err := p.Send(payloadN("live")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := p.Stop(ctx); err == nil {
		t.Fatal("failed initial seed hidden by watch")
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.manifests) != 2 || !observer.manifests[0].Initial || observer.manifests[0].Successful || observer.manifests[1].Initial || observer.manifests[1].BatchID == "initial" {
		t.Fatalf("watch amended old seed: %+v", observer.manifests)
	}
}

func TestPublisherSeedRejectedSendCannotSeal(t *testing.T) {
	observer := &receiptObserver{}
	p, _ := newTestPublisher(t, &capturingPublisher{}, WithCapacity(1), WithSendTimeout(time.Millisecond))
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	batch, _ := p.BeginSeed("initial", true)
	if err := batch.Send(payloadN("one")); err != nil {
		t.Fatal(err)
	}
	if err := batch.Send(payloadN("two")); err == nil {
		t.Fatal("fixture failed to overflow")
	}
	p.Start(context.Background())
	if err := batch.Finish(t.Context(), nil); err == nil {
		t.Fatal("rejected publication still sealed")
	}
	_ = p.Stop(t.Context())
	observer.mu.Lock()
	defer observer.mu.Unlock()
	for _, manifest := range observer.manifests {
		if manifest.Successful {
			t.Fatal("partial seed sealed")
		}
	}
}

func TestPublisherBindRequiresPreAdmission(t *testing.T) {
	p, _ := newTestPublisher(t, &capturingPublisher{})
	p.Start(context.Background())
	if err := p.BindSourceLifecycle(testBinding(), &receiptObserver{}); err == nil {
		t.Fatal("binding after Start admitted")
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	q, _ := newTestPublisher(t, &capturingPublisher{})
	if err := q.Send(payloadN("prior")); err != nil {
		t.Fatal(err)
	}
	if err := q.BindSourceLifecycle(testBinding(), &receiptObserver{}); err == nil {
		t.Fatal("binding after Send admitted")
	}
}

type supersededObserver struct {
	calls     int
	manifests int
}

func (o *supersededObserver) Published(context.Context, sourceintent.Publication) error {
	o.calls++
	return sourceintent.ErrSuperseded
}
func (o *supersededObserver) SeedFinished(context.Context, sourceintent.SeedManifest) error {
	o.manifests++
	return sourceintent.ErrSuperseded
}

func TestPublisherSupersededBindingDrainsWithoutGrantingFreshness(t *testing.T) {
	observer := &supersededObserver{}
	p, _ := newTestPublisher(t, &capturingPublisher{})
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	batch, _ := p.BeginSeed("initial", true)
	if err := batch.Send(payloadN("old-generation")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := batch.Finish(ctx, nil); err != nil {
		t.Fatalf("irrelevant receipt blocked live producer: %v", err)
	}
	if err := p.Send(payloadN("watch-old-generation")); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("superseded producer could not drain: %v", err)
	}
	if observer.calls != 2 || observer.manifests != 0 || p.Published() != 2 || p.ReceiptErrors() != 0 || p.Lost() != 0 {
		t.Fatalf("superseded evidence incorrectly completed or lost: receipts=%d manifests=%d errors=%d", observer.calls, observer.manifests, p.ReceiptErrors())
	}
}

func TestPublisherInitialSeedProofIsBoundOnly(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint(bound), func(t *testing.T) {
			observer := &receiptObserver{}
			p, _ := newTestPublisher(t, &capturingPublisher{})
			if bound {
				if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
					t.Fatal(err)
				}
			}
			p.Start(context.Background())
			err := p.RunInitialSeed(t.Context(), func(ctx context.Context) error {
				if err := p.SendContext(ctx, payloadN("current")); err != nil {
					return err
				}
				seedproof.Report(ctx, errors.New("unreadable sibling"))
				return nil
			})
			if (err != nil) != bound {
				t.Fatalf("proof did not preserve ordinary ingest: bound=%t err=%v", bound, err)
			}
			if err := p.SendContext(t.Context(), payloadN("watch")); err != nil {
				t.Fatal(err)
			}
			_ = p.Stop(t.Context())
			if bound {
				observer.mu.Lock()
				defer observer.mu.Unlock()
				if len(observer.manifests) != 2 || observer.manifests[0].Successful || !observer.manifests[0].Initial || observer.manifests[1].Initial {
					t.Fatalf("wrong proof separation: %+v", observer.manifests)
				}
			}
		})
	}
}

func TestPublisherRejectsInvalidBatchEvidence(t *testing.T) {
	observer := &receiptObserver{}
	p, _ := newTestPublisher(t, &capturingPublisher{})
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	if _, err := p.BeginSeed("live", false); err == nil {
		t.Fatal("live admitted before initial")
	}
	if _, err := p.BeginSeed("", true); err == nil {
		t.Fatal("empty batch admitted")
	}
	batch, _ := p.BeginSeed("initial", true)
	if _, err := p.BeginSeed("initial", true); err == nil {
		t.Fatal("batch identity reused")
	}
	if _, err := p.BeginSeed("different", true); err == nil {
		t.Fatal("second initial admitted")
	}
	if err := batch.Send(nil); err == nil {
		t.Fatal("nil evidence accepted")
	}
	foreign := payloadN("foreign")
	foreign.ID = "foreign.platform.docs.web.doc.one"
	if err := batch.Send(foreign); err == nil {
		t.Fatal("foreign authority evidence accepted")
	}
	p.Start(context.Background())
	if err := batch.Finish(t.Context(), nil); err == nil {
		t.Fatal("invalid seed completed")
	}
	if err := batch.Send(payloadN("late")); err == nil {
		t.Fatal("closed batch accepted payload")
	}
	if err := batch.Finish(t.Context(), nil); err == nil {
		t.Fatal("closed batch resealed")
	}
	_ = p.Stop(t.Context())
	if _, err := p.BeginSeed("late", false); err == nil {
		t.Fatal("stopped publisher admitted batch")
	}
}

func TestPublisherRejectedLiveBatchesDoNotAccumulate(t *testing.T) {
	observer := &receiptObserver{}
	p, _ := newTestPublisher(t, &capturingPublisher{}, WithCapacity(1), WithSendTimeout(time.Millisecond))
	if err := p.BindSourceLifecycle(testBinding(), observer); err != nil {
		t.Fatal(err)
	}
	initial, err := p.BeginSeed("initial", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.Finish(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	// Keep one accepted live publication queued; rejected attempts must release
	// only their own bookkeeping and cannot claim publication evidence.
	if err := p.Send(payloadN("queued")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err := p.Send(nil); err == nil {
			t.Fatal("invalid publication accepted")
		}
		if err := p.Send(payloadN(fmt.Sprintf("overflow-%d", i))); err == nil {
			t.Fatal("full-buffer publication accepted")
		}
	}
	p.receiptMu.Lock()
	retained, tracked := len(p.batches), len(p.tracked)
	p.receiptMu.Unlock()
	if retained != 2 || tracked != 1 {
		t.Fatalf("rejected live publications leaked bookkeeping: batches=%d tracked=%d", retained, tracked)
	}
	p.Start(context.Background())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := p.Stop(ctx); err == nil {
		t.Fatal("rejected publications lost their visible failure status")
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.publications) != 1 || len(observer.manifests) != 2 || p.Published() != 1 {
		t.Fatalf("rejected publications granted evidence: receipts=%d manifests=%d published=%d", len(observer.publications), len(observer.manifests), p.Published())
	}
}
