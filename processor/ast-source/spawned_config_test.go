package astsource

import (
	"context"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

// TestSpawnedConfigCarriesIndexVendoredJS decodes, through this component's
// strict config decode, what semsource.json becomes on the way here: a repo
// entry expanded into its ast child, then built by sourcespawn. A json-tag
// typo on either side fails the decode; a dropped field fails the value
// check. It lives here rather than in internal/sourcespawn because importing
// this package registers every parser, which would skew that package's
// registry-coverage test.
// spec: openspec/specs/asset-ingestion-guards/spec.md#Requirement: Unminified twins of minified assets are indexed as files
func TestSpawnedConfigCarriesIndexVendoredJS(t *testing.T) {
	for _, on := range []bool{false, true} {
		repo := config.SourceEntry{Type: "repo", URL: "https://github.com/example/repo", Branch: "main",
			Language: "javascript", IndexVendoredJS: on}
		expanded, err := config.ExpandRepoSources(context.Background(), []config.SourceEntry{repo}, t.TempDir())
		if err != nil {
			t.Fatalf("ExpandRepoSources: %v", err)
		}
		decoded := 0
		for _, src := range expanded.Sources {
			if src.Type != "ast" {
				continue
			}
			built, err := sourcespawn.Build(src, sourcespawn.Options{Org: "acme"})
			if err != nil {
				t.Fatalf("sourcespawn.Build: %v", err)
			}
			for name, cc := range built {
				if cc.Name != "ast-source" {
					continue
				}
				comp, err := NewComponent(cc.Config, component.Dependencies{
					Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"},
				})
				if err != nil {
					t.Fatalf("%s: ast-source rejected the spawned config: %v", name, err)
				}
				for _, wp := range comp.(*Component).config.WatchPaths {
					if wp.IndexVendoredJS != on {
						t.Errorf("%s: watch path index_vendored_js = %v, repo set %v", name, wp.IndexVendoredJS, on)
					}
				}
				decoded++
			}
		}
		if decoded == 0 {
			t.Fatalf("index_vendored_js=%v: no ast-source config was spawned", on)
		}
	}
}
