package svelte

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// TestSvelteScriptLocalsAreNotEntities applies #331's local rule to a
// component script: only the script's top-level declarations are entities.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestSvelteScriptLocalsAreNotEntities(t *testing.T) {
	src := `<script lang="ts">
  const count = 1;
  function handle() {
    const inner = 2;
    return inner;
  }
  const onClick = () => { let local = 3; return local; };
</script>
<button on:click={onClick}>{count}</button>
`
	got := names(parseSvelte(t, src))
	sort.Strings(got)
	want := []string{"count"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("const/var entities = %v, want %v", got, want)
	}
}

// TestSvelteScriptBlockLocalsAndNamespaceMembers applies the #329 rulings to a
// component script: a module-level block's declaration is a local, and a
// namespace member is scoped by its namespace, so neither collides with the
// script's top-level namesake.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestSvelteScriptBlockLocalsAndNamespaceMembers(t *testing.T) {
	src := `<script lang="ts">
  if (globalThis.x) { const cfg = 1; }
  for (const tmp of [1]) { const inLoop = tmp; }
  namespace N { export const cfg = 3; }
  const cfg = 2;
</script>
<p>{cfg}</p>
`
	auth := testAuthority("test-org")
	want := map[string]bool{
		ast.NewCodeEntity(auth, "svelte", "test-project", ast.TypeConst, "cfg", "Component.svelte").ID:                      true,
		ast.NewScopedCodeEntity(auth, "svelte", "test-project", ast.TypeConst, []string{"N"}, "cfg", "Component.svelte").ID: true,
	}
	got := parseSvelte(t, src)
	if len(got) != len(want) {
		t.Fatalf("const/var entities = %v, want %d", names(got), len(want))
	}
	lines := map[string]int{}
	for _, e := range got {
		if !want[e.ID] {
			t.Errorf("unexpected entity %s (line %d)", e.ID, e.StartLine)
		}
		lines[e.ID] = e.StartLine
	}
	// The unscoped cfg is the declaration one line below the namespace, not
	// the block-local. Lines are compared relatively because script entities
	// carry script-relative line numbers.
	top := ast.NewCodeEntity(auth, "svelte", "test-project", ast.TypeConst, "cfg", "Component.svelte").ID
	ns := ast.NewScopedCodeEntity(auth, "svelte", "test-project", ast.TypeConst, []string{"N"}, "cfg", "Component.svelte").ID
	if lines[top] != lines[ns]+1 {
		t.Errorf("top-level cfg at line %d, want %d (the line after namespace N)", lines[top], lines[ns]+1)
	}
}

// TestSvelteNamespaceCallsResolveToNamespaceMember applies the TS parser's
// namespace call rule to a component script: a bare call inside a namespace
// targets the namespace's own member, including an exported member of another
// declaration of the same namespace, and a top-level call the top-level one.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestSvelteNamespaceCallsResolveToNamespaceMember(t *testing.T) {
	src := `<script lang="ts">
  namespace N {
    export function g() { return 1; }
    export function caller() { return g(); }
  }
  namespace N { export function merged() { return g(); } }
  function g() { return 2; }
  function topCaller() { return g(); }
</script>
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Component.svelte")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewParser(testAuthority("test-org"), "test-project", dir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	auth := testAuthority("test-org")
	fn := func(scope []string, name string) string {
		return ast.NewScopedCodeEntity(auth, "svelte", "test-project", ast.TypeFunction, scope, name, "Component.svelte").ID
	}
	byID := map[string]*ast.CodeEntity{}
	for _, e := range result.Entities {
		byID[e.ID] = e
	}
	for caller, want := range map[string]string{
		fn([]string{"N"}, "caller"): fn([]string{"N"}, "g"),
		fn(nil, "topCaller"):        fn(nil, "g"),
		fn([]string{"N"}, "merged"): fn([]string{"N"}, "g"),
	} {
		e := byID[caller]
		if e == nil {
			t.Errorf("missing entity %s", caller)
			continue
		}
		if len(e.Calls) != 1 || e.Calls[0] != want {
			t.Errorf("%s calls %v, want [%s]", caller, e.Calls, want)
		}
		if byID[want] == nil {
			t.Errorf("call target %s is carried by no entity", want)
		}
	}
}

// TestSvelteMergedNamespacePrivatesAreLocals applies the merged-namespace
// private rule through the Svelte shared resolver: a merged body's private is
// not an entity and calls to it are inert, never reaching a top-level
// namesake, while a single-declaration namespace keeps its private.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestSvelteMergedNamespacePrivatesAreLocals(t *testing.T) {
	src := `<script lang="ts">
  namespace N { function priv() { return 1; } export function a() { return priv(); } }
  namespace N { function priv() { return 2; } export function b() { return priv(); } }
  namespace S { function helper() { return 1; } export function run() { return helper(); } }
  function priv() { return 3; }
</script>
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Component.svelte")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewParser(testAuthority("test-org"), "test-project", dir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	auth := testAuthority("test-org")
	fn := func(scope []string, name string) string {
		return ast.NewScopedCodeEntity(auth, "svelte", "test-project", ast.TypeFunction, scope, name, "Component.svelte").ID
	}
	want := map[string][]string{
		fn([]string{"N"}, "a"):      nil,
		fn([]string{"N"}, "b"):      nil,
		fn([]string{"S"}, "helper"): nil,
		fn([]string{"S"}, "run"):    {fn([]string{"S"}, "helper")},
		fn(nil, "priv"):             nil,
	}
	got := map[string][]string{}
	for _, e := range result.Entities {
		if e.Type != ast.TypeFunction {
			continue
		}
		if _, dup := got[e.ID]; dup {
			t.Errorf("duplicate ID %s", e.ID)
		}
		got[e.ID] = e.Calls
	}
	if len(got) != len(want) {
		t.Errorf("function entities = %d, want %d: %v", len(got), len(want), got)
	}
	for id, calls := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("missing entity %s", id)
			continue
		}
		if len(g) != len(calls) || (len(calls) == 1 && g[0] != calls[0]) {
			t.Errorf("%s calls %v, want %v", id, g, calls)
		}
	}
}
