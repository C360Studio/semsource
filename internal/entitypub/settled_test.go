package entitypub

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
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

func mustPublisher(t *testing.T, client NATSPublisher) *Publisher {
	t.Helper()
	pub, err := New(client, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
