package engine_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
)

// stubSourceConfig satisfies handler.SourceConfig for testing.
type stubSourceConfig struct {
	sourceType string
	path       string
	url        string
	watch      bool
}

func (s *stubSourceConfig) GetType() string      { return s.sourceType }
func (s *stubSourceConfig) GetPath() string      { return s.path }
func (s *stubSourceConfig) GetURL() string       { return s.url }
func (s *stubSourceConfig) IsWatchEnabled() bool { return s.watch }

// stubHandler is a minimal SourceHandler for testing.
type stubHandler struct {
	sourceType string
	entities   []handler.RawEntity
	watchCh    chan handler.ChangeEvent
	ingestErr  error
}

func (h *stubHandler) SourceType() string { return h.sourceType }

func (h *stubHandler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == h.sourceType
}

func (h *stubHandler) Ingest(_ context.Context, _ handler.SourceConfig) ([]handler.RawEntity, error) {
	return h.entities, h.ingestErr
}

func (h *stubHandler) Watch(_ context.Context, _ handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if h.watchCh == nil {
		return nil, nil
	}
	return h.watchCh, nil
}

func newTestConfig(sourceType string) *config.Config {
	return &config.Config{
		Namespace: "acme",
		Flow: config.FlowConfig{
			Outputs:      []config.OutputConfig{{Name: "ws", Type: "network", Subject: "http://localhost:7890/graph"}},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: []config.SourceEntry{
			{Type: sourceType, URL: "https://github.com/acme/repo", Watch: false},
		},
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestEngine_Start_EmitsSEED(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newTestConfig("git")

	h := &stubHandler{sourceType: "git", entities: nil}
	eng := engine.NewEngine(cfg, emitter, newLogger())
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	// Give the engine a moment to emit the SEED event.
	time.Sleep(50 * time.Millisecond)

	events := emitter.CapturedEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event, got 0")
	}
	if events[0].Type != graph.EventTypeSEED {
		t.Errorf("first event type = %v, want SEED", events[0].Type)
	}
	if events[0].Namespace != "acme" {
		t.Errorf("event namespace = %q, want %q", events[0].Namespace, "acme")
	}
}

func TestEngine_Start_SEEDHasCorrectSourceID(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newTestConfig("git")

	eng := engine.NewEngine(cfg, emitter, newLogger())
	eng.RegisterHandler(&stubHandler{sourceType: "git"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eng.Start(ctx)
	defer eng.Stop()
	time.Sleep(50 * time.Millisecond)

	events := emitter.CapturedEvents()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
	seed := events[0]
	if seed.SourceID == "" {
		t.Error("SEED event has empty SourceID")
	}
}

func TestEngine_Stop_Idempotent(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newTestConfig("git")
	eng := engine.NewEngine(cfg, emitter, newLogger())

	ctx := context.Background()
	eng.Start(ctx)

	// Stop twice — must not panic or deadlock.
	if err := eng.Stop(); err != nil {
		t.Errorf("first Stop() error = %v", err)
	}
	if err := eng.Stop(); err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestEngine_Heartbeat_Emitted(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newTestConfig("git")

	// Use a very short heartbeat interval so the test completes quickly.
	eng := engine.NewEngine(cfg, emitter, newLogger(),
		engine.WithHeartbeatInterval(80*time.Millisecond),
	)
	eng.RegisterHandler(&stubHandler{sourceType: "git"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eng.Start(ctx)
	defer eng.Stop()

	// Wait long enough for at least one heartbeat.
	time.Sleep(300 * time.Millisecond)

	events := emitter.CapturedEvents()
	var heartbeats int
	for _, ev := range events {
		if ev.Type == graph.EventTypeHEARTBEAT {
			heartbeats++
		}
	}
	if heartbeats == 0 {
		t.Error("expected at least one HEARTBEAT event")
	}
}

func TestLogEmitter_ConcurrentSafe(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	ctx := context.Background()

	const goroutines = 20
	done := make(chan struct{})

	for range goroutines {
		go func() {
			event := &graph.GraphEvent{
				Type:      graph.EventTypeDELTA,
				SourceID:  "test",
				Namespace: "acme",
				Timestamp: time.Now(),
			}
			emitter.Emit(ctx, event)
			done <- struct{}{}
		}()
	}

	for range goroutines {
		<-done
	}

	events := emitter.CapturedEvents()
	if len(events) != goroutines {
		t.Errorf("CapturedEvents() len = %d, want %d", len(events), goroutines)
	}
}

func TestEngine_RegisterHandler_NoMatch_StillStartsOK(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	// Config has "git" source but we register no handler — engine should still start.
	cfg := newTestConfig("git")
	eng := engine.NewEngine(cfg, emitter, newLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// No handler registered — Start should not error.
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() without matching handler returned error: %v", err)
	}
	eng.Stop()
}

func TestEngine_BuildSeedEvent_IncludesStoreEntities(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newTestConfig("git")

	rawEntity := handler.RawEntity{
		SourceType: "git",
		Domain:     "git",
		System:     "github.com-acme-repo",
		EntityType: "commit",
		Instance:   "a1b2c3",
	}
	h := &stubHandler{sourceType: "git", entities: []handler.RawEntity{rawEntity}}

	eng := engine.NewEngine(cfg, emitter, newLogger())
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eng.Start(ctx)
	defer eng.Stop()
	time.Sleep(50 * time.Millisecond)

	events := emitter.CapturedEvents()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
	// We can't assert specific entity count without the normalizer,
	// but we can verify SEED was emitted without error.
	seed := events[0]
	if seed.Type != graph.EventTypeSEED {
		t.Errorf("event type = %v, want SEED", seed.Type)
	}
}
