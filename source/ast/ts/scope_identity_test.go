package ts

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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
// declarations and namespace members are, under their unchanged IDs.
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
