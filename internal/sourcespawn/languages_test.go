package sourcespawn

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/source/ast"

	// Parser packages self-register in init(); the registry is the source of
	// truth this gate compares against, so link them the way run.go does.
	_ "github.com/c360studio/semsource/source/ast/golang"
	_ "github.com/c360studio/semsource/source/ast/java"
	_ "github.com/c360studio/semsource/source/ast/python"
	_ "github.com/c360studio/semsource/source/ast/svelte"
	_ "github.com/c360studio/semsource/source/ast/ts"
)

// TestASTComponentConfig_MultiLanguage pins compose-packaging-hardening D4:
// the source entry's language list reaches the spawned component's
// watch_paths[].languages verbatim; a bare entry keeps the historical go
// default.
func TestASTComponentConfig_MultiLanguage(t *testing.T) {
	langsOf := func(src config.SourceEntry) []string {
		t.Helper()
		_, cfg, err := astComponentConfig(src, "acme")
		if err != nil {
			t.Fatalf("astComponentConfig: %v", err)
		}
		wps, ok := cfg["watch_paths"].([]map[string]any)
		if !ok || len(wps) != 1 {
			t.Fatalf("watch_paths shape: %#v", cfg["watch_paths"])
		}
		langs, ok := wps[0]["languages"].([]string)
		if !ok {
			t.Fatalf("languages shape: %#v", wps[0]["languages"])
		}
		return langs
	}

	multi := langsOf(config.SourceEntry{Type: "ast", Path: "/w", Languages: []string{"go", "python", "svelte"}})
	if !slices.Equal(multi, []string{"go", "python", "svelte"}) {
		t.Errorf("multi languages = %v, want pass-through", multi)
	}
	def := langsOf(config.SourceEntry{Type: "ast", Path: "/w"})
	if !slices.Equal(def, []string{"go"}) {
		t.Errorf("default languages = %v, want [go]", def)
	}
}

// TestMVPConfigCoversAllRegisteredLanguages is the drift gate for D4's "the
// default install indexes the whole workspace honestly": the shipped default
// config's ast languages must equal the parser registry's registered names —
// a newly registered parser missing from mvp.json fails here instead of
// silently shrinking default coverage.
func TestMVPConfigCoversAllRegisteredLanguages(t *testing.T) {
	data, err := os.ReadFile("../../configs/mvp.json")
	if err != nil {
		t.Fatalf("read mvp.json: %v", err)
	}
	var cfg struct {
		Sources []config.SourceEntry `json:"sources"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode mvp.json: %v", err)
	}
	var declared []string
	for _, s := range cfg.Sources {
		if s.Type == "ast" {
			declared = s.EffectiveLanguages()
		}
	}
	registered := ast.DefaultRegistry.ListParsers()
	sort.Strings(declared)
	sort.Strings(registered)
	if !slices.Equal(declared, registered) {
		t.Errorf("mvp.json ast languages = %v, registry = %v — default install coverage drifted", declared, registered)
	}
}

// TestASTComponentConfig_Version pins version-registration-surface D1: an
// explicit version reaches the component's watch_paths[].version (the field
// ast-source already honors); absent stays byte-absent so version-less IDs are
// unchanged.
func TestASTComponentConfig_Version(t *testing.T) {
	_, withV, err := astComponentConfig(config.SourceEntry{Type: "ast", Path: "/w", Version: "v1.9.0"}, "acme")
	if err != nil {
		t.Fatalf("astComponentConfig: %v", err)
	}
	wps := withV["watch_paths"].([]map[string]any)
	if got := wps[0]["version"]; got != "v1.9.0" {
		t.Errorf("version = %v, want v1.9.0", got)
	}

	_, withoutV, err := astComponentConfig(config.SourceEntry{Type: "ast", Path: "/w"}, "acme")
	if err != nil {
		t.Fatalf("astComponentConfig: %v", err)
	}
	if _, present := withoutV["watch_paths"].([]map[string]any)[0]["version"]; present {
		t.Error("version key present for a version-less source (must stay byte-identical)")
	}
}

// TestASTComponentConfig_ProjectOverrideAndInstanceUniqueness pins the D1
// amendment: an explicit project overrides the path-derived slug (supersession
// corresponds BY project), and versioned registrations of one project get
// distinct instance names; version-less stays byte-identical to today.
func TestASTComponentConfig_ProjectOverrideAndInstanceUniqueness(t *testing.T) {
	n1, c1, err := astComponentConfig(config.SourceEntry{
		Type: "ast", Path: "/deps/depA-1.9.0", Project: "depA", Version: "v1.9.0"}, "acme")
	if err != nil {
		t.Fatal(err)
	}
	n2, c2, err := astComponentConfig(config.SourceEntry{
		Type: "ast", Path: "/deps/depA-1.10.0", Project: "depA", Version: "v1.10.0"}, "acme")
	if err != nil {
		t.Fatal(err)
	}
	p1 := c1["watch_paths"].([]map[string]any)[0]["project"]
	p2 := c2["watch_paths"].([]map[string]any)[0]["project"]
	if p1 != "depA" || p2 != "depA" {
		t.Errorf("projects = %v/%v, want shared depA (correspondence key)", p1, p2)
	}
	if n1 == n2 {
		t.Errorf("instance names collide for two versioned registrations: %s", n1)
	}

	// Version-less, project-less: byte-identical to the historical shape.
	nBare, cBare, err := astComponentConfig(config.SourceEntry{Type: "ast", Path: "/w/app"}, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if nBare != "ast-source-app" {
		t.Errorf("bare instance name = %s, want ast-source-app", nBare)
	}
	if _, present := cBare["watch_paths"].([]map[string]any)[0]["version"]; present {
		t.Error("bare config gained a version key")
	}
}
