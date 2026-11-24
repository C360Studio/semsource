package sourcespawn

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
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

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSystemCollisions(t *testing.T) {
	t.Parallel()
	// checkout is a standalone copy at the pinned commit; other is a checkout
	// at another commit.
	checkout, sha := gitCheckout(t, "pinned")
	other, _ := gitCheckout(t, "other")
	pinned := sha[:12]
	subdir := filepath.Join(checkout, "lib")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	pin := func(path, suffix string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: path, Project: "github-com-acme-lib", Version: pinned, InstanceSuffix: suffix}
	}
	pinOf := func(path, suffix string, langs ...string) config.SourceEntry {
		e := pin(path, suffix)
		e.Languages = langs
		return e
	}
	standalone := func(path string, langs ...string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: path, Project: "github-com-acme-lib", Version: pinned, Languages: langs}
	}
	docsPin := config.SourceEntry{Type: "docs", Paths: []string{"/p1/lib"}, Project: "github-com-acme-lib-" + pinned, InstanceSuffix: "via-p1-lib"}
	docsStandalone := func(path string) config.SourceEntry {
		return config.SourceEntry{Type: "docs", Paths: []string{path}, Project: "github-com-acme-lib-" + pinned}
	}
	vendored := standalone(checkout)
	vendored.IndexVendoredJS = true
	vendoredPin := pin("/p1/lib", "via-p1-lib")
	vendoredPin.IndexVendoredJS = true
	cases := []struct {
		name    string
		entries []config.SourceEntry
		// set edits the ast component of entries[i]: "watch_paths[0]."
		// keys its watch path, other keys the component config.
		set  map[int]map[string]any
		want []string // substrings of the refusal; nil = admitted
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
			name: "a docs and an object-store source under one project share the document ID space",
			entries: []config.SourceEntry{
				{Type: "docs", Paths: []string{"/r/a"}, Project: "handbook"},
				{Type: "s3", Endpoint: "http://minio:9000", Bucket: "artifacts", Prefix: "docs/", Project: "handbook"},
			},
			want: []string{"doc-source-handbook (root /r/a)", "(root http://minio:9000/artifacts/docs/)", `system "handbook"`},
		},
		{
			name: "two object-store sources under one project collide",
			entries: []config.SourceEntry{
				{Type: "s3", Bucket: "first", Project: "handbook"},
				{Type: "s3", Bucket: "second", Project: "handbook"},
			},
			want: []string{"objectstore-source-first (root first)", "objectstore-source-second (root second)", `system "handbook"`},
		},
		{
			name: "two prefixes of one bucket mint under the bucket's system",
			entries: []config.SourceEntry{
				{Type: "s3", Bucket: "artifacts", Prefix: "a/"},
				{Type: "s3", Bucket: "artifacts", Prefix: "b/"},
			},
			want: []string{"(root artifacts/a/)", "(root artifacts/b/)", `system "artifacts"`},
		},
		{
			name: "a versioned object-store source is another system than the docs",
			entries: []config.SourceEntry{
				{Type: "docs", Paths: []string{"/r/a"}, Project: "handbook"},
				{Type: "s3", Bucket: "artifacts", Project: "handbook", Version: "2.0"},
			},
		},
		{
			name: "ast and object-store under one project are different ID spaces",
			entries: []config.SourceEntry{
				{Type: "ast", Path: "/r/svc", Project: "svc"},
				{Type: "s3", Bucket: "artifacts", Project: "svc"},
			},
		},
		{
			name:    "two pins of one submodule at one SHA are admitted",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), pin("/p2/lib", "via-p2-lib")},
		},
		{
			name:    "two pins of one SHA indexing other languages are refused",
			entries: []config.SourceEntry{pinOf("/p1/lib", "via-p1-lib", "go"), pinOf("/p2/lib", "via-p2-lib", "python")},
			want:    []string{"(root /p1/lib)", "(root /p2/lib)", "pin /p2/lib indexes [python], pin /p1/lib indexes [go]"},
		},
		{
			name:    "two pins of one SHA indexing the same languages are admitted",
			entries: []config.SourceEntry{pinOf("/p1/lib", "via-p1-lib", "python", "go"), pinOf("/p2/lib", "via-p2-lib", "go", "python", "go")},
		},
		{
			name:    "a standalone checkout at the pinned commit and languages shares the pin's IDs",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(checkout)},
		},
		{
			name:    "a standalone checkout at another commit is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(other)},
			want:    []string{"(root /p1/lib)", "(root " + other + ")", other + " HEAD is ", `not pinned SHA "` + pinned + `"`},
		},
		{
			name:    "a standalone checkout indexing other languages is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(checkout, "go", "python")},
			want:    []string{"(root " + checkout + ")", checkout + " indexes [go python], pin /p1/lib indexes [go]"},
		},
		{
			name:    "a standalone that indexes vendored JS the pin does not is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), vendored},
			want:    []string{"(root /p1/lib)", "(root " + checkout + ")", checkout + " sets index_vendored_js true, pin /p1/lib sets index_vendored_js false"},
		},
		{
			name:    "a standalone that excludes other directories is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(checkout)},
			set:     map[int]map[string]any{1: {"watch_paths[0].excludes": []string{"vendor"}}},
			want:    []string{"(root " + checkout + ")", checkout + " sets excludes [vendor], pin /p1/lib sets excludes []"},
		},
		{
			name:    "a standalone with another per-file symbol cap is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(checkout)},
			set:     map[int]map[string]any{1: {"max_symbols_per_file": 100}},
			want:    []string{checkout + " sets max_symbols_per_file 100, pin /p1/lib sets max_symbols_per_file 5000"},
		},
		{
			name:    "a standalone with the pin's whole parse scope is admitted",
			entries: []config.SourceEntry{vendoredPin, vendored},
			set: map[int]map[string]any{
				0: {"watch_paths[0].excludes": []string{"vendor", "gen"}, "max_symbols_per_file": 5000},
				1: {"watch_paths[0].excludes": []string{"gen", "vendor", "gen"}},
			},
		},
		{
			name:    "a subdirectory of a checkout at the pinned commit is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone(subdir)},
			want:    []string{"(root " + subdir + ")", subdir + " is not the top level of a checkout"},
		},
		{
			name:    "a standalone root whose HEAD cannot be read is refused",
			entries: []config.SourceEntry{pin("/p1/lib", "via-p1-lib"), standalone("/r/missing")},
			want:    []string{"(root /r/missing)", "/r/missing HEAD unreadable"},
		},
		{
			name:    "a docs checkout at the pinned commit shares the docs pin's IDs",
			entries: []config.SourceEntry{docsPin, docsStandalone(checkout)},
		},
		{
			name:    "a docs checkout at another commit is refused",
			entries: []config.SourceEntry{docsPin, docsStandalone(other)},
			want:    []string{"(root " + other + ")", `not pinned SHA "` + pinned + `"`},
		},
		{
			name:    "two standalone sources at one pinned identity, no pin, collide",
			entries: []config.SourceEntry{standalone(checkout), standalone("/s/lib")},
			want:    []string{"(root " + checkout + ")", "(root /s/lib)"},
		},
		{
			name: "a standalone source at another SHA of the pinned project does not meet the pin",
			entries: []config.SourceEntry{
				pin("/p1/lib", "via-p1-lib"),
				{Type: "ast", Path: "/r/lib", Project: "github-com-acme-lib", Version: "0000aaaa1111"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var components []PlannedComponent
			for i, e := range tc.entries {
				pcs := planned(t, e)
				if edits, ok := tc.set[i]; ok {
					pcs[0].Config = withFields(t, pcs[0].Config, edits)
				}
				components = append(components, pcs...)
			}
			err := CheckSystemCollisions(context.Background(), components)
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

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSystemCollisions_DisabledEnvelopeDoesNotSpawn(t *testing.T) {
	t.Parallel()
	a := planned(t, config.SourceEntry{Type: "ast", Path: "/r/a", Project: "svc"})
	// A removed second writer of a's system: same config, another instance.
	b := PlannedComponent{Instance: "ast-source-svc-removed", Config: withRoot(t, a[0].Config, "/r/b")}
	b.Config.Enabled = false
	if err := CheckSystemCollisions(context.Background(), append(a, b)); err != nil {
		t.Fatalf("a disabled envelope was counted as a writer: %v", err)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckSourceCollisions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		sources []config.SourceEntry
		refused bool
		want    []string // substrings the refusal must carry
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
			name: "a docs and an object-store source under one project",
			sources: []config.SourceEntry{
				{Type: "docs", Paths: []string{"./docs"}, Project: "handbook"},
				{Type: "s3", Bucket: "artifacts", Project: "handbook"},
			},
			refused: true,
		},
		{
			name: "two object-store sources building one instance from different roots",
			sources: []config.SourceEntry{
				{Type: "s3", Bucket: "b-x"},
				{Type: "s3", Bucket: "b", Prefix: "x"},
			},
			refused: true,
			want:    []string{"instance objectstore-source-b-x", "root b-x", "root b/x"},
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
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("refusal %q does not name %q", err, w)
				}
			}
		})
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
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
			name:     "an object-store source under a registered docs project",
			existing: []config.SourceEntry{{Type: "docs", Paths: []string{"/r/a"}, Project: "handbook"}},
			add:      config.SourceEntry{Type: "s3", Bucket: "artifacts", Project: "handbook"},
			refused:  "doc-source-handbook (root /r/a)",
		},
		{
			name:     "re-adding the same object-store source",
			existing: []config.SourceEntry{{Type: "s3", Bucket: "artifacts", Project: "handbook"}},
			add:      config.SourceEntry{Type: "s3", Bucket: "artifacts", Project: "handbook", Watch: true},
		},
		{
			name:     "an object-store source building a registered instance from another root",
			existing: []config.SourceEntry{{Type: "s3", Bucket: "b-x"}},
			add:      config.SourceEntry{Type: "s3", Bucket: "b", Prefix: "x"},
			refused:  "instance objectstore-source-b-x would be built from root b-x and from root b/x",
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

	t.Run("a standalone add at a registered pin's identity shares the pin's IDs", func(t *testing.T) {
		t.Parallel()
		checkout, sha := gitCheckout(t, "pinned")
		pinned := planned(t, config.SourceEntry{Type: "ast", Path: "/p1/lib", Project: "github-com-acme-lib",
			Version: sha[:12], InstanceSuffix: "via-p1-lib"})[0]
		standalone := config.SourceEntry{Type: "ast", Path: checkout, Project: "github-com-acme-lib", Version: sha[:12]}
		for _, recorded := range []bool{true, false} {
			store := newFakeStore()
			cfg := store.cfg.Get()
			cfg.Components = semconfig.ComponentConfigs{pinned.Instance: pinned.Config}
			if recorded {
				record, _ := json.Marshal(map[string]any{"discovered": []map[string]any{
					{"parent": "/p1", "kind": "submodule", "components": []string{pinned.Instance}},
				}})
				cfg.Components["source-manifest"] = types.ComponentConfig{Type: types.ComponentTypeProcessor, Name: "source-manifest", Enabled: true, Config: record}
			}
			if err := store.cfg.Update(cfg); err != nil {
				t.Fatal(err)
			}
			_, err := AddWithChecker(context.Background(), standalone, store, nil, collisionOpts)
			if recorded && err != nil {
				t.Fatalf("add at a recorded pin's identity refused: %v", err)
			}
			if !recorded && CodeOf(err) != CodeValidationFailed {
				t.Fatalf("add colliding with an unrecorded (operator) source: err = %v, want %s", err, CodeValidationFailed)
			}
		}
	})

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

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestAddWithChecker_RefusesTakingOverADiscoveredName(t *testing.T) {
	t.Parallel()
	// Boot discovery owns ast-source-svc-feature from the worktree; an
	// operator removed it, which disables the envelope and keeps the name.
	discovered := planned(t, config.SourceEntry{Type: "ast", Path: "/discovered/worktree", Project: "svc", BranchSlug: "feature"})[0]
	seed := func(t *testing.T, recorded bool) *fakeStore {
		t.Helper()
		store := newFakeStore()
		cfg := store.cfg.Get()
		removed := discovered.Config
		removed.Enabled = false
		cfg.Components = semconfig.ComponentConfigs{discovered.Instance: removed}
		if recorded {
			record, _ := json.Marshal(map[string]any{"discovered": []map[string]any{
				{"parent": "/discovered", "kind": "branch", "components": []string{discovered.Instance}},
			}})
			cfg.Components["source-manifest"] = types.ComponentConfig{Type: types.ComponentTypeProcessor, Name: "source-manifest", Enabled: true, Config: record}
		}
		if err := store.cfg.Update(cfg); err != nil {
			t.Fatal(err)
		}
		return store
	}
	cases := []struct {
		name     string
		recorded bool
		add      config.SourceEntry
		refused  bool
	}{
		{"a re-add from another root is refused", true, config.SourceEntry{Type: "ast", Path: "/retained/standalone", Project: "svc-feature"}, true},
		{"a re-add from the discovered root is admitted", true, config.SourceEntry{Type: "ast", Path: "/discovered/worktree", Project: "svc-feature"}, false},
		{"a name no discovery record owns is the operator's", false, config.SourceEntry{Type: "ast", Path: "/retained/standalone", Project: "svc-feature"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := seed(t, tc.recorded)
			if got := planned(t, tc.add)[0].Instance; got != discovered.Instance {
				t.Fatalf("fixture drift: the add builds %s, not %s", got, discovered.Instance)
			}
			before := maps.Clone(store.cfg.Get().Components)
			_, err := AddWithChecker(context.Background(), tc.add, store, nil, collisionOpts)
			if !tc.refused {
				if err != nil {
					t.Fatalf("add refused: %v", err)
				}
				return
			}
			if CodeOf(err) != CodeValidationFailed {
				t.Fatalf("err = %v, want %s", err, CodeValidationFailed)
			}
			if !strings.Contains(err.Error(), "discovered root /discovered/worktree") {
				t.Errorf("refusal %q does not name the discovered root", err)
			}
			if len(store.puts) != 0 {
				t.Errorf("a refused add wrote %v", store.puts)
			}
			after := store.cfg.Get().Components
			if len(after) != len(before) {
				t.Errorf("components changed: %d, want %d", len(after), len(before))
			}
			for name, want := range before {
				if !after[name].Equal(want) {
					t.Errorf("%s changed by a refused add", name)
				}
			}
		})
	}
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

// withFields sets fields of an ast component config: a "watch_paths[0]."
// key on its first watch path, any other key on the component.
func withFields(t *testing.T, cc types.ComponentConfig, edits map[string]any) types.ComponentConfig {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(cc.Config, &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range edits {
		if field, ok := strings.CutPrefix(k, "watch_paths[0]."); ok {
			m["watch_paths"].([]any)[0].(map[string]any)[field] = v
			continue
		}
		m[k] = v
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	cc.Config = raw
	return cc
}

// gitCheckout is a git repository in a temp dir with one commit whose message
// is msg (distinct messages give distinct commits), and that commit's SHA.
func gitCheckout(t *testing.T, msg string) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@e.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", msg)
	return dir, git("rev-parse", "HEAD")
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckDiscoveredCollisions_RepeatedInstance(t *testing.T) {
	t.Parallel()
	a := planned(t, config.SourceEntry{Type: "ast", Path: "/d/a", Project: "svc", BranchSlug: "feature"})[0]
	b := PlannedComponent{Instance: a.Instance, Config: withRoot(t, a.Config, "/d/b")}
	if err := CheckDiscoveredCollisions(context.Background(), nil, []PlannedComponent{a, a}); err != nil {
		t.Fatalf("one discovered source listed twice was refused: %v", err)
	}
	err := CheckDiscoveredCollisions(context.Background(), nil, []PlannedComponent{a, b})
	if err == nil {
		t.Fatal("one discovered instance from two roots was admitted")
	}
	for _, want := range []string{"/d/a", "/d/b", a.Instance} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %q", err, want)
		}
	}
}
