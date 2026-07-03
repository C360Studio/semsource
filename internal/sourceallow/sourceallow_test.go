package sourceallow

import (
	"testing"

	"github.com/c360studio/semsource/config"
)

func TestEnforce(t *testing.T) {
	roots := []string{"/mnt/workspace", "/srv/deps"}
	cases := []struct {
		name    string
		src     config.SourceEntry
		roots   []string
		wantErr bool
	}{
		{"url source ignores allowlist", config.SourceEntry{Type: "git", URL: "https://x/y.git"}, roots, false},
		{"path under root", config.SourceEntry{Type: "git", Path: "/mnt/workspace/repo"}, roots, false},
		{"path equal to root", config.SourceEntry{Type: "git", Path: "/mnt/workspace"}, roots, false},
		{"path under second root", config.SourceEntry{Type: "docs", Paths: []string{"/srv/deps/x"}}, roots, false},
		{"path outside roots", config.SourceEntry{Type: "git", Path: "/etc/passwd"}, roots, true},
		{"traversal escapes root", config.SourceEntry{Type: "git", Path: "/mnt/workspace/../etc"}, roots, true},
		{"sibling-prefix is not under root", config.SourceEntry{Type: "git", Path: "/mnt/workspace-evil"}, roots, true},
		{"path but no roots configured", config.SourceEntry{Type: "git", Path: "/mnt/workspace/repo"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Enforce(tc.src, tc.roots)
			if tc.wantErr != (err != nil) {
				t.Fatalf("Enforce = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
