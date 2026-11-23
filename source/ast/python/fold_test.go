package python

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parseSource(t *testing.T, src string) *ast.ParseResult {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "m.py")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := NewParser(testAuthority("acme"), "proj", root).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return res
}

func named(res *ast.ParseResult, name string) []*ast.CodeEntity {
	var out []*ast.CodeEntity
	for _, e := range res.Entities {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestPropertyAccessorsGetDistinctIDs(t *testing.T) {
	res := parseSource(t, `class T:
    @property
    def x(self):
        return 1

    @x.setter
    def x(self, v):
        pass

    @x.deleter
    def x(self):
        pass
`)
	got := named(res, "x")
	if len(got) != 3 {
		t.Fatalf("want getter, setter and deleter, got %d entities", len(got))
	}
	for i, suffix := range []string{"-T-x", "-T-x-setter", "-T-x-deleter"} {
		if !strings.HasSuffix(got[i].ID, suffix) {
			t.Errorf("accessor %d has ID %s, want suffix %s", i, got[i].ID, suffix)
		}
	}
	class := named(res, "T")[0]
	if len(class.Contains) != 3 {
		t.Errorf("class contains %v, want the three accessor IDs", class.Contains)
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestOverloadStubsGiveWayToImplementation(t *testing.T) {
	res := parseSource(t, `from typing import overload
import typing

@overload
def f(x: int) -> int: ...
@overload
def f(x: str) -> str: ...
def f(x):
    return x

@typing.overload
def g(x: int) -> int: ...
@typing.overload
def g(x: str) -> str: ...
`)
	if got := named(res, "f"); len(got) != 1 || got[0].StartLine != 8 {
		t.Errorf("f: want only the implementation at line 8, got %d entities", len(got))
	}
	if got := named(res, "g"); len(got) != 1 || got[0].StartLine != 12 {
		t.Errorf("g: want only the first stub at line 12, got %d entities", len(got))
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestRedefinitionKeepsLastBinding(t *testing.T) {
	res := parseSource(t, `class P:
    def predict(self, f):
        return f

    def predict(self, f, threshold=0.5):
        return f
`)
	if got := named(res, "predict"); len(got) != 1 || got[0].StartLine != 5 {
		t.Errorf("predict: want only the second definition at line 5, got %d entities", len(got))
	}
}
