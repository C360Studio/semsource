package entitypub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

// enteringPublisher signals when a publish starts and holds it until released,
// so a test knows the drain loop has taken the entity out of the buffer.
type enteringPublisher struct {
	entered chan struct{}
	gate    chan struct{}
}

func (e *enteringPublisher) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return e.PublishToStream(ctx, subject, data)
}

func (e *enteringPublisher) PublishToStream(ctx context.Context, _ string, _ []byte) error {
	e.entered <- struct{}{}
	select {
	case <-e.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestWaitSettled_WaitsForTheEntityTheDrainLoopHolds pins #284: a source's
// post-seed report must wait for delivery, and an empty buffer is not
// delivery — the drain loop holds a read entity while it publishes.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Source status reflects delivery truth
func TestWaitSettled_WaitsForTheEntityTheDrainLoopHolds(t *testing.T) {
	if err := mustPublisher(t, &enteringPublisher{}).WaitSettled(context.Background()); err != nil {
		t.Fatalf("WaitSettled with nothing accepted = %v, want nil", err)
	}

	ep := &enteringPublisher{entered: make(chan struct{}, 1), gate: make(chan struct{})}
	pub := mustPublisher(t, ep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)
	var release sync.Once
	// Release before Stop: on a failed assertion the publish is still held, and
	// Stop settles accepted work rather than abandoning it.
	defer func() {
		release.Do(func() { close(ep.gate) })
		_ = pub.Stop(context.Background())
	}()

	if err := pub.Send(payloadN("held")); err != nil {
		t.Fatal(err)
	}
	<-ep.entered // the drain loop has read the entity and is publishing it

	if got := pub.Pending(); got != 0 {
		t.Fatalf("Pending() = %d while the entity is being published, want 0", got)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := pub.WaitSettled(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitSettled with an entity in flight = %v, want context.Canceled", err)
	}

	settled := make(chan error, 1)
	go func() {
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		settled <- pub.WaitSettled(wctx)
	}()
	release.Do(func() { close(ep.gate) })
	if err := <-settled; err != nil {
		t.Fatalf("WaitSettled after delivery = %v, want nil", err)
	}
	if got := pub.Published(); got != 1 {
		t.Fatalf("Published() = %d after WaitSettled returned, want 1", got)
	}
}

// recoveringPublisher refuses its first publish with a retryable error, then
// accepts, so the publisher enters backpressure and recovers on the retry.
type recoveringPublisher struct{ calls int }

func (r *recoveringPublisher) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return r.PublishToStream(ctx, subject, data)
}

func (r *recoveringPublisher) PublishToStream(context.Context, string, []byte) error {
	r.calls++
	if r.calls == 1 {
		return fmt.Errorf("first attempt: %w", nats.ErrNoResponders)
	}
	return nil
}

// pausingCounter holds the published metric's Inc, which runs after the
// delivery counter has moved, so a test can observe the publisher there.
type pausingCounter struct {
	prometheus.Counter
	reached chan struct{}
	resume  chan struct{}
}

func (c pausingCounter) Inc() {
	close(c.reached)
	<-c.resume
}

// TestWaitSettled_RecoveryStateSettlesWithDelivery pins the #287 review
// finding: once a retry succeeds, a caller woken by WaitSettled must not see
// full delivery alongside the backpressure flag of the recovered stall.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Backpressure is visible even when nothing is lost
func TestWaitSettled_RecoveryStateSettlesWithDelivery(t *testing.T) {
	pub, err := New(&recoveringPublisher{}, slog.Default(), WithRetryBackoff(time.Millisecond, time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	newCounter := func(name string) prometheus.Counter {
		return prometheus.NewCounter(prometheus.CounterOpts{Name: name})
	}
	paused := pausingCounter{Counter: newCounter("published"), reached: make(chan struct{}), resume: make(chan struct{})}
	pub.metrics = &pubMetrics{
		published:    paused,
		failed:       newCounter("failed"),
		dropped:      newCounter("dropped"),
		retries:      newCounter("retries"),
		backpressure: prometheus.NewGauge(prometheus.GaugeOpts{Name: "backpressure"}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)
	var resume sync.Once
	defer func() {
		resume.Do(func() { close(paused.resume) })
		_ = pub.Stop(context.Background())
	}()

	if err := pub.Send(payloadN("recovers")); err != nil {
		t.Fatal(err)
	}
	<-paused.reached // delivered and counted; the publisher is held here

	if err := pub.WaitSettled(ctx); err != nil {
		t.Fatalf("WaitSettled after delivery = %v, want nil", err)
	}
	if pub.InBackpressure() {
		t.Fatal("settled observer sees backpressure after the retry delivered")
	}
	if got := pub.Retries(); got != 1 {
		t.Fatalf("Retries() = %d, want 1: the harness must actually enter backpressure", got)
	}
}

func mustPublisher(t *testing.T, client NATSPublisher) *Publisher {
	t.Helper()
	pub, err := New(client, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
