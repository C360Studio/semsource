package config

import "testing"

// TestSourceEntry_Validate_GitAcceptsPathOnly is the regression guard for issue
// #1: a git source may be configured with a url OR a local path. Path-only is
// the sidecar case — index a mounted/agent worktree in place without cloning
// (ADR-0007). Previously git required a url, forcing the clone path.
func TestSourceEntry_Validate_GitAcceptsPathOnly(t *testing.T) {
	cases := []struct {
		name    string
		entry   SourceEntry
		wantErr bool
	}{
		{"git url only", SourceEntry{Type: "git", URL: "https://example.com/x.git"}, false},
		{"git path only (issue #1)", SourceEntry{Type: "git", Path: "/mnt/workspace"}, false},
		{"git url and path", SourceEntry{Type: "git", URL: "https://example.com/x.git", Path: "/mnt/workspace"}, false},
		{"git neither url nor path", SourceEntry{Type: "git"}, true},
		// repo already accepted url-or-path; assert parity so the two stay aligned.
		{"repo path only", SourceEntry{Type: "repo", Path: "/mnt/workspace"}, false},
		{"repo neither", SourceEntry{Type: "repo"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
