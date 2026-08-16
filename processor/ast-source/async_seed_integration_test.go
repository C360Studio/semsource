//go:build integration

package astsource

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// startedComponent builds and starts an ast-source against a real test NATS,
// pointed at watchRoot.
func startedComponent(t *testing.T, watchRoot string) (*Component, func()) {
	t.Helper()
	tc := natsclient.NewTestClient(t,
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity", "graph.ingest.batch"},
		}),
	)

	raw, err := json.Marshal(map[string]any{
		"instance_name": "ast-source-test",
		"watch_paths": []map[string]any{{
			"path":      watchRoot,
			"org":       "acme",
			"project":   "test",
			"languages": []string{"go"},
		}},
		"watch_enabled": false,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	comp, err := NewComponent(raw, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := comp.(*Component)
	return c, func() { _ = stopWithin(5*time.Second, c.Stop) }
}

// TestIntegration_StartReturnsWhilePathsUnavailable is the whole point of the
// change. The watch path does not exist, so the path-availability retry
// (~30 attempts) is certainly still running when Start returns — no timing race
// is needed to know the seed has not finished.
//
// Before this change that retry ran INSIDE Start, which held the framework's
// component-start barrier and prevented the HTTP status and metrics listeners
// from binding at all (semstreams#867) — the surfaces stayed dark for the whole
// window in which an operator would want them.
func TestIntegration_StartReturnsWhilePathsUnavailable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist-yet")
	c, cleanup := startedComponent(t, missing)
	defer cleanup()

	start := time.Now()
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v; an unavailable path must not fail the start", err)
	}
	elapsed := time.Since(start)

	// retry.Persistent backs off to ~10s per attempt over ~30 attempts, so a
	// synchronous seed could not possibly have returned this fast.
	if elapsed > 3*time.Second {
		t.Errorf("Start() took %s; it must return without waiting for the seed", elapsed)
	}

	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()
	if !running {
		t.Error("component is not marked running after Start returned")
	}

	if phase := c.currentPhase(); phase != "ingesting" {
		t.Errorf("phase = %q while the seed is still retrying, want %q", phase, "ingesting")
	}
}

// TestIntegration_StopDuringInFlightSeedWaits pins the shutdown ordering:
// publisher.Stop() closes the buffer, so Stop must cancel the seed AND wait for
// it, or seed work can publish into a closed publisher.
func TestIntegration_StopDuringInFlightSeedWaits(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist-yet")
	c, _ := startedComponent(t, missing)

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !c.seed.Running() {
		t.Fatal("seed already finished; the test would not exercise stop-during-seed")
	}

	if err := stopWithin(5*time.Second, c.Stop); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if c.seed.Running() {
		t.Error("Stop() returned while the seed goroutine was still running")
	}
}
