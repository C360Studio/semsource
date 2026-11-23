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

    @x.setter  # noqa: E501
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
@overload  # type: ignore[misc]
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

// Review #356: the decorator is matched on its syntax, so valid spellings with
// spacing or parentheses still discriminate, and the getter survives.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestAccessorDecoratorSpellings(t *testing.T) {
	res := parseSource(t, `class T:
    @property
    def x(self):
        return 1

    @x . setter
    def x(self, v):
        pass

    @(x.deleter)
    def x(self):
        pass
`)
	got := named(res, "x")
	want := []struct {
		suffix string
		line   int
	}{{"-T-x", 3}, {"-T-x-setter", 7}, {"-T-x-deleter", 11}}
	if len(got) != len(want) {
		t.Fatalf("want getter, setter and deleter, got %d entities", len(got))
	}
	for i, w := range want {
		if !strings.HasSuffix(got[i].ID, w.suffix) || got[i].StartLine != w.line {
			t.Errorf("accessor %d: ID %s line %d, want suffix %s line %d", i, got[i].ID, got[i].StartLine, w.suffix, w.line)
		}
	}
}

// Review #356: a rebound class keeps only the later body's members.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestReboundClassDropsShadowedMembers(t *testing.T) {
	res := parseSource(t, `class T:
    def old(self):
        pass

    def shared(self):
        return 1


class T:
    def shared(self):
        return 2

    def new(self):
        pass
`)
	if got := named(res, "old"); len(got) != 0 {
		t.Errorf("old: the shadowed class's method survived (%d entities)", len(got))
	}
	shared := named(res, "shared")
	if len(shared) != 1 || shared[0].StartLine != 10 {
		t.Fatalf("shared: want only the later body's method at line 10, got %d", len(shared))
	}
	classes := named(res, "T")
	if len(classes) != 1 || classes[0].StartLine != 9 {
		t.Fatalf("T: want only the later class at line 9, got %d", len(classes))
	}
	newM := named(res, "new")
	if len(newM) != 1 {
		t.Fatalf("new: want one entity, got %d", len(newM))
	}
	if want := []string{shared[0].ID, newM[0].ID}; strings.Join(classes[0].Contains, ",") != strings.Join(want, ",") {
		t.Errorf("T contains %v, want exactly %v", classes[0].Contains, want)
	}
}
