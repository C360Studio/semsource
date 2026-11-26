//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"

	semsourcegraph "github.com/c360studio/semsource/graph"
	cfghandler "github.com/c360studio/semsource/handler/cfgfile"
	"github.com/c360studio/semsource/internal/natstest"
	cfgfilesource "github.com/c360studio/semsource/processor/cfgfile-source"
	"github.com/c360studio/semsource/processor/supersession"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIntegration_CfgfileStaleness proves, over a real graph stack and a real
// cfgfile-source, that deleting a watched manifest marks its module and
// dependency entities stale through cfgfile-source's own trigger, that
// re-creating it clears the marker through the same trigger, and that a dependency dropped from a
// manifest that still exists is retained unmarked (#393).
//
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestIntegration_CfgfileStaleness(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t,
		natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	mr := metric.NewMetricsRegistry()
	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, index.Stop) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, q.Stop) })
	platform := types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}

	// The supersession responder starts first so the seed's trigger is answered.
	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdisc, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client, Platform: platform})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdisc.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, scomp.Stop) })

	root := t.TempDir()
	svcMod := filepath.Join(root, "svc", "go.mod")
	liveMod := filepath.Join(root, "live", "go.mod")
	const svcContent = "module example.com/svc\n\ngo 1.23\n\nrequire github.com/a/b v1.2.0\n"
	writeManifest(t, svcMod, svcContent)
	writeManifest(t, liveMod, "module example.com/live\n\ngo 1.23\n\nrequire (\n\tgithub.com/c/d v1.0.0\n\tgithub.com/e/f v1.0.0\n)\n")

	ccfg, _ := json.Marshal(map[string]any{
		"paths": []string{root}, "org": "acme", "watch_enabled": true, "instance_name": "cfgfile-source-test",
	})
	cdisc, err := cfgfilesource.NewComponent(ccfg, component.Dependencies{NATSClient: tc.Client, Platform: platform})
	if err != nil {
		t.Fatalf("cfgfile-source NewComponent: %v", err)
	}
	ccomp := cdisc.(*cfgfilesource.Component)
	if err := ccomp.Start(ctx); err != nil {
		t.Fatalf("cfgfile-source Start: %v", err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ccomp.Stop) })

	prefix := "acme.test-a1b2c3." + cfghandler.SystemSlug("", root) + ".config"
	byTitle := func() map[string]semgraph.EntityState {
		out := map[string]semgraph.EntityState{}
		for _, e := range prefixAll(ctx, tc.Client, prefix) {
			for _, tr := range e.Triples {
				if tr.Predicate == source.DcTitle {
					if s, ok := tr.Object.(string); ok {
						out[s] = e
					}
				}
			}
		}
		return out
	}
	titles := []string{"example.com/svc", "github.com/a/b", "example.com/live", "github.com/c/d", "github.com/e/f"}
	var seeded map[string]semgraph.EntityState
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		seeded = byTitle()
		if hasAll(seeded, titles) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !hasAll(seeded, titles) {
		t.Fatalf("seed incomplete: got %d of %v", len(seeded), titles)
	}

	// A dependency dropped from a manifest that still exists is retained.
	writeManifest(t, liveMod, "module example.com/live\n\ngo 1.23\n\nrequire github.com/e/f v1.0.0\n")
	waitRequiresAbsent(t, ctx, tc, seeded["example.com/live"].ID, seeded["github.com/c/d"].ID, 20*time.Second)

	// Deleting the watched manifest marks its module and its dependency.
	if err := os.Remove(svcMod); err != nil {
		t.Fatalf("remove svc/go.mod: %v", err)
	}
	for _, title := range []string{"example.com/svc", "github.com/a/b"} {
		waitTriple(t, ctx, tc.Client, seeded[title].ID, source.EntityLifecycleStale, source.LifecycleReasonFileDeleted, 20*time.Second)
	}
	for _, title := range []string{"example.com/live", "github.com/c/d", "github.com/e/f"} {
		if e, ok := fetchEntity(ctx, tc.Client, seeded[title].ID); !ok {
			t.Errorf("%s was not retained", title)
		} else if countObject(e, source.EntityLifecycleStale, source.LifecycleReasonFileDeleted) > 0 {
			t.Errorf("%s of a live manifest was marked stale", title)
		}
	}

	// Re-creating the manifest is a watch create, and cfgfile-source's own
	// pass clears the marker.
	writeManifest(t, svcMod, svcContent)
	for _, title := range []string{"example.com/svc", "github.com/a/b"} {
		waitPredicateAbsent(t, ctx, tc.Client, seeded[title].ID, source.EntityLifecycleStale, 20*time.Second)
	}
}

func writeManifest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasAll(m map[string]semgraph.EntityState, keys []string) bool {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// waitRequiresAbsent polls until module no longer names dep with a requires
// edge, or fails after timeout.
func waitRequiresAbsent(t *testing.T, ctx context.Context, tc *natstest.Server, module, dep string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e, ok := fetchEntity(ctx, tc.Client, module); ok && countObject(e, source.ConfigRequires, dep) == 0 {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("%s still requires %s after %s", module, dep, timeout)
}
