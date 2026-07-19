package sourcemanifest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/internal/sourcespawn"
)

// TestStatusAggregator_RemoveForgets pins the phantom-source fix: a removed
// source leaves status entirely (the audit observed removed sources reporting
// as "watching" with stale counts at a 20-minute horizon).
func TestStatusAggregator_RemoveForgets(t *testing.T) {
	agg := newStatusAggregator(2)
	agg.update(report("keep", SourcePhaseWatching))
	agg.update(report("gone", SourcePhaseWatching))

	if !agg.remove("gone") {
		t.Fatal("remove(gone) = false, want true")
	}
	status := agg.buildStatus("acme")
	if len(status.Sources) != 1 || status.Sources[0].InstanceName != "keep" {
		t.Fatalf("sources = %+v, want only 'keep'", status.Sources)
	}
	// Expected count decremented with the removal: the survivor alone is a
	// complete, ready set.
	if status.Phase != PhaseReady {
		t.Errorf("phase = %q after removal with survivor seeded, want %q", status.Phase, PhaseReady)
	}
	if agg.remove("gone") {
		t.Error("second remove(gone) = true, want false")
	}
}

// TestComponent_RemovedSourceReportIgnored pins the resurrection race: an
// in-flight periodic report from a deregistered component must not re-add a
// phantom entry.
func TestComponent_RemovedSourceReportIgnored(t *testing.T) {
	c := slowSeedComponent(t)
	ctx := context.Background()

	c.handleStatusReport(ctx, seedReport("ast-source-a", SourcePhaseWatching))
	c.handleStatusReport(ctx, seedReport("doc-source-b", SourcePhaseWatching))
	c.dropSourceStatus(ctx, "doc-source-b")

	// The tearing-down component's last report arrives late.
	c.handleStatusReport(ctx, seedReport("doc-source-b", SourcePhaseWatching))

	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()
	var payload StatusPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	for _, s := range payload.Sources {
		if s.InstanceName == "doc-source-b" {
			t.Fatalf("removed source resurrected in status: %+v", payload.Sources)
		}
	}
}

// TestRemoveManifestSourceByInstance_SiblingsKeepEntry pins instance-scoped
// manifest bookkeeping: removing one instance of an expanded repo keeps the
// entry while siblings remain registered (the audit found whole-repo erasure).
func TestRemoveManifestSourceByInstance_SiblingsKeepEntry(t *testing.T) {
	entry := ManifestSource{Type: "repo", URL: "https://github.com/acme/app", Branch: "main"}
	opts := sourcespawn.Options{Org: "acme", WorkspaceDir: t.TempDir()}
	built, err := sourcespawn.Build(manifestSourceToSourceEntry(entry), opts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(built) < 2 {
		t.Fatalf("repo expansion yielded %d instances, need >= 2 for this test", len(built))
	}
	var removed, sibling string
	for name := range built {
		if removed == "" {
			removed = name
		} else if sibling == "" {
			sibling = name
		}
	}

	c := &Component{config: Config{Namespace: "acme"}}
	c.manifestSources = []ManifestSource{entry}

	// Sibling still registered → entry stays.
	if c.removeManifestSourceByInstance(removed, opts, stubStore{components: []string{sibling}}) {
		t.Fatal("entry dropped while a sibling instance is still registered")
	}
	if len(c.manifestSources) != 1 {
		t.Fatalf("manifestSources len = %d, want 1", len(c.manifestSources))
	}

	// No siblings left → entry goes.
	if !c.removeManifestSourceByInstance(removed, opts, stubStore{}) {
		t.Fatal("entry kept after the last sibling was removed")
	}
	if len(c.manifestSources) != 0 {
		t.Fatalf("manifestSources len = %d, want 0", len(c.manifestSources))
	}
}
