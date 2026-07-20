package astsource

import (
	"testing"

	"github.com/c360studio/semsource/graph"
)

func TestDeleteTriggersLifecycleRun(t *testing.T) {
	watching := &Component{config: Config{WatchEnabled: true}}
	if !watching.deleteTriggersLifecycleRun() {
		t.Error("a watching component should trigger the lifecycle pass on delete")
	}

	frozen := &Component{config: Config{WatchEnabled: false}}
	if frozen.deleteTriggersLifecycleRun() {
		t.Error("a frozen (watch:false) component must never trigger the lifecycle pass on delete (D5 coherence)")
	}
}

func TestLifecycleRunRequestFor(t *testing.T) {
	pw := &pathWatcher{
		root:         "/workspace/github-com-acme-repo",
		scopedSystem: "github-com-acme-repo",
		config:       WatchPathConfig{Org: "acme", Project: "repo"},
	}

	req := lifecycleRunRequestFor(pw, graph.LifecycleReasonFileDeleted)

	if req.Org != "acme" {
		t.Errorf("Org = %q, want %q", req.Org, "acme")
	}
	if len(req.Systems) != 1 || req.Systems[0] != "github-com-acme-repo" {
		t.Errorf("Systems = %v, want [github-com-acme-repo]", req.Systems)
	}
	if req.RootPath != "/workspace/github-com-acme-repo" {
		t.Errorf("RootPath = %q, want %q", req.RootPath, "/workspace/github-com-acme-repo")
	}
	if req.Reason != graph.LifecycleReasonFileDeleted {
		t.Errorf("Reason = %q, want %q", req.Reason, graph.LifecycleReasonFileDeleted)
	}
}

func TestLifecycleRunRequestFor_SweepReason(t *testing.T) {
	pw := &pathWatcher{root: "/repo", scopedSystem: "sys", config: WatchPathConfig{Org: "acme"}}
	req := lifecycleRunRequestFor(pw, graph.LifecycleReasonPathMissing)
	if req.Reason != graph.LifecycleReasonPathMissing {
		t.Errorf("Reason = %q, want %q", req.Reason, graph.LifecycleReasonPathMissing)
	}
}
