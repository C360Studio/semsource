package c

import (
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func entitiesNamed(res *ast.ParseResult, name string) []*ast.CodeEntity {
	var out []*ast.CodeEntity
	for _, e := range res.Entities {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestPrototypeFoldsOntoDefinition(t *testing.T) {
	res, _ := parse(t, "f.c", `static int f(int x);
static int g(int x) { return x; }
static int f(int x) { return g(x); }
`)
	got := entitiesNamed(res, "f")
	if len(got) != 1 {
		t.Fatalf("want one entity for f, got %d", len(got))
	}
	if got[0].StartLine != 3 || len(got[0].Calls) == 0 {
		t.Errorf("f kept line %d with %d calls; want the definition (line 3) and its call to g", got[0].StartLine, len(got[0].Calls))
	}
	if n := countID(res.FileEntity.Contains, got[0].ID); n != 1 {
		t.Errorf("file lists f %d times in contains, want 1", n)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestPreprocessorTwinsFoldOntoFirst(t *testing.T) {
	res, _ := parse(t, "console.c", `#ifdef _WIN32
#define SEP '\\'
static void move_cursor(int x) { (void)x; }
#else
#define SEP '/'
static void move_cursor(int x) { (void)x; }
#endif
`)
	for name, line := range map[string]int{"move_cursor": 3, "SEP": 2} {
		got := entitiesNamed(res, name)
		if len(got) != 1 || got[0].StartLine != line {
			t.Errorf("%s: want one entity at line %d, got %d entities", name, line, len(got))
		}
	}
}

// A prototype with no definition in the file stays: a header-only API is
// declared this way and would otherwise index as empty.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestLonePrototypeIsKept(t *testing.T) {
	res, _ := parse(t, "api.h", "int api_open(const char *path);\n")
	if got := entitiesNamed(res, "api_open"); len(got) != 1 {
		t.Fatalf("want the prototype kept, got %d entities", len(got))
	}
}

func countID(ids []string, id string) int {
	n := 0
	for _, x := range ids {
		if x == id {
			n++
		}
	}
	return n
}
