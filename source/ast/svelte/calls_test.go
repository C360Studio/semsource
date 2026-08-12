package svelte

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// writeTree materialises {relPath: source} under a fresh root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// parseTree writes every file in the tree to disk but only drives ParseFile
// over the .svelte ones — matching the real ast-source dispatch, where each
// language's own parser is the one that walks a file, and a plain .ts helper
// exists purely as an import target that call resolution reads directly (never
// through svelte.Parser.ParseFile, which always parses with the Svelte
// grammar).
func parseTree(t *testing.T, files map[string]string) []*ast.CodeEntity {
	t.Helper()
	root := writeTree(t, files)
	var rels []string
	for rel := range files {
		if strings.HasSuffix(rel, ".svelte") {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)

	var all []*ast.CodeEntity
	p := NewParser("acme", "test", root)
	for _, rel := range rels {
		res, err := p.ParseFile(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		all = append(all, res.Entities...)
	}
	return all
}

func entityNamed(t *testing.T, ents []*ast.CodeEntity, name string) *ast.CodeEntity {
	t.Helper()
	var found []*ast.CodeEntity
	for _, e := range ents {
		if e.Name == name {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one entity named %q, got %d", name, len(found))
	}
	return found[0]
}

func callsOf(t *testing.T, ents []*ast.CodeEntity, name string) []string {
	t.Helper()
	return entityNamed(t, ents, name).Calls
}

func hasCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

func assertCalls(t *testing.T, ents []*ast.CodeEntity, caller, calleeName string) {
	t.Helper()
	callee := entityNamed(t, ents, calleeName)
	calls := callsOf(t, ents, caller)
	if !hasCall(calls, callee.ID) {
		t.Errorf("%s calls = %v\n  want it to contain %s definition ID %q", caller, calls, calleeName, callee.ID)
	}
}

// assertCallsExactly pins a caller's entire edge set — a dangling target is
// just as wrong as the wrong real one, and only an exact assertion catches it.
func assertCallsExactly(t *testing.T, ents []*ast.CodeEntity, caller string, want ...string) {
	t.Helper()
	got := append([]string(nil), callsOf(t, ents, caller)...)
	sort.Strings(got)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if len(got) != len(sorted) {
		t.Fatalf("%s calls = %v, want exactly %v", caller, got, sorted)
	}
	for i := range got {
		if got[i] != sorted[i] {
			t.Fatalf("%s calls = %v, want exactly %v", caller, got, sorted)
		}
	}
}

func assertNoCalls(t *testing.T, ents []*ast.CodeEntity, caller string) {
	t.Helper()
	assertCallsExactly(t, ents, caller)
}

// TestSvelteScriptSameFileCallResolves also pins the domain decision behind
// ExtractCalls's lang argument: the callee's own entity ID (built by
// parser.go's extractFunction) uses domain "svelte", so the edge must too, or
// it would dangle against a typescript/javascript-domain ID no entity has.
func TestSvelteScriptSameFileCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"Component.svelte": "<script>\n" +
			"function helper() {}\n" +
			"function run() { helper(); }\n" +
			"</script>\n",
	})
	assertCalls(t, ents, "run", "helper")
	helper := entityNamed(t, ents, "helper")
	if !strings.Contains(helper.ID, ".svelte.") {
		t.Fatalf("helper.ID = %q, want a svelte-domain entity ID", helper.ID)
	}
}

func TestSvelteScriptArrowFunctionCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"Component.svelte": "<script>\n" +
			"const helper = () => {};\n" +
			"function run() { helper(); }\n" +
			"</script>\n",
	})
	assertCalls(t, ents, "run", "helper")
}

// The spec scenario this change adds for Svelte: "a .svelte component whose
// script imports an in-tree function and calls it" emits the same edge shape
// as an equivalent .ts module's import call — design D3 calls this
// component→module wiring "free" once the shared ts pass drives the script
// block (see parser.go's callResolver / PrepareCallResolution).
//
// lib/util.ts is never routed through svelte.Parser.ParseFile (parseTree only
// drives .svelte files — see its doc comment), so it contributes no entity to
// `ents` for entityNamed/assertCalls to compare against; the expected callee ID
// is built directly instead, exactly as the real ts.Parser would build it for
// util.ts's own "helper" function_declaration.
func TestSvelteScriptImportsAndCallsInTreeFunction(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/util.ts": "export function helper() {}\n",
		"Component.svelte": "<script>\n" +
			"import { helper } from './lib/util';\n" +
			"function run() { helper(); }\n" +
			"</script>\n",
	})
	run := entityNamed(t, ents, "run")
	want := ast.NewCodeEntity("acme", "typescript", "test", ast.TypeFunction, "helper", "lib/util.ts")
	if !hasCall(run.Calls, want.ID) {
		t.Fatalf("run calls = %v, want it to contain util.ts's helper definition ID %q", run.Calls, want.ID)
	}
}

// An out-of-tree import target still stays observable via the "external:"
// marker, mirroring plain TS/JS resolution exactly (calls.go is shared, not
// reimplemented).
func TestSvelteScriptExternalImportEmitsExternalMarker(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"Component.svelte": "<script>\n" +
			"import { helper } from 'some-package';\n" +
			"function run() { helper(); }\n" +
			"</script>\n",
	})
	assertCallsExactly(t, ents, "run", "external:helper")
}

func TestSvelteScriptInertPropertyChainCall(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"Component.svelte": "<script>\n" +
			"function run() { a.b.c(); }\n" +
			"</script>\n",
	})
	assertNoCalls(t, ents, "run")
}

// A parameter shadowing a same-script function must stay inert in the script
// block too — this comes "for free" only if the local-shadow guard
// (ts/calls.go's localValueNames) is actually threaded through
// callResolver.ExtractCalls's new params argument; a missed wiring spot here
// would silently resolve the wrong real edge.
func TestSvelteScriptParamShadowsScriptFunctionStaysInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"Component.svelte": "<script>\n" +
			"function transform(x) { return x; }\n" +
			"function run(items, transform) { items.map(i => transform(i)); }\n" +
			"</script>\n",
	})
	assertNoCalls(t, ents, "run")
}
