package entitypub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestClassifyPublishError is the class table (#176): retryable is decided by
// errors.Is/As over typed values — including wrapped forms — never by string.
func TestClassifyPublishError(t *testing.T) {
	bg := context.Background()
	capacityErr := &jetstream.APIError{ErrorCode: jetstream.ErrorCode(10077), Description: "maximum bytes exceeded"}
	for _, tc := range []struct {
		name string
		err  error
		want publishErrClass
	}{
		{"circuit open", natsclient.ErrCircuitOpen, publishRetryable},
		{"circuit open wrapped", fmt.Errorf("publish: %w", natsclient.ErrCircuitOpen), publishRetryable},
		{"not connected", natsclient.ErrNotConnected, publishRetryable},
		{"nats timeout", nats.ErrTimeout, publishRetryable},
		{"deadline exceeded", context.DeadlineExceeded, publishRetryable},
		{"no responders", nats.ErrNoResponders, publishRetryable},
		{"stream capacity bytes", capacityErr, publishRetryable},
		{"stream capacity wrapped", fmt.Errorf("nats: %w", capacityErr), publishRetryable},
		{"capacity code, unrelated description", &jetstream.APIError{ErrorCode: jetstream.ErrorCode(10077), Description: "something else"}, publishTerminal},
		{"other api error", &jetstream.APIError{ErrorCode: jetstream.ErrorCode(10071), Description: "wrong last sequence"}, publishTerminal},
		{"unknown error", errors.New("invalid subject"), publishTerminal},
	} {
		if got := classifyPublishError(bg, tc.err); got != tc.want {
			t.Errorf("classify(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestClassifyPublishError_ShutdownIsCanceled — run-context cancellation is
// neither a retry nor a failure; it is the drain path's business.
func TestClassifyPublishError_ShutdownIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := classifyPublishError(ctx, ctx.Err()); got != publishCanceled {
		t.Errorf("canceled run ctx = %v, want publishCanceled", got)
	}
	// A deadline error with a LIVE run context is transport trouble, not
	// shutdown: it must stay retryable.
	if got := classifyPublishError(context.Background(), context.DeadlineExceeded); got != publishRetryable {
		t.Errorf("deadline with live ctx = %v, want publishRetryable", got)
	}
}

// capturingPublisher records msgIDs and fails the first N attempts with a
// configurable error, then succeeds.
type capturingPublisher struct {
	mu      sync.Mutex
	msgIDs  []string
	failN   int64
	failErr error
	seen    atomic.Int64
}

func (c *capturingPublisher) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return c.PublishToStreamWithMsgID(ctx, subject, data, "")
}

func (c *capturingPublisher) PublishToStreamWithMsgID(_ context.Context, _ string, _ []byte, msgID string) error {
	c.mu.Lock()
	c.msgIDs = append(c.msgIDs, msgID)
	c.mu.Unlock()
	if c.seen.Add(1) <= c.failN {
		return c.failErr
	}
	return nil
}

// TestPublishOne_MsgIDDeterministicAcrossAttempts — the dedup contract (#176,
// ADR-055): every attempt of one payload carries the SAME Nats-Msg-Id, and a
// different payload carries a different one.
func TestPublishOne_MsgIDDeterministicAcrossAttempts(t *testing.T) {
	client := &capturingPublisher{failN: 2, failErr: nats.ErrTimeout}
	p, _ := newTestPublisher(t, client, WithRetryBackoff(time.Millisecond, 2*time.Millisecond))

	if err := p.publishOne(context.Background(), payloadN("alpha")); err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	client.mu.Lock()
	ids := append([]string(nil), client.msgIDs...)
	client.mu.Unlock()
	if len(ids) != 3 {
		t.Fatalf("attempts = %d, want 3 (2 failures + success)", len(ids))
	}
	if ids[0] == "" || ids[0] != ids[1] || ids[1] != ids[2] {
		t.Errorf("msgID must be identical and non-empty across attempts, got %v", ids)
	}

	client2 := &capturingPublisher{}
	p2, _ := newTestPublisher(t, client2)
	if err := p2.publishOne(context.Background(), payloadN("beta")); err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if client2.msgIDs[0] == ids[0] {
		t.Error("different payloads must carry different msgIDs")
	}
}

// TestPublishOne_CapacityErrorsDriveBackpressure closes the D3 blind spot the
// live induction falsified: a stream-capacity refusal must move the retries
// counter and the backpressure gauge exactly like a breaker-open does.
func TestPublishOne_CapacityErrorsDriveBackpressure(t *testing.T) {
	capacityErr := fmt.Errorf("nats: %w", &jetstream.APIError{ErrorCode: jetstream.ErrorCode(10077), Description: "maximum bytes exceeded"})
	client := &capturingPublisher{failN: 3, failErr: capacityErr}
	p, h := newTestPublisher(t, client, WithRetryBackoff(time.Millisecond, 2*time.Millisecond))

	if err := p.publishOne(context.Background(), payloadN("gamma")); err != nil {
		t.Fatalf("publishOne must recover once capacity frees: %v", err)
	}
	if got := p.Retries(); got != 3 {
		t.Errorf("Retries() = %d, want 3", got)
	}
	if p.InBackpressure() {
		t.Error("backpressure must clear on success")
	}
	if n := h.count(slog.LevelWarn, "publish backpressure"); n != 1 {
		t.Errorf("backpressure entry lines = %d, want exactly 1 (edge-triggered)", n)
	}
}

// TestPublishOne_TerminalWording — an entity failed without retries must never
// claim retries happened; budget exhaustion states the attempt count.
func TestPublishOne_TerminalWording(t *testing.T) {
	client := &capturingPublisher{failN: 1 << 30, failErr: errors.New("invalid subject")}
	p, _ := newTestPublisher(t, client)
	err := p.publishOne(context.Background(), payloadN("delta"))
	if err == nil {
		t.Fatal("terminal error must fail")
	}
	if !strings.Contains(err.Error(), "first attempt") || strings.Contains(err.Error(), "after retries") {
		t.Errorf("first-attempt terminal wording wrong: %v", err)
	}

	client2 := &capturingPublisher{failN: 1 << 30, failErr: nats.ErrTimeout}
	p2, _ := newTestPublisher(t, client2, WithRetryBackoff(time.Microsecond, time.Microsecond))
	err = p2.publishOne(context.Background(), payloadN("epsilon"))
	if err == nil {
		t.Fatal("budget exhaustion must fail")
	}
	if !strings.Contains(err.Error(), "delivery budget exhausted after 20 attempts") {
		t.Errorf("budget-exhaustion wording wrong: %v", err)
	}
}

// TestDrainBatch_FailureFloodAggregates (#176 / ADR-0011): N terminal failures
// are ONE default-level entry plus a recovery line — never N lines.
func TestDrainBatch_FailureFloodAggregates(t *testing.T) {
	client := &capturingPublisher{failN: 50, failErr: errors.New("invalid subject")}
	p, h := newTestPublisher(t, client, WithCapacity(64))

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		if err := p.Send(payloadN(fmt.Sprintf("f%d", i))); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	for p.Failed() < 50 {
		p.drainBatch(ctx)
	}
	if n := h.count(slog.LevelWarn, "entity publishes are failing"); n != 1 {
		t.Errorf("failure entry lines = %d, want exactly 1 for a 50-entity flood", n)
	}
	if got := p.Failed(); got != 50 {
		t.Errorf("Failed() = %d, want 50 — aggregation must not hide the count", got)
	}

	// Recovery: the next successful publish clears the condition, once.
	if err := p.publishOne(ctx, payloadN("recovered")); err != nil {
		t.Fatalf("recovery publish: %v", err)
	}
	if n := h.count(slog.LevelInfo, "entity publishing recovered"); n != 1 {
		t.Errorf("recovery lines = %d, want exactly 1", n)
	}
}
