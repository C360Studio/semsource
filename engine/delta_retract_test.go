package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/normalizer"
)

// --- helpers ---

func newWatchConfig(sourceType string) *config.Config {
	return &config.Config{
		Namespace: "acme",
		Flow: config.FlowConfig{
			Outputs:      []config.OutputConfig{{Name: "ws", Type: "network", Subject: "http://localhost:7890/graph"}},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: []config.SourceEntry{
			{Type: sourceType, Path: "/tmp/test", Watch: true},
		},
	}
}

func newNorm() engine.Normalizer {
	return normalizer.New(normalizer.Config{Org: "acme"})
}

// watchHandler is a stub that returns a pre-seeded watch channel.
type watchHandler struct {
	sourceType string
	watchCh    chan handler.ChangeEvent
}

func (h *watchHandler) SourceType() string { return h.sourceType }
func (h *watchHandler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == h.sourceType
}
func (h *watchHandler) Ingest(_ context.Context, _ handler.SourceConfig) ([]handler.RawEntity, error) {
	return nil, nil
}
func (h *watchHandler) Watch(_ context.Context, _ handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	return h.watchCh, nil
}

// waitForEventType polls captured events until an event of the given type appears
// or the deadline is exceeded. Returns the matching event or nil.
func waitForEventType(emitter *engine.LogEmitter, want graph.EventType, deadline time.Duration) *graph.GraphEvent {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		for _, ev := range emitter.CapturedEvents() {
			if ev.Type == want {
				return ev
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

// countEventType counts captured events of the given type.
func countEventType(emitter *engine.LogEmitter, want graph.EventType) int {
	var n int
	for _, ev := range emitter.CapturedEvents() {
		if ev.Type == want {
			n++
		}
	}
	return n
}

// --- DELTA tests ---

func TestEngine_Watch_EmitsDELTA_OnCreate(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 1)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	// Send a create event with a valid entity.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      "/tmp/test/foo.go",
		Entities: []handler.RawEntity{
			{
				SourceType: "ast",
				Domain:     "golang",
				System:     "tmp-test",
				EntityType: "function",
				Instance:   "Foo",
				Properties: map[string]any{"path": "/tmp/test/foo.go"},
			},
		},
	}

	ev := waitForEventType(emitter, graph.EventTypeDELTA, 500*time.Millisecond)
	if ev == nil {
		t.Fatal("no DELTA event emitted after create ChangeEvent")
	}
	if len(ev.Entities) == 0 {
		t.Error("DELTA event has no entities")
	}
	if ev.Namespace != "acme" {
		t.Errorf("DELTA namespace = %q, want %q", ev.Namespace, "acme")
	}
}

func TestEngine_Watch_EmitsDELTA_OnModify(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 2)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	entity := handler.RawEntity{
		SourceType: "ast",
		Domain:     "golang",
		System:     "tmp-test",
		EntityType: "function",
		Instance:   "Bar",
		Properties: map[string]any{"path": "/tmp/test/bar.go"},
	}

	// First event: create.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      "/tmp/test/bar.go",
		Entities:  []handler.RawEntity{entity},
	}
	// Wait for it to land.
	if waitForEventType(emitter, graph.EventTypeDELTA, 300*time.Millisecond) == nil {
		t.Fatal("no DELTA on initial create")
	}

	// Second event: modify with changed property — should produce another DELTA.
	modified := entity
	modified.Properties = map[string]any{"path": "/tmp/test/bar.go", "doc_comment": "updated"}
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationModify,
		Path:      "/tmp/test/bar.go",
		Entities:  []handler.RawEntity{modified},
	}

	time.Sleep(100 * time.Millisecond)
	if countEventType(emitter, graph.EventTypeDELTA) < 2 {
		t.Error("expected at least 2 DELTA events after create + modify")
	}
}

func TestEngine_Watch_NoDELTA_WhenEntityUnchanged(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 2)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	entity := handler.RawEntity{
		SourceType: "ast",
		Domain:     "golang",
		System:     "tmp-test",
		EntityType: "function",
		Instance:   "Baz",
		Properties: map[string]any{"path": "/tmp/test/baz.go"},
	}

	ce := handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      "/tmp/test/baz.go",
		Entities:  []handler.RawEntity{entity},
	}

	// First event: creates entity and emits DELTA.
	watchCh <- ce
	if waitForEventType(emitter, graph.EventTypeDELTA, 300*time.Millisecond) == nil {
		t.Fatal("no DELTA on initial create")
	}
	deltaCount := countEventType(emitter, graph.EventTypeDELTA)

	// Second identical event: entity is unchanged — store dedup prevents DELTA.
	watchCh <- ce
	time.Sleep(100 * time.Millisecond)

	if countEventType(emitter, graph.EventTypeDELTA) != deltaCount {
		t.Error("DELTA emitted for unchanged entity — store dedup failed")
	}
}

func TestEngine_Watch_DELTA_EntityHasCorrectID(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 1)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      "/tmp/test/qux.go",
		Entities: []handler.RawEntity{
			{
				SourceType: "ast",
				Domain:     "golang",
				System:     "tmp-test",
				EntityType: "function",
				Instance:   "Qux",
				Properties: map[string]any{"path": "/tmp/test/qux.go"},
			},
		},
	}

	ev := waitForEventType(emitter, graph.EventTypeDELTA, 500*time.Millisecond)
	if ev == nil {
		t.Fatal("no DELTA event")
	}
	if len(ev.Entities) == 0 {
		t.Fatal("DELTA has no entities")
	}
	// ID must follow 6-part scheme: acme.semsource.golang.tmp-test.function.Qux
	wantID := "acme.semsource.golang.tmp-test.function.Qux"
	if ev.Entities[0].ID != wantID {
		t.Errorf("entity ID = %q, want %q", ev.Entities[0].ID, wantID)
	}
}

// --- RETRACT tests ---

func TestEngine_Watch_EmitsRETRACT_OnDelete(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 2)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	const filePath = "/tmp/test/del.go"

	// First create the entity so it exists in the store.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      filePath,
		Entities: []handler.RawEntity{
			{
				SourceType: "ast",
				Domain:     "golang",
				System:     "tmp-test",
				EntityType: "function",
				Instance:   "ToDelete",
				Properties: map[string]any{"path": filePath},
			},
		},
	}
	if waitForEventType(emitter, graph.EventTypeDELTA, 300*time.Millisecond) == nil {
		t.Fatal("no DELTA on initial create")
	}

	// Now delete the file.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationDelete,
		Path:      filePath,
	}

	ev := waitForEventType(emitter, graph.EventTypeRETRACT, 500*time.Millisecond)
	if ev == nil {
		t.Fatal("no RETRACT event emitted after delete ChangeEvent")
	}
	if len(ev.Retractions) == 0 {
		t.Error("RETRACT event has no retraction IDs")
	}
}

func TestEngine_Watch_RETRACT_ContainsEntityID(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 2)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	const filePath = "/tmp/test/retract_id.go"
	const wantID = "acme.semsource.golang.tmp-test.function.RetractMe"

	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      filePath,
		Entities: []handler.RawEntity{
			{
				SourceType: "ast",
				Domain:     "golang",
				System:     "tmp-test",
				EntityType: "function",
				Instance:   "RetractMe",
				Properties: map[string]any{"path": filePath},
			},
		},
	}
	if waitForEventType(emitter, graph.EventTypeDELTA, 300*time.Millisecond) == nil {
		t.Fatal("no DELTA on initial create")
	}

	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationDelete,
		Path:      filePath,
	}

	ev := waitForEventType(emitter, graph.EventTypeRETRACT, 500*time.Millisecond)
	if ev == nil {
		t.Fatal("no RETRACT event")
	}

	var found bool
	for _, id := range ev.Retractions {
		if id == wantID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RETRACT retractions = %v, want to contain %q", ev.Retractions, wantID)
	}
}

func TestEngine_Watch_NoRETRACT_WhenEntityNotInStore(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 1)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	// Delete a path that was never ingested — no RETRACT should be emitted.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationDelete,
		Path:      "/tmp/test/nonexistent.go",
	}

	time.Sleep(100 * time.Millisecond)
	if countEventType(emitter, graph.EventTypeRETRACT) != 0 {
		t.Error("RETRACT emitted for a path that was never in the store")
	}
}

func TestEngine_Watch_NormalizationError_SkipsBatch(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := newWatchConfig("ast")
	norm := newNorm()

	watchCh := make(chan handler.ChangeEvent, 1)
	h := &watchHandler{sourceType: "ast", watchCh: watchCh}

	eng := engine.NewEngine(cfg, emitter, newLogger(), engine.WithNormalizer(norm))
	eng.RegisterHandler(h)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eng.Stop()

	// Entity is missing required fields — normalization will fail.
	watchCh <- handler.ChangeEvent{
		Operation: handler.OperationCreate,
		Path:      "/tmp/test/bad.go",
		Entities: []handler.RawEntity{
			{
				SourceType: "ast",
				// Domain, System, EntityType, Instance all empty — invalid.
			},
		},
	}

	time.Sleep(100 * time.Millisecond)
	if countEventType(emitter, graph.EventTypeDELTA) != 0 {
		t.Error("DELTA emitted despite normalization error — batch should be skipped")
	}
}
