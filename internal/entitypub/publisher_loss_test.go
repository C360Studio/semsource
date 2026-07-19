package entitypub

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
)

// gatedPublisher blocks PublishToStream until released, simulating a wedged
// NATS connection without sleeps.
type gatedPublisher struct {
	mu       sync.Mutex
	gate     chan struct{}
	sent     []string
	sendSeen chan struct{}
}

func newGatedPublisher() *gatedPublisher {
	return &gatedPublisher{gate: make(chan struct{}), sendSeen: make(chan struct{}, 1024)}
}

func (g *gatedPublisher) PublishToStream(ctx context.Context, _ string, _ []byte) error {
	select {
	case <-g.gate:
	case <-ctx.Done():
		return ctx.Err()
	}
	g.mu.Lock()
	g.sent = append(g.sent, "x")
	g.mu.Unlock()
	select {
	case g.sendSeen <- struct{}{}:
	default:
	}
	return nil
}

func (g *gatedPublisher) sentCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sent)
}

func payloadN(id string) *graph.EntityPayload {
	return &graph.EntityPayload{
		ID:                  "org.semsource.golang.sys.function." + id,
		UpdatedAt:           time.Now(),
		IndexingProfileHint: graph.IndexingProfileContent,
	}
}

// TestSend_SustainedOverflowDropsLoudly pins the audit finding: when the
// buffer stays full past the bounded backpressure window, Send must return an
// error and the dropped counter must actually increment (it could never
// increment under DropOldest).
func TestSend_SustainedOverflowDropsLoudly(t *testing.T) {
	gp := newGatedPublisher() // never released → drain loop wedges on first item
	pub, err := New(gp, slog.Default(), WithCapacity(2), WithSendTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)

	// Fill: capacity 2 + 1 in-flight in the wedged drain loop is the worst
	// case; keep sending until Send fails.
	var dropErr error
	for i := 0; i < 10; i++ {
		if err := pub.Send(payloadN("e")); err != nil {
			dropErr = err
			break
		}
	}
	if dropErr == nil {
		t.Fatal("Send never returned an error under sustained overflow")
	}
	if got := pub.Dropped(); got < 1 {
		t.Errorf("Dropped() = %d, want >= 1 (the audit's dead counter)", got)
	}

	// Unblock shutdown: release the gate so the wedged publish completes.
	close(gp.gate)
	cancel()
	pub.Stop()
}

// TestSend_TransientOverflowLosesNothing pins the transient case: a full
// buffer that drains within the backpressure window delivers everything and
// the drop counter stays zero.
func TestSend_TransientOverflowLosesNothing(t *testing.T) {
	gp := newGatedPublisher()
	pub, err := New(gp, slog.Default(), WithCapacity(2), WithSendTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)

	const total = 8
	done := make(chan error, 1)
	go func() {
		for i := 0; i < total; i++ {
			if err := pub.Send(payloadN("t")); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	// Release the gate shortly after senders start blocking: the buffer
	// drains and every Send completes within the (generous) window.
	time.Sleep(20 * time.Millisecond)
	close(gp.gate)

	if err := <-done; err != nil {
		t.Fatalf("Send failed during transient overflow: %v", err)
	}
	// Wait for all deliveries (explicit synchronization, no arbitrary sleep).
	deadline := time.After(5 * time.Second)
	for gp.sentCount() < total {
		select {
		case <-gp.sendSeen:
		case <-deadline:
			t.Fatalf("delivered %d/%d before deadline", gp.sentCount(), total)
		}
	}
	if got := pub.Dropped(); got != 0 {
		t.Errorf("Dropped() = %d, want 0 for transient overflow", got)
	}
	cancel()
	pub.Stop()
}

// TestFailed_TerminalPublishErrorsAreCounted pins that entities leaving the
// buffer but failing NATS terminally are visible via Failed()/Lost().
func TestFailed_TerminalPublishErrorsAreCounted(t *testing.T) {
	pub, err := New(failingPublisher{}, slog.Default(), WithCapacity(4), WithSendTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)

	if err := pub.Send(payloadN("f")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for pub.Failed() < 1 {
		select {
		case <-deadline:
			t.Fatalf("Failed() = %d, want >= 1", pub.Failed())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if pub.Lost() < 1 {
		t.Errorf("Lost() = %d, want >= 1", pub.Lost())
	}
	cancel()
	pub.Stop()
}

type failingPublisher struct{}

func (failingPublisher) PublishToStream(context.Context, string, []byte) error {
	return context.DeadlineExceeded // terminal (non-circuit-breaker) error
}
