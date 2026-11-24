package sourcespawn

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
)

var collisionOpts = Options{Org: "acme"}

// planned builds the components sourcespawn makes for entry, pinned when the
// entry carries a submodule InstanceSuffix, as boot discovery marks it.
func planned(t *testing.T, entry config.SourceEntry) []PlannedComponent {
	t.Helper()
	specs, err := buildSpecs(context.Background(), entry, collisionOpts)
	if err != nil {
		t.Fatalf("build %+v: %v", entry, err)
	}
	var out []PlannedComponent
	for _, spec := range specs {
		pc, err := plannedFromSpec(spec, entry.InstanceSuffix != "")
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, pc)
	}
	return out
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSystemCollisions(t *testing.T) {
	t.Parallel()
	pin := func(path, suffix string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: path, Project: "github-com-acme-lib", Version: "abc123def456", InstanceSuffix: suffix}
	}
	cases := []struct {
		name    string
		entries []config.SourceEntry
		want    []string // substrings of the refusal; nil = admitted
	}{
		{
			name: "same project and version collide",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "/r/a", Project: "svc", Version: "1.0"},
				{Type: "ast", Path: "/r/b", Project: "svc", Version: "1.0"},
			},
			want: []string{"ast-source-svc-1-0 (root /r/a)", "ast-source-svc-1-0 (root /r/b)", `system "svc-1-0"`},
		},
		{
			name: "different versions are admitted",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "/r/v1", Project: "lib", Version: "1.0"},
				{Type: "ast", Path: "/r/v2", Project: "lib", Version: "2.0"},
			},
		},
		{
			name: "different branch scopes are admitted",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "/r/main", Project: "svc"},
				{Type: "ast", Path: "/r/feature", Project: "svc", BranchSlug: "feature-x"},
			},
		},
		{
			name: "a path-derived project equal to an explicit one collides",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "svc"},
				{Type: "ast", Path: "/r/other", Project: "svc"},
			},
			want: []string{"(root svc)", "(root /r/other)", `system "svc"`},
		},
		{
			name: "a doc source with several roots under one project collides",
			entries: []config.SourceEntry{
				{Type: "docs", Paths: []string{"/r/a", "/r/b"}, Project: "handbook"},
			},
			want: []string{"doc-source-handbook (root /r/a)", "doc-source-handbook (root /r/b)", `system "handbook"`},
		},
		{
			name: "ast and doc under one project are different kinds",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "/r/svc", Project: "svc"},
				{Type: "docs", Paths: []string{"/r/svc/docs"}, Project: "svc"},
			},
		},
		{
			name:    "two pins of one submodule at one SHA are admitted",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), pin("/p2/lib", "via-p2-lib")},
		},
		{
			name: "a pin plus an operator source collides",
			entries: []config.SourceEntry{
				pin("/p1/lib", "via-p1-lib"),
				{Type: "ast", Path: "/r/lib", Project: "github-com-acme-lib", Version: "abc123def456"},
			},
			want: []string{"via-p1-lib (root /p1/lib)", "(root /r/lib)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var components []PlannedComponent
			for _, e := range tc.entries {
				components = append(components, planned(t, e)...)
			}
			err := CheckSystemCollisions(components)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("admitted set refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("colliding set admitted")
			}
			if CodeOf(err) != CodeValidationFailed {
				t.Errorf("code = %q, want %q", CodeOf(err), CodeValidationFailed)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not name %q", err, w)
				}
			}
		})
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSystemCollisions_DisabledEnvelopeDoesNotSpawn(t *testing.T) {
	t.Parallel()
	a := planned(t, config.SourceEntry{Type: "ast", Path: "/r/a", Project: "svc"})
	// A removed second writer of a's system: same config, another instance.
	b := PlannedComponent{Instance: "ast-source-svc-removed", Config: withRoot(t, a[0].Config, "/r/b")}
	b.Config.Enabled = false
	if err := CheckSystemCollisions(append(a, b)); err != nil {
		t.Fatalf("a disabled envelope was counted as a writer: %v", err)
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSourceCollisions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		sources []config.SourceEntry
		refused bool
	}{
		{
			name: "two ast sources under one project",
			sources: []config.SourceEntry{
				{Type: "ast", Path: "./a", Project: "svc"},
				{Type: "ast", Path: "./b", Project: "svc"},
			},
			refused: true,
		},
		{
			name: "one source listed twice is one source",
			sources: []config.SourceEntry{
				{Type: "ast", Path: "./a", Project: "svc"},
				{Type: "ast", Path: "./a", Project: "svc"},
			},
		},
		{
			name: "a repo and an ast source under one project",
			sources: []config.SourceEntry{
				{Type: "repo", Path: "/r/svc", Project: "svc"},
				{Type: "ast", Path: "/r/other", Project: "svc"},
			},
			refused: true,
		},
		{
			name: "multi-branch repos are left to boot",
			sources: []config.SourceEntry{
				{Type: "repo", URL: "https://example.invalid/acme/svc.git", Branches: []string{"main"}, Project: "svc"},
				{Type: "ast", Path: "/r/other", Project: "svc"},
			},
		},
		{
			name: "git sources are outside the rule",
			sources: []config.SourceEntry{
				{Type: "git", Path: "/r/a"},
				{Type: "git", Path: "/r/a"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := CheckSourceCollisions(context.Background(), tc.sources, collisionOpts)
			if tc.refused != (err != nil) {
				t.Fatalf("refused = %v, want %v (err %v)", err != nil, tc.refused, err)
			}
		})
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestAddWithChecker_RefusesSharedSystem(t *testing.T) {
	t.Parallel()
	register := func(t *testing.T, store *fakeStore, src config.SourceEntry) {
		t.Helper()
		if _, err := Add(context.Background(), src, store, collisionOpts); err != nil {
			t.Fatalf("seed add %+v: %v", src, err)
		}
	}
	cases := []struct {
		name     string
		existing []config.SourceEntry
		add      config.SourceEntry
		refused  string // instance the refusal must name; "" = admitted
	}{
		{
			name:     "another path under a registered project",
			existing: []config.SourceEntry{{Type: "ast", Path: "/r/a", Project: "svc"}},
			add:      config.SourceEntry{Type: "ast", Path: "/r/b", Project: "svc"},
			refused:  "ast-source-svc (root /r/a)",
		},
		{
			name:     "a doc root under a different project",
			existing: []config.SourceEntry{{Type: "docs", Paths: []string{"/r/a"}, Project: "svc"}},
			add:      config.SourceEntry{Type: "docs", Paths: []string{"/r/b"}, Project: "svc-2"},
		},
		{
			name:     "re-adding the same instance",
			existing: []config.SourceEntry{{Type: "ast", Path: "/r/a", Project: "svc", Watch: true}},
			add:      config.SourceEntry{Type: "ast", Path: "/r/a", Project: "svc"},
		},
		{
			name:     "a registered collision does not block an unrelated add",
			existing: []config.SourceEntry{{Type: "ast", Path: "/r/a", Project: "svc"}},
			add:      config.SourceEntry{Type: "ast", Path: "/r/c", Project: "other"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newFakeStore()
			for _, src := range tc.existing {
				register(t, store, src)
			}
			before := len(store.puts)
			_, err := AddWithChecker(context.Background(), tc.add, store, nil, collisionOpts)
			if tc.refused == "" {
				if err != nil {
					t.Fatalf("add refused: %v", err)
				}
				return
			}
			var serr *Error
			if !errors.As(err, &serr) || serr.Code != CodeValidationFailed {
				t.Fatalf("err = %v, want %s", err, CodeValidationFailed)
			}
			if !strings.Contains(err.Error(), tc.refused) {
				t.Errorf("refusal %q does not name the registered instance %q", err, tc.refused)
			}
			if len(store.puts) != before {
				t.Errorf("a refused add wrote %d components", len(store.puts)-before)
			}
		})
	}

	t.Run("a collision already registered is not this add's", func(t *testing.T) {
		t.Parallel()
		store := newFakeStore()
		cfg := store.cfg.Get()
		shared := planned(t, config.SourceEntry{Type: "ast", Path: "/r/x", Project: "old"})[0].Config
		cfg.Components = semconfig.ComponentConfigs{
			"ast-source-old":     shared,
			"ast-source-old-dup": withRoot(t, shared, "/r/y"),
		}
		if err := store.cfg.Update(cfg); err != nil {
			t.Fatal(err)
		}
		if _, err := AddWithChecker(context.Background(), config.SourceEntry{Type: "ast", Path: "/r/c", Project: "new"}, store, nil, collisionOpts); err != nil {
			t.Fatalf("unrelated add refused for a pre-existing collision: %v", err)
		}
	})
}

// withRoot returns an ast component config with its watch path moved to root.
func withRoot(t *testing.T, cc types.ComponentConfig, root string) types.ComponentConfig {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(cc.Config, &m); err != nil {
		t.Fatal(err)
	}
	m["watch_paths"].([]any)[0].(map[string]any)["path"] = root
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	cc.Config = raw
	if !strings.Contains(string(cc.Config), root) {
		t.Fatalf("fixture drift: %s", cc.Config)
	}
	return cc
}
