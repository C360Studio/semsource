package ts

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parseTSFile(t *testing.T, name, src string) []*ast.CodeEntity {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewParser(testAuthority("test-org"), "test-project", dir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	var out []*ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type != ast.TypeFile {
			out = append(out, e)
		}
	}
	return out
}

func sortedNames(es []*ast.CodeEntity) []string {
	var out []string
	for _, e := range es {
		out = append(out, string(e.Type)+":"+e.Name)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLocalsAreNotEntities pins #331: declarations local to a function, arrow,
// method or static block are not emitted in any form, while module-level
// declarations (under their unchanged IDs) and namespace members are.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestLocalsAreNotEntities(t *testing.T) {
	src := `export const shared = 1;
export function top(): number { return shared; }

describe("first", () => {
  const fetcher = () => 1;
  function inner() { return fetcher(); }
  class Local { run(): void {} }
  let n = inner();
});

describe("second", () => {
  const fetcher = () => 2;
  function inner(): Local2 { return new Local2(); }
  class Local2 {}
});

export class Box {
  static { const seeded = 1; }
  run(): void { const inMethod = 1; }
}

export namespace Ns {
  export const member = 1;
}
`
	got := sortedNames(parseTSFile(t, "scoped.test.ts", src))
	want := []string{"class:Box", "const:member", "const:shared", "function:top", "method:run"}
	if !equalStrings(got, want) {
		t.Fatalf("entities = %v, want %v", got, want)
	}

	// Module-level IDs keep their pre-#331 form.
	for _, e := range parseTSFile(t, "scoped.test.ts", src) {
		if e.Name == "shared" {
			if want := ast.NewCodeEntity(testAuthority("test-org"), "typescript", "test-project", ast.TypeConst, "shared", "scoped.test.ts").ID; e.ID != want {
				t.Errorf("module-level ID = %s, want unchanged %s", e.ID, want)
			}
		}
	}
}

// TestClassMemberFormsGetDistinctStableIDs pins #331's member discriminators:
// accessor pairs and static/instance namesakes get distinct IDs, applied to
// every such member, and keep their source name.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestClassMemberFormsGetDistinctStableIDs(t *testing.T) {
	src := `export class Box {
  private n = 0;
  get size(): number { return this.n; }
  set size(v: number) { this.n = v; }
  static make(): Box { return this.build(); }
  static build(): Box { return new Box(); }
  make(): Box { return this.clone(); }
  clone(): Box { return this; }
}
`
	ents := parseTSFile(t, "box.ts", src)
	ids := map[string]bool{}
	byID := map[string]*ast.CodeEntity{}
	for _, e := range ents {
		if ids[e.ID] {
			t.Errorf("duplicate ID %s", e.ID)
		}
		ids[e.ID] = true
		byID[e.ID] = e
	}
	id := func(instance string) string {
		return ast.NewScopedCodeEntity(testAuthority("test-org"), "typescript", "test-project", ast.TypeMethod, []string{"Box"}, instance, "box.ts").ID
	}
	for instance, name := range map[string]string{
		"size-get": "size", "size-set": "size", "make-static": "make", "build-static": "build", "make": "make", "clone": "clone",
	} {
		e, ok := byID[id(instance)]
		if !ok {
			t.Errorf("no entity under %s", id(instance))
			continue
		}
		if e.Name != name {
			t.Errorf("%s has Name %q, want %q", e.ID, e.Name, name)
		}
	}

	// this.-calls follow the member's form: a static method reaches the static
	// member, an instance method the instance member.
	if calls := byID[id("make-static")].Calls; len(calls) != 1 || calls[0] != id("build-static") {
		t.Errorf("static make calls %v, want [%s]", calls, id("build-static"))
	}
	if calls := byID[id("make")].Calls; len(calls) != 1 || calls[0] != id("clone") {
		t.Errorf("instance make calls %v, want [%s]", calls, id("clone"))
	}
}

// TestModuleLevelBlockDeclarationsAreLocals pins the owner ruling on #329:
// a declaration in a module-level block, loop, try or switch is a local, not
// an entity, so it can neither collide with a sibling block's namesake nor
// displace the module-level declaration of the same name. var is included:
// it hoists in JS, but the ruling scopes identity by block, not by runtime
// binding.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestModuleLevelBlockDeclarationsAreLocals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"if/else branches", "if (A) { const cfg = 1 } else { const cfg = 2 }\n", nil},
		{"block-local beside a module export", "if (globalThis.x) { const cfg = 1; }\nexport const cfg = 2;\n", []string{"const:cfg"}},
		{"try/catch/finally", "try { const t = 1 } catch (e) { const c = e } finally { const f = 1 }\nexport const keep = 1;\n", []string{"const:keep"}},
		{"loops", "for (const tmp of xs) { const inOf = tmp }\nfor (const k in o) { let inIn = k }\nfor (let i = 0; i < 1; i++) { const inFor = i }\nwhile (w) { const inWhile = 1 }\ndo { const inDo = 1 } while (w)\nexport const keep = 1;\n", []string{"const:keep"}},
		{"switch cases", "switch (k) { case 1: const s = 1; break; default: { const d = 2 } }\nexport const keep = 1;\n", []string{"const:keep"}},
		{"bare and labeled blocks", "{ const bare = 1; function inBlock() {} class InBlock {} }\nlbl: { const labeled = 1 }\nexport const keep = 1;\n", []string{"const:keep"}},
		{"var in a block", "if (a) var hoisted = 1;\nif (b) { var hoisted = 2 }\nexport var keep = 1;\n", []string{"var:keep"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := parseTSFile(t, "a.ts", tt.src)
			if got := sortedNames(ents); !equalStrings(got, tt.want) {
				t.Fatalf("entities = %v, want %v", got, tt.want)
			}
			for _, e := range ents {
				if want := ast.NewCodeEntity(testAuthority("test-org"), "typescript", "test-project", e.Type, e.Name, "a.ts").ID; e.ID != want {
					t.Errorf("module-level ID = %s, want unchanged %s", e.ID, want)
				}
			}
		})
	}

	// The surviving cfg is the export on line 2, not the block-local.
	ents := parseTSFile(t, "a.ts", "if (globalThis.x) { const cfg = 1; }\nexport const cfg = 2;\n")
	if len(ents) != 1 || ents[0].StartLine != 2 {
		t.Fatalf("cfg entity = %+v, want the line-2 export", ents)
	}
}

// TestNamespaceMembersAreScopedByNamespaceChain pins the owner ruling on
// #329: a namespace member's ID carries its namespace chain, so namesakes in
// two namespaces, or in a namespace and at module level, stay distinct, while
// module-level IDs are unchanged. Bare calls inside a namespace resolve to the
// namespace's own member, and class members inside one keep this.-resolution.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestNamespaceMembersAreScopedByNamespaceChain(t *testing.T) {
	src := `export namespace A {
  export const v = 1;
  export function g() { return 1; }
  export function caller() { return g(); }
  export class K { run() { return this.stop(); } stop() {} }
}
export namespace B { export const v = 1; export function g() {} }
namespace Outer.Inner { export const deep = 1; }
namespace Outer { namespace Nested { export const n = 1; } }
module Legacy { export function g() {} }
declare namespace Ambient { const amb: number; }
export function g() { return 2; }
export function topCaller() { return g(); }
export const v = 2;
`
	auth := testAuthority("test-org")
	id := func(typ ast.CodeEntityType, scope []string, name string) string {
		return ast.NewScopedCodeEntity(auth, "typescript", "test-project", typ, scope, name, "ns.ts").ID
	}
	want := map[string]bool{
		id(ast.TypeConst, []string{"A"}, "v"):                                                            true,
		id(ast.TypeFunction, []string{"A"}, "g"):                                                         true,
		id(ast.TypeFunction, []string{"A"}, "caller"):                                                    true,
		id(ast.TypeClass, []string{"A"}, "K"):                                                            true,
		id(ast.TypeMethod, []string{"A", "K"}, "run"):                                                    true,
		id(ast.TypeMethod, []string{"A", "K"}, "stop"):                                                   true,
		id(ast.TypeConst, []string{"B"}, "v"):                                                            true,
		id(ast.TypeFunction, []string{"B"}, "g"):                                                         true,
		id(ast.TypeConst, []string{"Outer", "Inner"}, "deep"):                                            true,
		id(ast.TypeConst, []string{"Outer", "Nested"}, "n"):                                              true,
		id(ast.TypeFunction, []string{"Legacy"}, "g"):                                                    true,
		id(ast.TypeConst, []string{"Ambient"}, "amb"):                                                    true,
		ast.NewCodeEntity(auth, "typescript", "test-project", ast.TypeFunction, "g", "ns.ts").ID:         true,
		ast.NewCodeEntity(auth, "typescript", "test-project", ast.TypeFunction, "topCaller", "ns.ts").ID: true,
		ast.NewCodeEntity(auth, "typescript", "test-project", ast.TypeConst, "v", "ns.ts").ID:            true,
	}
	byID := map[string]*ast.CodeEntity{}
	for _, e := range parseTSFile(t, "ns.ts", src) {
		if byID[e.ID] != nil {
			t.Errorf("duplicate ID %s", e.ID)
		}
		byID[e.ID] = e
		if !want[e.ID] {
			t.Errorf("unexpected entity %s", e.ID)
		}
	}
	for w := range want {
		if byID[w] == nil {
			t.Errorf("missing entity %s", w)
		}
	}

	calls := func(e *ast.CodeEntity) []string {
		if e == nil {
			return nil
		}
		return e.Calls
	}
	if got, w := calls(byID[id(ast.TypeFunction, []string{"A"}, "caller")]), id(ast.TypeFunction, []string{"A"}, "g"); len(got) != 1 || got[0] != w {
		t.Errorf("A.caller calls %v, want [%s]", got, w)
	}
	topG := ast.NewCodeEntity(auth, "typescript", "test-project", ast.TypeFunction, "g", "ns.ts").ID
	if got := calls(byID[ast.NewCodeEntity(auth, "typescript", "test-project", ast.TypeFunction, "topCaller", "ns.ts").ID]); len(got) != 1 || got[0] != topG {
		t.Errorf("topCaller calls %v, want [%s]", got, topG)
	}
	if got, w := calls(byID[id(ast.TypeMethod, []string{"A", "K"}, "run")]), id(ast.TypeMethod, []string{"A", "K"}, "stop"); len(got) != 1 || got[0] != w {
		t.Errorf("A.K.run calls %v, want [%s]", got, w)
	}
}

// TestNamespaceTypeReferencesResolveInnermostFirst pins that a type reference
// inside a namespace — extends, implements, a parameter or return annotation —
// targets the namespace's own sibling type, falling back to module level for a
// name the namespace does not declare, and that the same names at module level
// still target the module-level types. Every edge is pinned exactly: a
// dangling target is as wrong as a wrong one.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestNamespaceTypeReferencesResolveInnermostFirst(t *testing.T) {
	src := `export class Base {}
export interface Shape {}
export class Top extends Base implements Shape { take(s: Shape): Base { return new Base(); } }
export function topFn(b: Base): Shape { return b; }
export namespace A {
  export class Base {}
  export interface Shape {}
  export interface Wide extends Shape {}
  export type Id = string;
  export class Kid extends Base implements Shape { take(s: Shape): Base { return new Base(); } }
  export function nsFn(b: Base, i: Id): Shape { return b; }
  export function outer(t: Top): Kid { return new Kid(); }
}
`
	auth := testAuthority("test-org")
	mod := func(typ ast.CodeEntityType, name string) string {
		return ast.NewCodeEntity(auth, "typescript", "test-project", typ, name, "refs.ts").ID
	}
	in := func(typ ast.CodeEntityType, scope []string, name string) string {
		return ast.NewScopedCodeEntity(auth, "typescript", "test-project", typ, scope, name, "refs.ts").ID
	}
	a := []string{"A"}
	byID := map[string]*ast.CodeEntity{}
	for _, e := range parseTSFile(t, "refs.ts", src) {
		byID[e.ID] = e
	}
	type edges struct{ extends, implements, params, returns []string }
	tests := []struct {
		id   string
		want edges
	}{
		{mod(ast.TypeClass, "Top"), edges{extends: []string{mod(ast.TypeClass, "Base")}, implements: []string{mod(ast.TypeInterface, "Shape")}}},
		{in(ast.TypeMethod, []string{"Top"}, "take"), edges{params: []string{mod(ast.TypeInterface, "Shape")}, returns: []string{mod(ast.TypeClass, "Base")}}},
		{mod(ast.TypeFunction, "topFn"), edges{params: []string{mod(ast.TypeClass, "Base")}, returns: []string{mod(ast.TypeInterface, "Shape")}}},
		{in(ast.TypeInterface, a, "Wide"), edges{extends: []string{in(ast.TypeInterface, a, "Shape")}}},
		{in(ast.TypeClass, a, "Kid"), edges{extends: []string{in(ast.TypeClass, a, "Base")}, implements: []string{in(ast.TypeInterface, a, "Shape")}}},
		{in(ast.TypeMethod, []string{"A", "Kid"}, "take"), edges{params: []string{in(ast.TypeInterface, a, "Shape")}, returns: []string{in(ast.TypeClass, a, "Base")}}},
		{in(ast.TypeFunction, a, "nsFn"), edges{params: []string{in(ast.TypeClass, a, "Base"), in(ast.TypeType, a, "Id")}, returns: []string{in(ast.TypeInterface, a, "Shape")}}},
		{in(ast.TypeFunction, a, "outer"), edges{params: []string{mod(ast.TypeClass, "Top")}, returns: []string{in(ast.TypeClass, a, "Kid")}}},
	}
	for _, tt := range tests {
		e := byID[tt.id]
		if e == nil {
			t.Errorf("missing entity %s", tt.id)
			continue
		}
		for _, c := range []struct {
			field     string
			got, want []string
		}{
			{"Extends", e.Extends, tt.want.extends},
			{"Implements", e.Implements, tt.want.implements},
			{"Parameters", e.Parameters, tt.want.params},
			{"Returns", e.Returns, tt.want.returns},
		} {
			if !equalStrings(c.got, c.want) {
				t.Errorf("%s %s = %v, want %v", tt.id, c.field, c.got, c.want)
			}
			for _, target := range c.got {
				if byID[target] == nil {
					t.Errorf("%s %s targets %s, which no entity carries", tt.id, c.field, target)
				}
			}
		}
	}
}

// refEdges is the exact edge set one entity carries.
type refEdges struct{ calls, extends, params, returns []string }

// assertRefEdges pins each entity's edges exactly and requires every target
// to be carried by an entity of the same parse: a dangling target is as wrong
// as a wrong one.
func assertRefEdges(t *testing.T, ents []*ast.CodeEntity, want map[string]refEdges) {
	t.Helper()
	byID := map[string]*ast.CodeEntity{}
	for _, e := range ents {
		byID[e.ID] = e
	}
	for id, w := range want {
		e := byID[id]
		if e == nil {
			t.Errorf("missing entity %s", id)
			continue
		}
		for _, c := range []struct {
			field     string
			got, want []string
		}{
			{"Calls", e.Calls, w.calls},
			{"Extends", e.Extends, w.extends},
			{"Parameters", e.Parameters, w.params},
			{"Returns", e.Returns, w.returns},
		} {
			if !equalStrings(c.got, c.want) {
				t.Errorf("%s %s = %v, want %v", id, c.field, c.got, c.want)
			}
			for _, target := range c.got {
				if byID[target] == nil && !strings.HasPrefix(target, "builtin:") && !strings.HasPrefix(target, "external:") {
					t.Errorf("%s %s targets %s, which no entity carries", id, c.field, target)
				}
			}
		}
	}
}

// TestAmbientTypeReferencesResolveToTheirDeclarations pins that a type
// declared in `declare global {}` is a module-level type for references, and
// one in `declare module "m" {}` resolves within that module's scope.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestAmbientTypeReferencesResolveToTheirDeclarations(t *testing.T) {
	src := `declare global { interface Win { x: number } }
declare module "m" {
  export interface Mod {}
  export interface Sub extends Mod {}
}
export function useWin(w: Win): Win { return w; }
`
	auth := testAuthority("test-org")
	mod := func(typ ast.CodeEntityType, name string) string {
		return ast.NewCodeEntity(auth, "typescript", "test-project", typ, name, "a.ts").ID
	}
	inM := ast.NewScopedCodeEntity(auth, "typescript", "test-project", ast.TypeInterface, []string{"m"}, "Mod", "a.ts").ID
	assertRefEdges(t, parseTSFile(t, "a.ts", src), map[string]refEdges{
		mod(ast.TypeFunction, "useWin"): {params: []string{mod(ast.TypeInterface, "Win")}, returns: []string{mod(ast.TypeInterface, "Win")}},
		ast.NewScopedCodeEntity(auth, "typescript", "test-project", ast.TypeInterface, []string{"m"}, "Sub", "a.ts").ID: {extends: []string{inM}},
	})
}

// TestMergedNamespaceDeclarationsShareExportedMembers pins TypeScript's
// namespace merging for bare names: a call or type reference inside one
// declaration of a namespace chain resolves to an exported member of any
// declaration of that chain or of an enclosing chain, while a non-exported
// member stays visible only inside its own body and the name falls back to
// module level.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestMergedNamespaceDeclarationsShareExportedMembers(t *testing.T) {
	src := `namespace N { export function g() {} function priv() {} }
namespace N { export function f() { return g(); } export function q() { return priv(); } }
namespace M { export interface T {} }
namespace M { export function useT(t: T): T { return t; } }
namespace Outer { export function shared() {} export interface Sh {} function hidden() {} }
namespace Outer.Inner { export function use(s: Sh): void { shared(); hidden(); } }
function g() {}
function priv() {}
function shared() {}
function hidden() {}
interface T {}
interface Sh {}
export function topT(t: T): T { g(); return t; }
`
	auth := testAuthority("test-org")
	id := func(typ ast.CodeEntityType, scope []string, name string) string {
		return ast.NewScopedCodeEntity(auth, "typescript", "test-project", typ, scope, name, "merge.ts").ID
	}
	fn := func(scope []string, name string) string { return id(ast.TypeFunction, scope, name) }
	n, m, outer := []string{"N"}, []string{"M"}, []string{"Outer"}
	assertRefEdges(t, parseTSFile(t, "merge.ts", src), map[string]refEdges{
		fn(n, "f"):                            {calls: []string{fn(n, "g")}},
		fn(n, "q"):                            {calls: []string{fn(nil, "priv")}},
		fn(m, "useT"):                         {params: []string{id(ast.TypeInterface, m, "T")}, returns: []string{id(ast.TypeInterface, m, "T")}},
		fn([]string{"Outer", "Inner"}, "use"): {calls: []string{fn(outer, "shared"), fn(nil, "hidden")}, params: []string{id(ast.TypeInterface, outer, "Sh")}, returns: []string{"builtin:void"}},
		fn(nil, "topT"):                       {calls: []string{fn(nil, "g")}, params: []string{id(ast.TypeInterface, nil, "T")}, returns: []string{id(ast.TypeInterface, nil, "T")}},
	})
}

// TestMergedNamespacePrivatesAreLocals pins the #329 ruling on merged
// namespaces: in a namespace chain declared more than once in a file, each
// body's non-exported members are locals of that body — TypeScript scopes them
// per declaration, and no intrinsic discriminator tells two same-named ones
// apart — so they are not entities, and a reference to one from inside its
// body is inert rather than a call to a dropped ID or a module-level namesake.
// A namespace declared once keeps its private members as entities.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestMergedNamespacePrivatesAreLocals(t *testing.T) {
	merged := `namespace N { function priv() { return 1; } export function a() { return priv(); } }
namespace N { function priv() { return 2; } export function b() { return priv(); } }
`
	tests := []struct {
		name string
		src  string
		want map[string]refEdges // exact entity set (file entity excluded) and edges
	}{
		{"merged privates", merged, map[string]refEdges{
			"N.a": {}, "N.b": {},
		}},
		{"merged privates beside a module namesake", merged + "function priv() { return 3; }\n", map[string]refEdges{
			"N.a": {}, "N.b": {}, ".priv": {},
		}},
		{"single declaration keeps its private", "namespace S { function helper() { return 1; } export function run() { return helper(); } }\nfunction helper() { return 2; }\n", map[string]refEdges{
			"S.helper": {}, "S.run": {calls: []string{"S.helper"}}, ".helper": {},
		}},
		{"merged private type reference is inert", "namespace T { interface P {} export function use(p: P): void {} }\nnamespace T { export interface Q {} }\ninterface P {}\n", map[string]refEdges{
			"T.use": {returns: []string{"builtin:void"}}, "T.Q": {}, ".P": {},
		}},
	}
	auth := testAuthority("test-org")
	// "N.a" names a function or type a in namespace N; ".priv" a module-level one.
	resolve := func(ents []*ast.CodeEntity, short string) string {
		scope, name, _ := strings.Cut(short, ".")
		for _, e := range ents {
			var sc []string
			if scope != "" {
				sc = []string{scope}
			}
			if e.Name == name && e.ID == ast.NewScopedCodeEntity(auth, "typescript", "test-project", e.Type, sc, name, "m.ts").ID {
				return e.ID
			}
		}
		return "missing:" + short
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := parseTSFile(t, "m.ts", tt.src)
			seen := map[string]bool{}
			for _, e := range ents {
				if seen[e.ID] {
					t.Errorf("duplicate ID %s", e.ID)
				}
				seen[e.ID] = true
			}
			if len(ents) != len(tt.want) {
				t.Errorf("entities = %v, want %d", sortedNames(ents), len(tt.want))
			}
			want := map[string]refEdges{}
			for short, w := range tt.want {
				for i, c := range w.calls {
					w.calls[i] = resolve(ents, c)
				}
				want[resolve(ents, short)] = w
			}
			assertRefEdges(t, ents, want)
		})
	}
}

// #340: string module names keep their raw specifier in the ID, so a
// `declare module "foo.bar"` member no longer shares an ID with the same name
// in `namespace foo.bar` or in `declare module "foo/bar"`. Calls inside each
// block still resolve to that block's own member.
// spec: entity-identity-safety / TypeScript module-declaration members are keyed by their raw specifier
func TestStringModuleMembersDoNotShareNamespaceIDs(t *testing.T) {
	src := `declare module "foo.bar" { export function x() {} export function c1() { return x(); } }
declare module "foo/bar" { export function x() {} export function c2() { return x(); } }
namespace foo.bar { export function x() {} export function c3() { return x(); } }
`
	ents := parseTSFile(t, "m.ts", src)
	xs := map[string]*ast.CodeEntity{}
	byName := map[string]*ast.CodeEntity{}
	for _, e := range ents {
		if e.Name == "x" {
			xs[e.ID] = e
		}
		byName[e.Name] = e
	}
	if len(xs) != 3 {
		t.Fatalf("want 3 distinct IDs for x, got %d: %v", len(xs), sortedNames(ents))
	}
	// The dotted namespace keeps its pre-#340 ID.
	auth := testAuthority("test-org")
	nsX := ast.NewScopedCodeEntity(auth, "typescript", "test-project", ast.TypeFunction, []string{"foo", "bar"}, "x", "m.ts").ID
	if xs[nsX] == nil {
		t.Errorf("namespace foo.bar member x lost its ID %s", nsX)
	}
	for _, caller := range []string{"c1", "c2", "c3"} {
		c := byName[caller]
		if c == nil || len(c.Calls) != 1 || xs[c.Calls[0]] == nil {
			t.Fatalf("%s: want one call to a block-local x, got %+v", caller, c)
		}
	}
	if byName["c1"].Calls[0] == byName["c2"].Calls[0] || byName["c2"].Calls[0] == byName["c3"].Calls[0] || byName["c1"].Calls[0] == byName["c3"].Calls[0] {
		t.Errorf("callers resolve x to a shared ID: c1=%s c2=%s c3=%s", byName["c1"].Calls[0], byName["c2"].Calls[0], byName["c3"].Calls[0])
	}
}
