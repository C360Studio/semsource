package config

import (
	"context"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

// TestRepoExpansionForwardsIndexInterval pins #400: index_interval on a repo
// entry reaches the ast child it expands into, for single-branch and
// per-branch expansion alike, and stays unset when the repo leaves it unset
// (so the spawn path's own default still applies).
// spec: openspec/specs/ast-source-configuration/spec.md#Requirement: A repo source's index interval governs its ast child
func TestRepoExpansionForwardsIndexInterval(t *testing.T) {
	expanders := map[string]func(t *testing.T, src SourceEntry) []SourceEntry{
		"single branch": func(t *testing.T, src SourceEntry) []SourceEntry {
			// Branch set so expansion never runs a live git ls-remote.
			src.Branch = "main"
			result, err := ExpandRepoSources(context.Background(), []SourceEntry{src}, t.TempDir())
			if err != nil {
				t.Fatalf("ExpandRepoSources: %v", err)
			}
			return result.Sources
		},
		"per branch": func(_ *testing.T, src SourceEntry) []SourceEntry {
			return expandBranchSources(src, workspace.BranchState{
				Branch: "scenario/a", Slug: "scenario-a", WorktreePath: "/tmp/wt/scenario-a",
			})
		},
	}
	for name, expand := range expanders {
		for _, interval := range []string{"", "30m"} {
			src := SourceEntry{Type: "repo", URL: "https://github.com/example/repo", Language: "go", Watch: true, IndexInterval: interval}
			var asts int
			for _, s := range expand(t, src) {
				if s.Type != "ast" {
					continue
				}
				asts++
				if s.IndexInterval != interval {
					t.Errorf("%s: ast child index_interval = %q, repo set %q", name, s.IndexInterval, interval)
				}
			}
			if asts == 0 {
				t.Errorf("%s: expansion produced no ast child", name)
			}
		}
	}
}
