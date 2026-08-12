package ts

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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

// parseTree parses every file in a tree and returns all entities. Files are
// parsed in sorted order so a test's result does not depend on map iteration.
func parseTree(t *testing.T, files map[string]string) []*ast.CodeEntity {
	t.Helper()
	root := writeTree(t, files)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return parseRels(t, root, rels)
}

func parseRels(t *testing.T, root string, rels []string) []*ast.CodeEntity {
	t.Helper()
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

// entityNamed returns the single entity with the given name, failing when it is
// absent or ambiguous — so a test can never silently assert against the wrong
// one.
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

// assertCallsExactly pins a caller's entire edge set. Asserting the exact set —
// rather than merely "no edge to a real entity" — is what makes the inert cases
// real guards: emitting a DANGLING target (an ID no entity has) is just as wrong
// as emitting the wrong real one, and only an exact assertion catches it.
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

// assertNoCalls is the inert case: not one edge of any kind.
func assertNoCalls(t *testing.T, ents []*ast.CodeEntity, caller string) {
	t.Helper()
	assertCallsExactly(t, ents, caller)
}

// --- same-file resolution ---------------------------------------------------

func TestSameFileFunctionCall(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function helper() {}\nfunction run() { helper(); }\n",
	})
	assertCalls(t, ents, "run", "helper")
}

// A same-file call target can just as well be an arrow-function declarator —
// D3 names both forms explicitly, and parser.go builds an identical TypeFunction
// entity for either, so the two definition shapes must resolve identically.
func TestSameFileArrowFunctionAsCalleeResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "const helper = () => {};\nfunction run() { helper(); }\n",
	})
	assertCalls(t, ents, "run", "helper")
}

// The caller side of the same coin: an arrow function's OWN body must also be
// walked for calls, not just a plain function_declaration's.
func TestArrowFunctionBodyCallsResolve(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function helper() {}\nconst run = () => { helper(); };\n",
	})
	assertCalls(t, ents, "run", "helper")
}

// --- cross-module import resolution -----------------------------------------

func TestCrossModuleNamedImportCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/util.ts": "export function helper() {}\n",
		"lib/app.ts":  "import { helper } from './util';\nfunction run() { helper(); }\n",
	})
	assertCalls(t, ents, "run", "helper")
}

func TestNamespaceImportCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/util.ts": "export function helper() {}\n",
		"lib/app.ts":  "import * as util from './util';\nfunction run() { util.helper(); }\n",
	})
	assertCalls(t, ents, "run", "helper")
}

// A member access on an identifier that is NOT a namespace import (just an
// ordinary local value) must stay inert — resolving it would require typing an
// arbitrary expression, exactly the guess D3 forbids.
func TestMemberCallOnNonNamespaceIdentifierStaysInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function run() { const obj = {}; obj.helper(); }\n",
	})
	assertNoCalls(t, ents, "run")
}

func TestDefaultImportCallResolves_NamedDeclaration(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/def.ts": "export default function realName() {}\n",
		"lib/app.ts": "import Def from './def';\nfunction run() { Def(); }\n",
	})
	assertCalls(t, ents, "run", "realName")
}

func TestDefaultImportCallResolves_IdentifierReference(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/def.ts": "function realName() {}\nexport default realName;\n",
		"lib/app.ts": "import Def from './def';\nfunction run() { Def(); }\n",
	})
	assertCalls(t, ents, "run", "realName")
}

// An anonymous default export has no name to point an edge at — the local
// import binding name ("Def") is caller-chosen and cannot substitute for it
// (imports.go's own comment documents the same fail-inert choice for renamed
// default imports at the TYPE-reference layer).
func TestDefaultImportCallInert_AnonymousDefaultExport(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/def.ts": "export default function() {}\n",
		"lib/app.ts": "import Def from './def';\nfunction run() { Def(); }\n",
	})
	assertNoCalls(t, ents, "run")
}

// A default export that resolves to a non-function (a class, here) must stay
// inert too — moduleInfo.funcs is the confirming check, and a class never
// populates it.
func TestDefaultImportCallInert_NonFunctionDefaultExport(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/def.ts": "export default class Thing {}\n",
		"lib/app.ts": "import Def from './def';\nfunction run() { Def(); }\n",
	})
	assertNoCalls(t, ents, "run")
}

// --- this.-method resolution --------------------------------------------------

func TestThisMethodCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "class Foo {\n  bar() {}\n  baz() { this.bar(); }\n}\n",
	})
	assertCalls(t, ents, "baz", "bar")
}

// An inherited method is not in the OWN class's method set (D3: no supers walk
// for TS), so this.-resolution against a subclass must stay inert even though
// the method exists on an ancestor.
func TestThisMethodCallToInheritedMethodStaysInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "class Base {\n  bar() {}\n}\nclass Foo extends Base {\n  baz() { this.bar(); }\n}\n",
	})
	assertNoCalls(t, ents, "baz")
}

// A plain nested function rebinds `this` (e.g. a callback passed to a timer or
// event API) — resolving this.bar() there against the enclosing method's class
// would be a fabricated edge, since at runtime that `this` is something else
// entirely.
func TestThisReboundInNestedFunctionStaysInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "class Foo {\n  bar() {}\n  baz() {\n    setTimeout(function() { this.bar(); }, 0);\n  }\n}\n",
	})
	assertNoCalls(t, ents, "baz")
}

// The converse of the previous case: an ARROW callback does not rebind `this`,
// so this.bar() inside one still resolves against the enclosing method's class.
func TestThisNotReboundInArrowCallbackResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "class Foo {\n  bar() {}\n  baz() {\n    setTimeout(() => { this.bar(); }, 0);\n  }\n}\n",
	})
	assertCalls(t, ents, "baz", "bar")
}

// --- deliberately inert forms ------------------------------------------------

func TestInertPropertyChainCall(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function run() { a.b.c(); }\n",
	})
	assertNoCalls(t, ents, "run")
}

func TestInertComputedCalleeCall(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function run() { const fns = {}; const key = 'x'; fns[key](); }\n",
	})
	assertNoCalls(t, ents, "run")
}

// A bare call to a genuinely undeclared/global name (not a local definition,
// not any kind of import) has nothing to resolve against at all — this is the
// zero-edge inert case, distinct from the "external:" marker an out-of-tree
// IMPORT produces below.
func TestUnknownBareCallStaysInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "function run() { console.log('hi'); doSomethingUndeclared(); }\n",
	})
	assertNoCalls(t, ents, "run")
}

// --- out-of-tree imports ------------------------------------------------------

func TestExternalNamedImportCallEmitsExternalMarker(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "import { helper } from 'some-package';\nfunction run() { helper(); }\n",
	})
	assertCallsExactly(t, ents, "run", "external:helper")
}

func TestExternalNamespaceImportCallEmitsExternalMarker(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "import * as util from 'some-package';\nfunction run() { util.helper(); }\n",
	})
	assertCallsExactly(t, ents, "run", "external:util.helper")
}

func TestExternalDefaultImportCallEmitsExternalMarker(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m.ts": "import Def from 'some-package';\nfunction run() { Def(); }\n",
	})
	assertCallsExactly(t, ents, "run", "external:some-package")
}

// --- shadowing ---------------------------------------------------------------

// A locally-defined function shadows an import of the same name (mirrors
// python/calls.go's documented priority). Ordinary TS/JS syntax forbids the
// exact collision this constructs (a top-level function redeclaring an import
// binding is a SyntaxError under a real type checker), but tree-sitter parses
// each statement independently and the priority rule is worth pinning as a
// resolver-level guarantee regardless.
func TestLocalDefinitionShadowsImport(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/other.ts": "export function helper() {}\n",
		"lib/app.ts":   "import { helper } from './other';\nfunction helper() {}\nfunction run() { helper(); }\n",
	})
	// Two entities are named "helper" here (other.ts's and app.ts's own), so
	// entityNamed/assertCalls would be ambiguous — build the expected own-file
	// ID directly instead.
	run := entityNamed(t, ents, "run")
	ownHelper := ast.NewCodeEntity("acme", "typescript", "test", ast.TypeFunction, "helper", "lib/app.ts")
	if !hasCall(run.Calls, ownHelper.ID) {
		t.Fatalf("run calls = %v, want own-file helper %q (not the imported one)", run.Calls, ownHelper.ID)
	}
	if len(run.Calls) != 1 {
		t.Fatalf("run calls = %v, want exactly one edge (own-file helper only)", run.Calls)
	}
}
