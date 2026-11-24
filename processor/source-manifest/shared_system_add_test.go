package sourcemanifest

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestAddSourceRefusesSharedSystemAndPersistsNothing(t *testing.T) {
	store := newDesiredStore(t)
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	first := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "ast", Path: "/r/a", Project: "svc"}}, cfg)
	if first.Error != nil {
		t.Fatalf("first add: %+v", first.Error)
	}
	manifest := bytes.Clone(store.components["source-manifest"].Config)
	component := bytes.Clone(store.components["ast-source-svc"].Config)

	reply := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "ast", Path: "/r/b", Project: "svc"}}, cfg)
	if reply.Error == nil || reply.Error.Code != CodeValidationFailed {
		t.Fatalf("colliding add reply error = %+v, want %s", reply.Error, CodeValidationFailed)
	}
	if !strings.Contains(reply.Error.Message, "ast-source-svc (root /r/a)") {
		t.Errorf("refusal %q does not name the registered instance", reply.Error.Message)
	}
	if reply.DesiredChanged || len(reply.Components) != 0 {
		t.Errorf("refused add reported a desired change: %+v", reply)
	}
	if !bytes.Equal(store.components["source-manifest"].Config, manifest) {
		t.Error("refused add rewrote the desired manifest")
	}
	if !bytes.Equal(store.components["ast-source-svc"].Config, component) {
		t.Error("refused add replaced the registered component")
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestRemoveDesiredSourceWithoutARunningService(t *testing.T) {
	store := newDesiredStore(t)
	c := &Component{logger: slog.Default()}
	opts := sourcespawn.Options{Org: "acme"}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: opts}
	for _, src := range []config.SourceEntry{
		{Type: "ast", Path: "/r/a", Project: "svc"},
		{Type: "ast", Path: "/r/b", Project: "other"},
	} {
		if reply := c.addSource(context.Background(), AddRequest{Source: src}, cfg); reply.Error != nil {
			t.Fatalf("add %+v: %+v", src, reply.Error)
		}
	}
	// A file source: registered, but no desired manifest source builds it.
	fileOnly := store.components["ast-source-other"]
	fileOnly.Config = bytes.Replace(fileOnly.Config, []byte("/r/b"), []byte("/r/file"), 1)
	store.components["ast-source-file"] = fileOnly

	if err := RemoveDesiredSource(context.Background(), store, opts, "ast-source-svc"); err != nil {
		t.Fatal(err)
	}
	if store.components["ast-source-svc"].Enabled {
		t.Error("the removed instance is still enabled for the next boot")
	}
	desired := desiredOf(t, store)
	if len(desired.Sources) != 1 || desired.Sources[0].Project != "other" || desired.ExpectedSourceCount != 2 {
		t.Errorf("desired manifest after removal = %+v", desired)
	}

	// A second removal finds no manifest source left to remove.
	if err := RemoveDesiredSource(context.Background(), store, opts, "ast-source-svc"); err == nil {
		t.Error("a second removal of an entry the manifest no longer holds was accepted")
	}
	for _, name := range []string{"ast-source-file", "ast-source-unknown"} {
		err := RemoveDesiredSource(context.Background(), store, opts, name)
		if sourcespawn.CodeOf(err) != sourcespawn.CodeNotFound {
			t.Errorf("remove %s: err = %v, want %s", name, err, sourcespawn.CodeNotFound)
		}
	}
	if !store.components["ast-source-file"].Enabled {
		t.Error("removing a file source through the manifest disabled it")
	}
}

func desiredOf(t *testing.T, store *desiredStore) Config {
	t.Helper()
	var desired Config
	if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	return desired
}
