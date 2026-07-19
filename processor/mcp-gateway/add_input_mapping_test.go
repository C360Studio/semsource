package mcpgateway

import (
	"slices"
	"testing"
)

// TestSourceEntryFromAddInput pins the tool-boundary mapping: the flat `path`
// argument must land where each source type's validation reads it (docs/config/
// media validate Paths, not Path), and the polyglot languages list must ride
// through (compose-packaging-hardening D4). Every mapped entry must pass
// SourceEntry.Validate — the exact gate that made add_source{type:docs,path}
// an unconditional VALIDATION_FAILED before this mapping existed.
func TestSourceEntryFromAddInput(t *testing.T) {
	cases := []struct {
		name      string
		in        AddSourceInput
		wantPath  string
		wantPaths []string
	}{
		{
			name:      "docs path maps to plural",
			in:        AddSourceInput{Type: "docs", Path: "/workspace"},
			wantPaths: []string{"/workspace"},
		},
		{
			name:      "config path maps to plural",
			in:        AddSourceInput{Type: "config", Path: "/workspace"},
			wantPaths: []string{"/workspace"},
		},
		{
			name:     "ast path stays singular",
			in:       AddSourceInput{Type: "ast", Path: "/workspace"},
			wantPath: "/workspace",
		},
		{
			name:     "repo path stays singular with languages",
			in:       AddSourceInput{Type: "repo", Path: "/workspace", Languages: []string{"go", "svelte"}},
			wantPath: "/workspace",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := sourceEntryFromAddInput(tc.in)
			if src.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", src.Path, tc.wantPath)
			}
			if !slices.Equal(src.Paths, tc.wantPaths) {
				t.Errorf("Paths = %v, want %v", src.Paths, tc.wantPaths)
			}
			if !slices.Equal(src.Languages, tc.in.Languages) {
				t.Errorf("Languages = %v, want %v", src.Languages, tc.in.Languages)
			}
			if err := src.Validate(); err != nil {
				t.Errorf("mapped entry fails validation: %v", err)
			}
		})
	}
}
