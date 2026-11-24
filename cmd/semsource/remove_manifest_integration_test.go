//go:build integration

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// A source persisted before the refusal existed collides with a file source:
// boot refuses and names it, the offline removal takes it out of the desired
// manifest with no service running, and the next boot's set is admitted.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestIntegrationRemoveManifestClearsAPersistedCollision(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", guideSource))
	bootConfigFile(ctx, t, broker.URL, path)

	// Persisted the way add_source records a source, minus today's check: its
	// second root's path-derived system is the file source's "guide".
	persisted := config.SourceEntry{Type: "docs", Paths: []string{"./other", filepath.Join(t.TempDir(), "guide")}}
	opts := sourcespawn.Options{Org: "acme"}
	built, err := sourcespawn.Build(persisted, opts)
	if err != nil {
		t.Fatal(err)
	}
	nc, ssCfg, err := setupNATS(ctx, broker.URL, mustLoad(ctx, t, path), slog.New(&recordedLogs{}), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	configMgr, err := startConfigManager(ctx, ssCfg, nc, slog.New(&recordedLogs{}))
	if err != nil {
		t.Fatal(err)
	}
	var instance string
	for name, cc := range built {
		instance = name
		if err := configMgr.PutComponentToKV(ctx, name, cc); err != nil {
			t.Fatal(err)
		}
	}
	if err := sourcemanifest.RewriteDesiredManifest(ctx, configMgr, opts, []config.SourceEntry{persisted}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := configMgr.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := nc.Close(ctx); err != nil {
		t.Fatal(err)
	}

	refused := checkBootCollisions(bootConfigFile(ctx, t, broker.URL, path).effective.Components)
	if refused == nil || !strings.Contains(refused.Error(), instance) || !strings.Contains(refused.Error(), "doc-source-guide") {
		t.Fatalf("boot over the persisted collision: err = %v, want a refusal naming %s and doc-source-guide", refused, instance)
	}

	if err := removeManifestCmd(ctx, path, broker.URL, instance); err != nil {
		t.Fatal(err)
	}

	after := bootConfigFile(ctx, t, broker.URL, path).effective.Components
	if err := checkBootCollisions(after); err != nil {
		t.Fatalf("boot after the offline removal still refused: %v", err)
	}
	if after[instance].Enabled {
		t.Errorf("%s is still enabled for the next boot", instance)
	}
	var desired sourcemanifest.Config
	if err := json.Unmarshal(after["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	for _, src := range desired.Sources {
		if len(src.Paths) > 0 && src.Paths[0] == "./other" {
			t.Errorf("the desired manifest still holds the removed source: %+v", desired.Sources)
		}
	}
	if _, ok := after["doc-source-guide"]; !ok || !after["doc-source-guide"].Enabled {
		t.Error("the file source did not survive the offline removal")
	}
}
