package entitypub

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
)

// breakerPublisher reports the circuit breaker as open for the first N calls,
// then accepts. It reproduces backpressure that RECOVERS — the case that
// previously logged nothing above Debug and exported no signal at all.
type breakerPublisher struct {
	openCalls int64
	seen      atomic.Int64
}

func (b *breakerPublisher) PublishToStream(context.Context, string, []byte) error {
	if b.seen.Add(1) <= b.openCalls {
		// Matched by string in publishOne — the only retryable error.
		return errors.New("circuit breaker is open")
	}
	return nil
}

// countingHandler captures log records so a test can assert how MANY times a
// condition was reported, not merely that it was.
type countingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

func (h *countingHandler) count(level slog.Level, substr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == level && strings.Contains(r.Message, substr) {
			n++
		}
	}
	return n
}

func newTestPublisher(t *testing.T, client NATSPublisher, opts ...Option) (*Publisher, *countingHandler) {
	t.Helper()
	h := &countingHandler{}
	p, err := New(client, slog.New(h), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return p, h
}

// fastBackoff keeps retry sequences in the millisecond range so tests that
// exercise recovery do not real-clock the production 500ms-exponential ladder.
// Tests that must STAY in backpressure (never recover within the test window)
// keep the default backoff instead: 20 attempts at fast backoff reach the
// terminal maxAttempts failure in tens of milliseconds.
func fastBackoff() Option { return WithRetryBackoff(time.Millisecond, 4*time.Millisecond) }

func payload(id string) *graph.EntityPayload {
	return &graph.EntityPayload{
		ID:                  "org.semsource.golang.sys.function." + id,
		UpdatedAt:           time.Now(),
		IndexingProfileHint: graph.IndexingProfileContent,
	}
}

// TestRetryPressureIsDistinctFromFailure is the core diagnostic separation: a
// publisher retrying against a refusing transport must show retries WITHOUT
// showing failures or drops. Folding retries into failures — or not exporting
// them — is what made a stalled publisher indistinguishable from a healthy one.
func TestRetryPressureIsDistinctFromFailure(t *testing.T) {
	client := &breakerPublisher{openCalls: 3}
	p, _ := newTestPublisher(t, client, fastBackoff())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	if err := p.Send(payload("e1")); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return p.Published() == 1 }, "entity to publish after retries")

	if got := p.Retries(); got != 3 {
		t.Errorf("retries = %d, want 3", got)
	}
	if got := p.Failed(); got != 0 {
		t.Errorf("failed = %d, want 0 — a retry that eventually succeeded is not a failure", got)
	}
	if got := p.Dropped(); got != 0 {
		t.Errorf("dropped = %d, want 0", got)
	}
}

// TestBackpressureLogsOnceAndReportsRecovery pins the transition contract: a
// sustained condition costs ONE line, not one per attempt, and clearing it is
// reported. Logging per attempt is what forced this signal down to Debug.
func TestBackpressureLogsOnceAndReportsRecovery(t *testing.T) {
	client := &breakerPublisher{openCalls: 3}
	p, h := newTestPublisher(t, client, fastBackoff())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	if err := p.Send(payload("e1")); err != nil {
		t.Fatal(err)
	}

	// Wait on the condition under test — the recovery LINE — not on the
	// published counter: the counter becomes visible before the drain goroutine
	// flips the backpressure state and logs, so waiting on it races every
	// assertion below (#155's "flake" was exactly that window).
	waitFor(t, func() bool { return h.count(slog.LevelInfo, "backpressure cleared") >= 1 },
		"recovery line after retries")

	if p.Retries() < 3 {
		t.Fatalf("retries = %d, want >= 3; the test would not exercise repetition", p.Retries())
	}
	if got := h.count(slog.LevelWarn, "backpressure"); got != 1 {
		t.Errorf("WARN backpressure lines = %d, want exactly 1 across %d retries", got, p.Retries())
	}
	if got := h.count(slog.LevelInfo, "backpressure cleared"); got != 1 {
		t.Errorf("recovery lines = %d, want exactly 1", got)
	}
	if p.InBackpressure() {
		t.Error("InBackpressure() still true after recovery")
	}
}

// TestBackpressureStateIsObservableWhileActive proves the condition is readable
// from outside the process while it is happening — the question that could not
// be answered during the incident.
func TestBackpressureStateIsObservableWhileActive(t *testing.T) {
	// Never recovers within the test window.
	client := &breakerPublisher{openCalls: 1 << 30}
	p, _ := newTestPublisher(t, client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx)
	if err := p.Send(payload("e1")); err != nil {
		t.Fatal(err)
	}

	waitFor(t, p.InBackpressure, "backpressure state to become observable")

	if p.Published() != 0 {
		t.Errorf("published = %d, want 0 while the transport refuses", p.Published())
	}
	if p.Failed() != 0 || p.Dropped() != 0 {
		t.Errorf("failed=%d dropped=%d, want 0/0 — stalled is not the same as losing data",
			p.Failed(), p.Dropped())
	}
}

func TestMetricSubsystemSanitisesInstanceNames(t *testing.T) {
	// Prometheus rejects hyphens; an unsanitised name registers nothing and the
	// series silently never appears.
	for in, want := range map[string]string{
		"ast-source-workspace": "ast_source_workspace",
		"doc.source":           "doc_source",
		"9lives":               "_9lives",
		"already_ok":           "already_ok",
		"":                     "",
	} {
		if got := metricSubsystem(in); got != want {
			t.Errorf("metricSubsystem(%q) = %q, want %q", in, got, want)
		}
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	// Recovery tests use fastBackoff so conditions arrive in milliseconds; the
	// deadline stays generous so a loaded CI runner cannot turn slow into failed.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
