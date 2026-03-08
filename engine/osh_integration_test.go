//go:build integration

package engine_test

import (
	"context"
	"os"
	"os/exec"
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

const oshRepo = "https://github.com/opensensorhub/osh-core.git"
const oshBranch = "master"

// cloneOSH does a shallow clone of the Open Sensor Hub core repo into a temp
// directory. Returns the local path. Skips the test if git is unavailable or
// the network is unreachable.
func cloneOSH(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping OSH clone in short mode")
	}

	dir := filepath.Join(t.TempDir(), "osh-core")
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", oshBranch, oshRepo, dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Skipf("skipping OSH test: git clone failed: %v", err)
	}
	return dir
}

func TestIntegration_OSH_JavaAndMaven(t *testing.T) {
	oshDir := cloneOSH(t)

	emitter := engine.NewLogEmitter(newLogger())
	cfg := &config.Config{
		Namespace: "osh",
		Flow: config.FlowConfig{
			Outputs:      []config.OutputConfig{{Name: "log", Type: "log", Subject: "test"}},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: []config.SourceEntry{
			{Type: "ast", Path: oshDir, Language: "java"},
			{Type: "docs", Paths: []string{oshDir}},
			{Type: "config", Paths: []string{oshDir}},
		},
	}

	norm := normalizer.New(normalizer.Config{Org: "osh"})
	eng := engine.NewEngine(cfg, emitter, newLogger(),
		engine.WithNormalizer(norm),
		engine.WithHeartbeatInterval(30*time.Second),
		engine.WithReseedInterval(60*time.Second),
	)

	eng.RegisterHandler(asthandler.New(newLogger()))
	eng.RegisterHandler(dochandler.New())
	eng.RegisterHandler(cfgfile.New(nil))
	eng.RegisterHandler(githandler.New(githandler.Config{WorkspaceDir: t.TempDir()}))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer eng.Stop()

	events := collectEvents(emitter, 1*time.Second)

	// Collect entity IDs.
	entityIDs := make(map[string]bool)
	for _, ev := range events {
		if ev.Type == federation.EventTypeSEED && ev.Entity.ID != "" {
			entityIDs[ev.Entity.ID] = true
		}
	}
	t.Logf("total events: %d, unique entities: %d", len(events), len(entityIDs))

	if len(entityIDs) == 0 {
		t.Fatal("no entities produced from OSH repo")
	}

	// All IDs should use "osh" org and "semsource" platform.
	for id := range entityIDs {
		parts := strings.SplitN(id, ".", 3)
		if len(parts) < 2 {
			t.Errorf("malformed ID: %q", id)
			continue
		}
		if parts[0] != "osh" {
			t.Errorf("entity %q: org = %q, want 'osh'", id, parts[0])
		}
		if parts[1] != "semsource" {
			t.Errorf("entity %q: platform = %q, want 'semsource'", id, parts[1])
		}
	}

	// Check for Java AST entities.
	var javaCount int
	for id := range entityIDs {
		if strings.Contains(id, ".java.") {
			javaCount++
		}
	}
	t.Logf("Java entities: %d", javaCount)
	if javaCount == 0 {
		t.Error("expected Java AST entities from osh-core (it's a Java project)")
	}

	// Check for Maven config entities (pom.xml).
	var mavenCount int
	for id := range entityIDs {
		if strings.Contains(id, ".config.") {
			mavenCount++
		}
	}
	t.Logf("config entities: %d", mavenCount)
	if mavenCount == 0 {
		t.Error("expected config entities from pom.xml files")
	}

	// Check for doc entities (README.md, etc.).
	var docCount int
	for id := range entityIDs {
		if strings.Contains(id, ".web.") {
			docCount++
		}
	}
	t.Logf("doc entities: %d", docCount)

	// Verify entities have triples.
	var withTriples int
	for _, ev := range events {
		if ev.Type == federation.EventTypeSEED && len(ev.Entity.Triples) > 0 {
			withTriples++
		}
	}
	t.Logf("entities with triples: %d / %d", withTriples, len(entityIDs))
	if withTriples == 0 {
		t.Error("no entities have triples")
	}

	// Spot-check: OSH uses "org.sensorhub" group — look for a known package.
	foundSensorhub := false
	for id := range entityIDs {
		if strings.Contains(id, "sensorhub") {
			foundSensorhub = true
			break
		}
	}
	t.Logf("found sensorhub reference: %v", foundSensorhub)
}
