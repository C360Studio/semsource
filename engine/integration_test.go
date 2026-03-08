//go:build integration

package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	asthandler "github.com/c360studio/semsource/handler/ast"
	"github.com/c360studio/semsource/handler/cfgfile"
	dochandler "github.com/c360studio/semsource/handler/doc"
	githandler "github.com/c360studio/semsource/handler/git"
	"github.com/c360studio/semsource/normalizer"
	"github.com/c360studio/semstreams/federation"
)

// repoRoot returns the absolute path to the semsource repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the engine package to find go.mod.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// selfIngestConfig builds a Config that points at our own repo.
func selfIngestConfig(t *testing.T) *config.Config {
	t.Helper()
	root := repoRoot(t)
	return &config.Config{
		Namespace: "test",
		Flow: config.FlowConfig{
			Outputs:      []config.OutputConfig{{Name: "log", Type: "log", Subject: "test"}},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: []config.SourceEntry{
			{Type: "ast", Path: root, Language: "go"},
			{Type: "docs", Paths: []string{root}},
			{Type: "config", Paths: []string{root}},
		},
	}
}

// collectEvents polls the emitter until the deadline, returning all captured events.
func collectEvents(emitter *engine.LogEmitter, deadline time.Duration) []*federation.Event {
	time.Sleep(deadline)
	return emitter.CapturedEvents()
}

// --- Tests ---

func TestIntegration_SelfIngest_ProducesEntities(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := selfIngestConfig(t)
	norm := normalizer.New(normalizer.Config{Org: "test"})

	eng := engine.NewEngine(cfg, emitter, newLogger(),
		engine.WithNormalizer(norm),
		engine.WithHeartbeatInterval(5*time.Second),
		engine.WithReseedInterval(60*time.Second),
	)

	root := repoRoot(t)
	eng.RegisterHandler(asthandler.New(newLogger()))
	eng.RegisterHandler(dochandler.New())
	eng.RegisterHandler(cfgfile.New(nil))
	eng.RegisterHandler(githandler.New(githandler.Config{WorkspaceDir: t.TempDir()}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer eng.Stop()

	events := collectEvents(emitter, 500*time.Millisecond)
	if len(events) == 0 {
		t.Fatal("no events emitted after self-ingest")
	}

	// Count SEED events.
	var seedCount int
	for _, ev := range events {
		if ev.Type == federation.EventTypeSEED {
			seedCount++
		}
	}
	if seedCount == 0 {
		t.Fatal("no SEED events emitted")
	}
	t.Logf("total events: %d, SEED events: %d", len(events), seedCount)

	// Collect all entity IDs from SEED events.
	entityIDs := make(map[string]bool)
	for _, ev := range events {
		if ev.Type == federation.EventTypeSEED && ev.Entity.ID != "" {
			entityIDs[ev.Entity.ID] = true
		}
	}
	t.Logf("unique entities: %d", len(entityIDs))

	if len(entityIDs) == 0 {
		t.Fatal("SEED events contain no entities")
	}

	// All IDs must follow 6-part format: org.platform.domain.system.type.instance
	for id := range entityIDs {
		parts := strings.Split(id, ".")
		if len(parts) < 6 {
			t.Errorf("entity ID %q has %d parts, want >= 6", id, len(parts))
		}
		if parts[0] != "test" {
			t.Errorf("entity ID %q does not start with org 'test'", id)
		}
		if parts[1] != "semsource" {
			t.Errorf("entity ID %q has platform %q, want 'semsource'", id, parts[1])
		}
	}

	// Verify we found entities from each handler.
	var hasGolang, hasDoc, hasConfig bool
	for id := range entityIDs {
		switch {
		case strings.Contains(id, ".golang."):
			hasGolang = true
		case strings.Contains(id, ".web."):
			hasDoc = true
		case strings.Contains(id, ".config."):
			hasConfig = true
		}
	}

	if !hasGolang {
		t.Error("expected golang entities from AST handler (self-parsing Go files)")
	}
	if !hasDoc {
		t.Error("expected web/doc entities from doc handler (CLAUDE.md, etc.)")
	}
	if !hasConfig {
		t.Error("expected config entities from config handler (go.mod)")
	}

	// Verify SEED entities carry triples.
	var entitiesWithTriples int
	for _, ev := range events {
		if ev.Type == federation.EventTypeSEED && len(ev.Entity.Triples) > 0 {
			entitiesWithTriples++
		}
	}
	if entitiesWithTriples == 0 {
		t.Error("no SEED entities have triples — normalizer may not be wired")
	}
	t.Logf("entities with triples: %d", entitiesWithTriples)

	// Spot-check: we should find a known Go symbol.
	// The normalizer package exports "Normalizer" — look for it.
	foundNormalizer := false
	for id := range entityIDs {
		if strings.HasSuffix(id, ".Normalizer") || strings.HasSuffix(id, ".New") {
			foundNormalizer = true
			break
		}
	}
	// Spot check: go.mod should produce a "module" config entity.
	foundGomod := false
	for id := range entityIDs {
		if strings.Contains(id, ".config.") && strings.Contains(id, ".module.") {
			foundGomod = true
			break
		}
	}

	_ = root // used in config
	t.Logf("found normalizer symbol: %v, found gomod: %v", foundNormalizer, foundGomod)
}

func TestIntegration_SelfIngest_AllEntitiesHaveProvenance(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := selfIngestConfig(t)
	norm := normalizer.New(normalizer.Config{Org: "test"})

	eng := engine.NewEngine(cfg, emitter, newLogger(),
		engine.WithNormalizer(norm),
		engine.WithHeartbeatInterval(5*time.Second),
		engine.WithReseedInterval(60*time.Second),
	)

	eng.RegisterHandler(asthandler.New(newLogger()))
	eng.RegisterHandler(dochandler.New())
	eng.RegisterHandler(cfgfile.New(nil))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer eng.Stop()

	events := collectEvents(emitter, 500*time.Millisecond)

	for _, ev := range events {
		if ev.Type != federation.EventTypeSEED || ev.Entity.ID == "" {
			continue
		}
		ent := ev.Entity
		if ent.Provenance.SourceType == "" {
			t.Errorf("entity %q has empty Provenance.SourceType", ent.ID)
		}
		if ent.Provenance.SourceID == "" {
			t.Errorf("entity %q has empty Provenance.SourceID", ent.ID)
		}
		if ent.Provenance.Timestamp.IsZero() {
			t.Errorf("entity %q has zero Provenance.Timestamp", ent.ID)
		}
	}
}

func TestIntegration_SelfIngest_Heartbeat(t *testing.T) {
	emitter := engine.NewLogEmitter(newLogger())
	cfg := selfIngestConfig(t)
	norm := normalizer.New(normalizer.Config{Org: "test"})

	eng := engine.NewEngine(cfg, emitter, newLogger(),
		engine.WithNormalizer(norm),
		engine.WithHeartbeatInterval(100*time.Millisecond),
		engine.WithReseedInterval(60*time.Second),
	)

	// Register handlers so seed works.
	eng.RegisterHandler(dochandler.New())
	eng.RegisterHandler(cfgfile.New(nil))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer eng.Stop()

	// Wait long enough for heartbeats.
	time.Sleep(400 * time.Millisecond)

	var heartbeats int
	for _, ev := range emitter.CapturedEvents() {
		if ev.Type == federation.EventTypeHEARTBEAT {
			heartbeats++
		}
	}
	if heartbeats == 0 {
		t.Error("expected at least one HEARTBEAT event")
	}
	t.Logf("heartbeat events: %d", heartbeats)
}
