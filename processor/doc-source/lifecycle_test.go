package docsource

import (
	"reflect"
	"testing"
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

// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestRootsForPath(t *testing.T) {
	c := &Component{config: Config{Paths: []string{"/workspace/docs", "/workspace/specs"}}}

	tests := []struct {
		name      string
		path      string
		wantRoots []string
	}{
		{"file under first root", "/workspace/docs/readme.md", []string{"/workspace/docs"}},
		{"nested file under second root", "/workspace/specs/sub/dir/file.md", []string{"/workspace/specs"}},
		{"exact root match", "/workspace/docs", []string{"/workspace/docs"}},
		{"unrelated path", "/elsewhere/file.md", nil},
		// A path that merely shares a string prefix with a configured root
		// (no path separator boundary) must not match.
		{"prefix collision without separator", "/workspace/docs-extra/file.md", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.rootsForPath(tt.path); !reflect.DeepEqual(got, tt.wantRoots) {
				t.Errorf("rootsForPath(%q) = %v, want %v", tt.path, got, tt.wantRoots)
			}
		})
	}
}

// TestRootsForPath_OverlappingRootsAllAnnounced: with roots [root, root/sub],
// a file under root/sub was ingested twice — one document per root — so both
// roots must be swept when it goes away (#293 review).
// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestRootsForPath_OverlappingRootsAllAnnounced(t *testing.T) {
	c := &Component{config: Config{Paths: []string{"/workspace", "/workspace/sub", "/workspace"}}}
	want := []string{"/workspace", "/workspace/sub"}
	if got := c.rootsForPath("/workspace/sub/a.md"); !reflect.DeepEqual(got, want) {
		t.Errorf("rootsForPath = %v, want every containing root once: %v", got, want)
	}
}
