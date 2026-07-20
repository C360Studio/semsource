package docsource

import "testing"

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

func TestRootForPath(t *testing.T) {
	c := &Component{config: Config{Paths: []string{"/workspace/docs", "/workspace/specs"}}}

	tests := []struct {
		name     string
		path     string
		wantRoot string
		wantOK   bool
	}{
		{"file under first root", "/workspace/docs/readme.md", "/workspace/docs", true},
		{"nested file under second root", "/workspace/specs/sub/dir/file.md", "/workspace/specs", true},
		{"exact root match", "/workspace/docs", "/workspace/docs", true},
		{"unrelated path", "/elsewhere/file.md", "", false},
		// A path that merely shares a string prefix with a configured root
		// (no path separator boundary) must not match.
		{"prefix collision without separator", "/workspace/docs-extra/file.md", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, ok := c.rootForPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("rootForPath(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			}
			if ok && root != tt.wantRoot {
				t.Errorf("rootForPath(%q) root = %q, want %q", tt.path, root, tt.wantRoot)
			}
		})
	}
}
