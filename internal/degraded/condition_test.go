package degraded

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type capture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capture) Enabled(context.Context, slog.Level) bool { return true }
func (h *capture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *capture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capture) WithGroup(string) slog.Handler      { return h }

func (h *capture) count(level slog.Level, substr string) int {
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

// TestSustainedConditionLogsOnce is the whole point of the type: a condition
// that persists across many events must cost one line, not one per event. That
// is what allows it to sit at Warn instead of being pushed down to Debug.
func TestSustainedConditionLogsOnce(t *testing.T) {
	h := &capture{}
	logger := slog.New(h)
	var c Condition

	for i := 0; i < 500; i++ {
		c.Enter(logger, "broken", "i", i)
	}
	if got := h.count(slog.LevelWarn, "broken"); got != 1 {
		t.Errorf("WARN lines = %d across 500 occurrences, want exactly 1", got)
	}
	if !c.Active() {
		t.Error("Active() = false while degraded")
	}
}

// TestRecoveryIsReportedOnceAndReArms proves the signal is usable for a
// flapping condition: each degrade/recover cycle reports exactly once, so an
// operator can tell a condition that recurred from one that never cleared.
func TestRecoveryIsReportedOnceAndReArms(t *testing.T) {
	h := &capture{}
	logger := slog.New(h)
	var c Condition

	for cycle := 0; cycle < 3; cycle++ {
		c.Enter(logger, "broken")
		c.Enter(logger, "broken") // repeats within a cycle are silent
		c.Clear(logger, "recovered")
		c.Clear(logger, "recovered")
	}

	if got := h.count(slog.LevelWarn, "broken"); got != 3 {
		t.Errorf("WARN lines = %d, want 3 (one per degrade cycle)", got)
	}
	if got := h.count(slog.LevelInfo, "recovered"); got != 3 {
		t.Errorf("recovery lines = %d, want 3 (one per recovery)", got)
	}
	if c.Active() {
		t.Error("Active() = true after final recovery")
	}
}

// TestClearOnHealthyIsSilent matters because Clear is called on every success
// path; if it logged unconditionally it would become the loudest line in the
// service.
func TestClearOnHealthyIsSilent(t *testing.T) {
	h := &capture{}
	logger := slog.New(h)
	var c Condition

	for i := 0; i < 100; i++ {
		c.Clear(logger, "recovered")
	}
	if got := h.count(slog.LevelInfo, "recovered"); got != 0 {
		t.Errorf("recovery lines = %d on a never-degraded condition, want 0", got)
	}
}

func TestConcurrentEnterReportsOnce(t *testing.T) {
	h := &capture{}
	logger := slog.New(h)
	var c Condition

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Enter(logger, "broken")
		}()
	}
	wg.Wait()

	if got := h.count(slog.LevelWarn, "broken"); got != 1 {
		t.Errorf("WARN lines = %d from 64 concurrent callers, want exactly 1", got)
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	var c Condition
	c.Enter(nil, "broken")
	if !c.Active() {
		t.Error("state must still be tracked without a logger")
	}
	c.Clear(nil, "recovered")
	if c.Active() {
		t.Error("Clear must still clear state without a logger")
	}
}
