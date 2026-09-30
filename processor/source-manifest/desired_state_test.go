package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

type desiredStore struct {
	components   semconfig.ComponentConfigs
	failManifest bool
}

func (s *desiredStore) GetConfig() *semconfig.SafeConfig {
	return semconfig.NewSafeConfig(&semconfig.Config{Components: s.components})
}
func (s *desiredStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	if name == "source-manifest" && s.failManifest {
		return errors.New("manifest persistence failed")
	}
	s.components[name] = cfg
	return nil
}
func (s *desiredStore) DeleteComponentFromKV(_ context.Context, name string) error {
	delete(s.components, name)
	return nil
}
func newDesiredStore(t *testing.T) *desiredStore {
	t.Helper()
	raw, err := json.Marshal(Config{Namespace: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	return &desiredStore{components: semconfig.ComponentConfigs{"source-manifest": {Name: "source-manifest", Enabled: true, Config: raw}}}
}

func TestSourceChangesPersistNextBootManifestWithoutChangingRuntime(t *testing.T) {
	store := newDesiredStore(t)
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	reply := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/docs"}}}, cfg)
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if !reply.RestartRequired || reply.RuntimeChanged || !reply.DesiredChanged {
		t.Fatalf("dishonest activation receipt: %+v", reply)
	}
	if len(c.manifestSources) != 0 {
		t.Fatal("add changed running source manifest")
	}
	var desired Config
	if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Sources) != 1 || desired.ExpectedSourceCount != 1 {
		t.Fatalf("next boot manifest = %+v", desired)
	}
	removed := c.removeSource(context.Background(), reply.Components[0].InstanceName, "test", cfg)
	if removed.Error != nil || !removed.RestartRequired || removed.RuntimeChanged {
		t.Fatalf("remove receipt: %+v", removed)
	}
	if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Sources) != 0 || desired.ExpectedSourceCount != 0 {
		t.Fatalf("removed next boot manifest = %+v", desired)
	}
}

func TestDesiredManifestFailureReportsCommittedSource(t *testing.T) {
	store := newDesiredStore(t)
	store.failManifest = true
	c := &Component{logger: slog.Default()}
	reply := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/docs"}}}, IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}})
	if reply.Error == nil || len(reply.Components) != 1 || !reply.RestartRequired || !reply.DesiredChanged {
		t.Fatalf("partial persistence receipt = %+v", reply)
	}
}

func TestRemoveRepairsPartialManifestWrite(t *testing.T) {
	store := newDesiredStore(t)
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	added := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/docs"}}}, cfg)
	if added.Error != nil {
		t.Fatal(added.Error)
	}
	instance := added.Components[0].InstanceName
	store.failManifest = true
	if first := c.removeSource(context.Background(), instance, "", cfg); first.Error == nil || !first.Removed {
		t.Fatalf("partial remove = %+v", first)
	}
	store.failManifest = false
	if repaired := c.removeSource(context.Background(), instance, "", cfg); repaired.Error != nil || !repaired.RestartRequired {
		t.Fatalf("repair = %+v", repaired)
	}
	var desired Config
	if err := json.Unmarshal(store.components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Sources) != 0 || desired.ExpectedSourceCount != 0 {
		t.Fatalf("stale desired manifest = %+v", desired)
	}
	if unknown := c.removeSource(context.Background(), "url-source-unknown", "", cfg); unknown.Error == nil || unknown.Error.Code != CodeNotFound {
		t.Fatalf("unknown handle = %+v", unknown)
	}
}
