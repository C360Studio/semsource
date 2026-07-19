package config

import (
	"slices"
	"testing"
)

// TestSourceEntry_LanguagesPrecedence pins compose-packaging-hardening D4: the
// plural languages field wins, the singular stays honored alone, and a
// contradictory pair is rejected rather than silently dropping intent.
func TestSourceEntry_LanguagesPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		entry    SourceEntry
		want     []string
		validErr bool
	}{
		{
			name:  "plural wins",
			entry: SourceEntry{Type: "ast", Path: "/w", Language: "go", Languages: []string{"go", "python"}},
			want:  []string{"go", "python"},
		},
		{
			name:  "singular alone honored",
			entry: SourceEntry{Type: "ast", Path: "/w", Language: "java"},
			want:  []string{"java"},
		},
		{
			name:  "neither set yields nil (spawner default)",
			entry: SourceEntry{Type: "ast", Path: "/w"},
			want:  nil,
		},
		{
			name:     "contradiction rejected",
			entry:    SourceEntry{Type: "ast", Path: "/w", Language: "go", Languages: []string{"python"}},
			validErr: true,
		},
		{
			name:     "empty language string rejected",
			entry:    SourceEntry{Type: "ast", Path: "/w", Languages: []string{"go", ""}},
			validErr: true,
		},
		{
			name:  "repo entries carry languages too",
			entry: SourceEntry{Type: "repo", URL: "https://github.com/acme/app", Languages: []string{"go", "svelte"}},
			want:  []string{"go", "svelte"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if tc.validErr {
				if err == nil {
					t.Fatal("Validate() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if got := tc.entry.EffectiveLanguages(); !slices.Equal(got, tc.want) {
				t.Errorf("EffectiveLanguages() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestExpandRepoSources_LanguagesPropagation pins the add_source parity half of
// D4: a polyglot repo entry's languages reach the expanded ast entry, so the
// curator path gets the same coverage as the local mount.
func TestExpandRepoSources_LanguagesPropagation(t *testing.T) {
	entries := expandSingleBranch(SourceEntry{
		Type:      "repo",
		URL:       "https://github.com/example/poly",
		Branch:    "main",
		Languages: []string{"go", "typescript"},
	}, t.TempDir())

	var ast *SourceEntry
	for i := range entries {
		if entries[i].Type == "ast" {
			ast = &entries[i]
		}
	}
	if ast == nil {
		t.Fatalf("no ast entry in expansion: %+v", entries)
	}
	if !slices.Equal(ast.Languages, []string{"go", "typescript"}) {
		t.Errorf("ast.Languages = %v, want propagation", ast.Languages)
	}
}

// TestExpandRepoSources_VersionPropagation pins D1's expansion half: a repo
// entry's explicit version reaches the expanded ast entry (never invented —
// absent stays absent).
func TestExpandRepoSources_VersionPropagation(t *testing.T) {
	entries := expandSingleBranch(SourceEntry{
		Type:    "repo",
		URL:     "https://github.com/example/app",
		Branch:  "main",
		Version: "2024.06",
	}, t.TempDir())
	for _, e := range entries {
		if e.Type == "ast" && e.Version != "2024.06" {
			t.Errorf("ast entry version = %q, want propagation", e.Version)
		}
	}
	bare := expandSingleBranch(SourceEntry{
		Type: "repo", URL: "https://github.com/example/app", Branch: "main",
	}, t.TempDir())
	for _, e := range bare {
		if e.Type == "ast" && e.Version != "" {
			t.Errorf("version invented for version-less repo entry: %q", e.Version)
		}
	}
}
