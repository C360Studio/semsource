//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"

	semsourceconfig "github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// memConfigStore is a stateful sourcespawn.ConfigStore: puts register
// components, deletes deregister them — the seam the removal contract runs
// through (real component spawn/despawn is the framework's KV watch, proven
// elsewhere).
type memConfigStore struct {
	mu  sync.Mutex
	cfg *semconfig.SafeConfig
}

func newMemConfigStore() *memConfigStore {
	return &memConfigStore{cfg: semconfig.NewSafeConfig(&semconfig.Config{
		Platform: semconfig.PlatformConfig{Org: "acme", ID: "test"},
		Components: map[string]types.ComponentConfig{"source-manifest": {
			Name: "source-manifest", Type: types.ComponentTypeProcessor, Enabled: true,
			Config: json.RawMessage(`{"namespace":"acme","sources":[],"expected_source_count":0}`),
		}},
		// A spawned source declares a graph.ingest output port, which makes
		// config validation resolve the GRAPH stream and — since semstreams
		// beta.159 — demand its bounds. Production supplies this from
		// cmd/semsource's graphStreamConfig; without it the fixture validates a
		// config shape that cannot exist at runtime, so Add fails for a reason
		// no real deployment would hit.
		Streams: semconfig.StreamConfigs{
			"GRAPH": semconfig.StreamConfig{
				Subjects: []string{
					"graph.ingest.entity",
					"graph.ingest.batch",
					"graph.ingest.manifest",
					"graph.ingest.status",
					"graph.ingest.predicates",
				},
				Storage:  "memory",
				MaxBytes: 256 * 1024 * 1024,
				MaxAge:   "1h",
				Replicas: 1,
				Discard:  semconfig.StreamDiscardNew,
			},
		},
	})}
}

func (m *memConfigStore) GetConfig() *semconfig.SafeConfig { return m.cfg }

func (m *memConfigStore) PutComponentToKV(_ context.Context, name string, compConfig types.ComponentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Mutate(func(c *semconfig.Config) error {
		c.Components[name] = compConfig
		return nil
	})
}

func (m *memConfigStore) DeleteComponentFromKV(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Mutate(func(c *semconfig.Config) error {
		delete(c.Components, name)
		return nil
	})
}

// TestIntegration_SourceRemovalRoundTrip proves the boot-only desired-state contract
// over real NATS: add/remove persists the next boot's manifest while the running
// source remains visible until restart. The process restart is covered by E2E.
// source_removed lifecycle replay is a separate migration merge blocker.
func TestIntegration_SourceRemovalRoundTrip(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity", "graph.ingest.manifest", "graph.ingest.status", "graph.ingest.predicates"},
		}),
	)

	rawCfg, err := json.Marshal(sourcemanifest.Config{
		Namespace:           "acme",
		Sources:             []sourcemanifest.ManifestSource{},
		ExpectedSourceCount: 0,
		Ports:               sourcemanifest.DefaultConfig().Ports,
	})
	if err != nil {
		t.Fatal(err)
	}
	disc, err := sourcemanifest.NewComponent(rawCfg, component.Dependencies{NATSClient: tc.Client, Platform: component.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	manifest := disc.(*sourcemanifest.Component)
	if err := manifest.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := manifest.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, manifest.Stop) })

	store := newMemConfigStore()
	if err := manifest.RegisterIngestHandlers(ctx, sourcemanifest.IngestHandlerConfig{
		Namespace: "acme",
		Store:     store,
		Spawn:     sourcespawn.Options{Org: "acme", WorkspaceDir: t.TempDir()},
	}); err != nil {
		t.Fatalf("RegisterIngestHandlers: %v", err)
	}

	// 1. Add a docs source over NATS.
	addReq, _ := json.Marshal(sourcemanifest.AddRequest{
		Source:     manifestSourceEntryForDocs(t),
		Provenance: sourcemanifest.Provenance{Actor: "removal-test"},
	})
	addRaw, err := tc.Client.Request(ctx, "graph.ingest.add.acme", addReq, 5*time.Second)
	if err != nil {
		t.Fatalf("add request: %v", err)
	}
	var addReply sourcemanifest.AddReply
	if err := json.Unmarshal(addRaw, &addReply); err != nil {
		t.Fatalf("decode add reply: %v", err)
	}
	if addReply.Error != nil || len(addReply.Components) == 0 {
		t.Fatalf("add failed: %+v", addReply)
	}
	handle := addReply.Components[0].InstanceName
	if !addReply.DesiredChanged || addReply.RuntimeChanged || !addReply.RestartRequired {
		t.Fatalf("add must report desired-only change: %+v", addReply)
	}

	// 2. Simulate the spawned component's status report; source appears.
	publishStatusReport(t, ctx, tc, handle)
	waitForSourceInStatus(t, ctx, tc, handle, true)

	// 3. Remove desired registration; the running source remains until restart.
	removeReq, _ := json.Marshal(sourcemanifest.RemoveRequest{InstanceName: handle})
	removeRaw, err := tc.Client.Request(ctx, "graph.ingest.remove.acme", removeReq, 5*time.Second)
	if err != nil {
		t.Fatalf("remove request: %v", err)
	}
	var removeReply sourcemanifest.RemoveReply
	if err := json.Unmarshal(removeRaw, &removeReply); err != nil {
		t.Fatalf("decode remove reply: %v", err)
	}
	if !removeReply.Removed || removeReply.Error != nil {
		t.Fatalf("remove failed: %+v", removeReply)
	}
	if !removeReply.DesiredChanged || removeReply.RuntimeChanged || !removeReply.RestartRequired {
		t.Fatalf("remove must report desired-only change: %+v", removeReply)
	}
	var desired sourcemanifest.Config
	if err := json.Unmarshal(store.GetConfig().Get().Components["source-manifest"].Config, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Sources) != 0 || desired.ExpectedSourceCount != 0 {
		t.Fatalf("next-boot manifest retained removed source: %+v", desired)
	}
	waitForSourceInStatus(t, ctx, tc, handle, true)

	// 4. Running producer reports remain valid until restart.
	publishStatusReport(t, ctx, tc, handle)
	waitForSourceInStatus(t, ctx, tc, handle, true)

	// 5. Removing an unknown handle is NOT_FOUND, never removed:true.
	unknownReq, _ := json.Marshal(sourcemanifest.RemoveRequest{InstanceName: "no-such-source"})
	unknownRaw, err := tc.Client.Request(ctx, "graph.ingest.remove.acme", unknownReq, 5*time.Second)
	if err != nil {
		t.Fatalf("unknown remove request: %v", err)
	}
	var unknownReply sourcemanifest.RemoveReply
	if err := json.Unmarshal(unknownRaw, &unknownReply); err != nil {
		t.Fatalf("decode unknown remove reply: %v", err)
	}
	if unknownReply.Removed || unknownReply.Error == nil || unknownReply.Error.Code != sourcemanifest.CodeNotFound {
		t.Fatalf("unknown handle reply = %+v, want NOT_FOUND", unknownReply)
	}
}

func manifestSourceEntryForDocs(t *testing.T) semsourceconfig.SourceEntry {
	t.Helper()
	return semsourceconfig.SourceEntry{Type: "docs", Paths: []string{t.TempDir()}}
}

func publishStatusReport(t *testing.T, ctx context.Context, tc *natsclient.TestClient, instance string) {
	t.Helper()
	report, _ := json.Marshal(sourcemanifest.SourceStatusReport{
		InstanceName: instance,
		SourceType:   "docs",
		Phase:        sourcemanifest.SourcePhaseWatching,
		EntityCount:  1,
		Timestamp:    time.Now(),
	})
	if err := tc.Client.Publish(ctx, "semsource.internal.status", report); err != nil {
		t.Fatalf("publish status report: %v", err)
	}
}

func waitForSourceInStatus(t *testing.T, ctx context.Context, tc *natsclient.TestClient, instance string, want bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		raw, err := tc.Client.Request(ctx, "graph.query.status", nil, 2*time.Second)
		if err == nil {
			last = string(raw)
			if strings.Contains(last, instance) == want {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("source %q presence in status never became %v; last status: %s", instance, want, last)
}
