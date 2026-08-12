package c

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/source/ast"
)

// parseCTree materialises {relPath: source} under a fresh root and parses every
// file in sorted order with ONE parser, returning all entities.
func parseCTree(t *testing.T, files map[string]string) []*ast.CodeEntity {
	t.Helper()
	root := t.TempDir()
	rels := make([]string, 0, len(files))
	for rel, src := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	p := NewParser("acme", "test", root)
	var all []*ast.CodeEntity
	for _, rel := range rels {
		res, err := p.ParseFile(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		all = append(all, res.Entities...)
	}
	return all
}

func callsOfC(t *testing.T, ents []*ast.CodeEntity, caller string) []string {
	t.Helper()
	// Prototypes and definitions share name+type; only the DEFINITION's
	// extraction runs extractCalls, so prefer the entity carrying edges and
	// fall back to any (the genuinely-empty case).
	var fallback *ast.CodeEntity
	for _, e := range ents {
		if e.Type != ast.TypeFunction || e.Name != caller {
			continue
		}
		if len(e.Calls) > 0 {
			return e.Calls
		}
		fallback = e
	}
	if fallback == nil {
		t.Fatalf("function %q not found", caller)
	}
	return fallback.Calls
}

// assertCCallsExactly pins the caller's entire edge set: a dangling target is
// as wrong as a wrong real one, and only an exact assertion catches it.
func assertCCallsExactly(t *testing.T, ents []*ast.CodeEntity, caller string, want ...string) {
	t.Helper()
	got := append([]string(nil), callsOfC(t, ents, caller)...)
	sort.Strings(got)
	wantIDs := make([]string, 0, len(want))
	for _, spec := range want {
		// "name" matches any function entity of that name; "path#name" pins
		// the defining file — needed when a prototype creates a same-name
		// entity in another file and the edge must target the DEFINITION.
		name, path := spec, ""
		if idx := strings.IndexByte(spec, '#'); idx >= 0 {
			path, name = spec[:idx], spec[idx+1:]
		}
		var found *ast.CodeEntity
		for _, e := range ents {
			if e.Type == ast.TypeFunction && e.Name == name && (path == "" || e.Path == path) {
				found = e
				break
			}
		}
		if found == nil {
			t.Fatalf("expected callee %q has no entity", spec)
		}
		wantIDs = append(wantIDs, found.ID)
	}
	sort.Strings(wantIDs)
	if len(got) != len(wantIDs) {
		t.Fatalf("%s calls = %v, want exactly %v", caller, got, wantIDs)
	}
	for i := range got {
		if got[i] != wantIDs[i] {
			t.Fatalf("%s calls = %v, want exactly %v", caller, got, wantIDs)
		}
	}
}

func TestCSameFileCallResolves(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"src/a.c": "static void helper(void) {}\nvoid run(void) { helper(); }\n",
	})
	assertCCallsExactly(t, ents, "run", "helper")
}

func TestCCrossFileCallResolves(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"src/util.c": "void util_go(void) {}\n",
		"src/app.c":  "void util_go(void);\nvoid run(void) { util_go(); }\n",
	})
	assertCCallsExactly(t, ents, "run", "src/util.c#util_go")
}

// Two files defining the same name is the measured 1.4% tail (redis): the
// binding is ambiguous and the edge drops — never a coin flip.
func TestCCollidingNameStaysInert(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"a/dup.c":   "void frob(void) {}\n",
		"b/dup.c":   "void frob(void) {}\n",
		"src/app.c": "void frob(void);\nvoid run(void) { frob(); }\n",
	})
	assertCCallsExactly(t, ents, "run")
}

// A callee bound to a parameter or local is a function-pointer call: the
// pointee is unknowable statically (#143's rule applied to C).
func TestCFunctionPointerCallsStayInert(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"src/a.c": "static int probe(int x) { return x; }\n" +
			"void run(int (*cb)(int)) {\n" +
			"  int (*local)(int) = probe;\n" +
			"  cb(1);\n" +
			"  local(2);\n" +
			"  probe(3);\n" +
			"}\n",
	})
	assertCCallsExactly(t, ents, "run", "probe")
}

// Field and pointer-member calls need value tracking; inert.
func TestCMemberCallsStayInert(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"src/a.c": "struct ops { void (*fire)(void); };\n" +
			"void run(struct ops *o) { o->fire(); }\n",
	})
	assertCCallsExactly(t, ents, "run")
}

// A name with no in-tree definition (libc, macros) emits nothing — C has no
// import bindings to justify even an external: marker.
func TestCUndefinedNameStaysInert(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"src/a.c": "void run(void) { printf(\"x\"); }\n",
	})
	assertCCallsExactly(t, ents, "run")
}

// A header-inline definition is a definition; the prototype in another header
// is not. Only function_definition nodes join the index.
func TestCHeaderInlineDefinitionResolves(t *testing.T) {
	ents := parseCTree(t, map[string]string{
		"include/u.h": "static inline int twice(int x) { return x + x; }\n",
		"src/app.c":   "#include \"u.h\"\nint run(int v) { return twice(v); }\n",
	})
	assertCCallsExactly(t, ents, "run", "twice")
}

// The index and the ingester must agree on corpus membership: a skip set
// looser than the ingester's mints dangling edges, a tighter one silently
// unbinds real definitions. handler.DefaultExcludedDirNames is the contract.
func TestDefSkipDirsMatchIngester(t *testing.T) {
	want := map[string]bool{}
	for _, name := range handler.DefaultExcludedDirNames() {
		want[name] = true
	}
	if len(defSkipDirs) != len(want) {
		t.Fatalf("defSkipDirs = %v, ingester excludes %v", defSkipDirs, want)
	}
	for name := range want {
		if !defSkipDirs[name] {
			t.Errorf("ingester excludes %q but the call index does not", name)
		}
	}
}

// A repo root whose own base name is hidden (a checkout under ".cache/...")
// must still index — the prune guard applies to children, not the root.
func TestHiddenRootStillIndexes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, ".cache")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, src := range map[string]string{
		"src/a.c": "static void helper(void) {}\nvoid run(void) { helper(); }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := NewParser("acme", "test", root)
	res, err := p.ParseFile(context.Background(), filepath.Join(root, "src/a.c"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Entities {
		if e.Name == "run" && len(e.Calls) == 1 {
			return
		}
	}
	t.Fatal("hidden-named root must still resolve call edges")
}
