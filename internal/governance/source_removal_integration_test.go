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
		Platform:   semconfig.PlatformConfig{Org: "acme", ID: "test"},
		Components: map[string]types.ComponentConfig{},
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

// TestIntegration_SourceRemovalRoundTrip drives add → status → remove →
// status over real NATS and pins the removal-integrity contract: removal is
// observable (the source leaves status), late reports cannot resurrect a
// phantom, and unknown handles are NOT_FOUND (the audit observed removed:true
// for anything and phantom "watching" entries at a 20-minute horizon).
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
	disc, err := sourcemanifest.NewComponent(rawCfg, component.Dependencies{NATSClient: tc.Client})
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
	t.Cleanup(func() { _ = manifest.Stop(5 * time.Second) })

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

	// 2. Simulate the spawned component's status report; source appears.
	publishStatusReport(t, ctx, tc, handle)
	waitForSourceInStatus(t, ctx, tc, handle, true)

	// 3. Remove it; the source leaves status within the bound.
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
	waitForSourceInStatus(t, ctx, tc, handle, false)

	// 4. A late in-flight report must not resurrect the phantom.
	publishStatusReport(t, ctx, tc, handle)
	time.Sleep(300 * time.Millisecond)
	waitForSourceInStatus(t, ctx, tc, handle, false)

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
