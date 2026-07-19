//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"

	"github.com/c360studio/semsource/config"
	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourcespawn"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/processor/supersession"
)

// TestIntegration_VersionRegistrationToDiff is the version-registration-surface
// acceptance (D4): the audit found the whole version-intelligence chain
// unreachable because no registration surface could set a version. This proof
// drives the REAL registration path — config.SourceEntry with explicit
// project+version through sourcespawn.Build (the builder every surface funnels
// into) — instantiates the RESULTING component configs as real ast-source
// components over a live graph stack, runs the supersession pass, and asserts
// graph.query.versionDiff answers the fixture's known changeset.
func TestIntegration_VersionRegistrationToDiff(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(ctx, tc.Client, nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	mr := metric.NewMetricsRegistry()
	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = q.Stop(5 * time.Second) })

	// Fixture: known diff between 1.9.0 and 1.10.0 —
	// Run changed, Stable unchanged, Gone removed, Fresh added.
	root := t.TempDir()
	write := func(rel, src string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("depA-1.9.0/dep.go", "package dep\n\nfunc Run() int { return 1 }\n\nfunc Stable() int { return 0 }\n\nfunc Gone() {}\n")
	write("depA-1.10.0/dep.go", "package dep\n\nfunc Run() int { return 2 }\n\nfunc Stable() int { return 0 }\n\nfunc Fresh() {}\n")

	// The registration surface proper: SourceEntry → sourcespawn.Build.
	opts := sourcespawn.Options{Org: "acme", WorkspaceDir: root}
	var instanceNames []string
	for _, reg := range []struct{ path, version string }{
		{"depA-1.9.0", "1.9.0"},
		{"depA-1.10.0", "1.10.0"},
	} {
		built, err := sourcespawn.Build(config.SourceEntry{
			Type:     "ast",
			Path:     filepath.Join(root, reg.path),
			Project:  "depA",
			Version:  reg.version,
			Language: "go",
		}, opts)
		if err != nil {
			t.Fatalf("sourcespawn.Build(%s): %v", reg.version, err)
		}
		if len(built) != 1 {
			t.Fatalf("Build yielded %d components, want 1 ast-source", len(built))
		}
		for name, compCfg := range built {
			instanceNames = append(instanceNames, name)
			discovered, err := astsource.NewComponent(compCfg.Config, component.Dependencies{NATSClient: tc.Client})
			if err != nil {
				t.Fatalf("ast-source NewComponent from registration config: %v", err)
			}
			comp := discovered.(*astsource.Component)
			if err := comp.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			if err := comp.Start(ctx); err != nil {
				t.Fatalf("Start: %v", err)
			}
			t.Cleanup(func() { _ = comp.Stop(5 * time.Second) })
		}
	}
	if instanceNames[0] == instanceNames[1] {
		t.Fatalf("versioned registrations collided on instance name %q", instanceNames[0])
	}

	// Supersession component serves graph.query.versionDiff and the pass.
	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdiscovered, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdiscovered.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = scomp.Stop(5 * time.Second) })

	// Poll the diff until the asynchronously-indexed entities are all visible.
	// Expected over REAL parse output: Fresh added, Gone removed, Run changed
	// PLUS the dep.go file entity changed (its content hash differs across
	// versions — a real change, correctly counted), Stable unchanged.
	var resp supersession.VersionDiffResponse
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp = requestDiff(t, ctx, tc.Client, "depA", "1.9.0", "1.10.0")
		c := resp.Counts
		if c.Added == 1 && c.Removed == 1 && c.Changed == 2 && c.Unchanged >= 1 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	c := resp.Counts
	if c.Added != 1 || c.Removed != 1 || c.Changed != 2 || c.Unchanged < 1 {
		t.Fatalf("diff counts = %+v, want added1 removed1 changed2 unchanged>=1 (registration-built ingest)", c)
	}
	var changed *supersession.Change
	for i := range resp.Changes {
		if resp.Changes[i].Name == "Run" && resp.Changes[i].Status == "changed" {
			changed = &resp.Changes[i]
		}
	}
	if changed == nil {
		t.Fatalf("no changed entry for Run: %+v", resp.Changes)
	}
	if !strings.Contains(changed.FromBody, "return 1") || !strings.Contains(changed.ToBody, "return 2") {
		t.Errorf("changed bodies not hydrated end-to-end: from=%q to=%q (errors: %v/%v)",
			changed.FromBody, changed.ToBody, changed.FromBodyError, changed.ToBodyError)
	}

	// And the lineage half: the supersession pass relates the two versions.
	summary := runPassAndSummary(t, ctx, tc.Client)
	if summary.Supersedes < 1 {
		t.Errorf("supersession pass produced no lineage edges over registration-built ingest: %+v", summary)
	}
}
