package astsource

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/entityid"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// symbolsFor parses each file through the asset guard and returns, per base
// name, how many non-container entities it yielded.
func symbolsFor(t *testing.T, indexVendored bool, files map[string]string) map[string]int {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(root, "js", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	auth := entityid.Authority{Org: "acme", Platform: "test"}
	parser, err := semsourceast.DefaultRegistry.CreateParser("javascript", auth, "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	pw := &pathWatcher{
		root:         root,
		scopedSystem: "proj",
		config:       WatchPathConfig{Org: "acme", Project: "proj", IndexVendoredJS: indexVendored},
		parsers:      map[string]semsourceast.FileParser{"javascript": parser},
		routes:       map[string]string{".js": "javascript"},
	}
	c := &Component{logger: slog.Default(), authority: auth}

	out := map[string]int{}
	for name := range files {
		res, err := c.parseFileWithWatcher(context.Background(), pw, filepath.Join(root, "js", name))
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if res.FileEntity == nil {
			t.Errorf("%s has no file entity", name)
		}
		for _, e := range res.Entities {
			if !containerTypes[e.Type] {
				out[name]++
			}
		}
	}
	return out
}

var twinFiles = map[string]string{
	"jquery.min.js":   "var a=1;function f(){return a}\n",
	"jquery.js":       "function jQuery() {\n  return 1;\n}\n",
	"process_flow.js": "function flow() {\n  return 2;\n}\n",
}

// TestVendoredTwinIsFileEntityOnly pins asset-ingestion-guards, "Unminified
// twins of minified assets are indexed as files": a vendored X.js beside
// X.min.js keeps its file entity and yields no symbols; first-party JS is
// untouched.
// spec: openspec/specs/asset-ingestion-guards/spec.md#Requirement: Unminified twins of minified assets are indexed as files
func TestVendoredTwinIsFileEntityOnly(t *testing.T) {
	got := symbolsFor(t, false, twinFiles)
	if got["jquery.js"] != 0 || got["jquery.min.js"] != 0 {
		t.Errorf("vendored files yielded symbols: %v", got)
	}
	if got["process_flow.js"] == 0 {
		t.Errorf("first-party process_flow.js yielded no symbols: %v", got)
	}
}

// TestIndexVendoredJSOptsBackIn pins the opt-in scenario: the twin rule is
// off, so jquery.js yields its symbols again.
// spec: openspec/specs/asset-ingestion-guards/spec.md#Requirement: Unminified twins of minified assets are indexed as files
func TestIndexVendoredJSOptsBackIn(t *testing.T) {
	got := symbolsFor(t, true, twinFiles)
	if got["jquery.js"] == 0 {
		t.Errorf("index_vendored_js: jquery.js yielded no symbols: %v", got)
	}
	if got["process_flow.js"] == 0 {
		t.Errorf("first-party process_flow.js yielded no symbols: %v", got)
	}
}
