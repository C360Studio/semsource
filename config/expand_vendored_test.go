package config

import (
	"context"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

// TestRepoExpansionForwardsIndexVendoredJS pins that index_vendored_js on a
// repo entry reaches the ast child it expands into, for single-branch and
// per-branch expansion alike, and stays off when the repo leaves it unset.
// spec: openspec/specs/asset-ingestion-guards/spec.md#Requirement: Unminified twins of minified assets are indexed as files
func TestRepoExpansionForwardsIndexVendoredJS(t *testing.T) {
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
		for _, on := range []bool{false, true} {
			src := SourceEntry{Type: "repo", URL: "https://github.com/example/repo", Language: "javascript", IndexVendoredJS: on}
			var asts int
			for _, s := range expand(t, src) {
				if s.Type != "ast" {
					continue
				}
				asts++
				if s.IndexVendoredJS != on {
					t.Errorf("%s: ast child index_vendored_js = %v, repo set %v", name, s.IndexVendoredJS, on)
				}
			}
			if asts == 0 {
				t.Errorf("%s: expansion produced no ast child", name)
			}
		}
	}
}
