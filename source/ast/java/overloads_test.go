package java

import (
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// callableAt returns the method or constructor entity named name declared on
// line, failing when there is not exactly one — overloads share a name, so the
// line is what tells them apart in a test.
func callableAt(t *testing.T, ents []*ast.CodeEntity, name string, line int) *ast.CodeEntity {
	t.Helper()
	var found []*ast.CodeEntity
	for _, e := range ents {
		if e.Name == name && e.Type == ast.TypeMethod && e.StartLine == line {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one callable %q on line %d, got %d", name, line, len(found))
	}
	return found[0]
}

func callablesNamed(ents []*ast.CodeEntity, name string) []*ast.CodeEntity {
	var out []*ast.CodeEntity
	for _, e := range ents {
		if e.Name == name && e.Type == ast.TypeMethod {
			out = append(out, e)
		}
	}
	return out
}

func assertDistinctIDs(t *testing.T, ents []*ast.CodeEntity, name string, want int) {
	t.Helper()
	got := callablesNamed(ents, name)
	if len(got) != want {
		t.Fatalf("want %d callables named %q, got %d", want, name, len(got))
	}
	ids := make(map[string]bool)
	for _, e := range got {
		if ids[e.ID] {
			t.Fatalf("two %q callables share ID %q", name, e.ID)
		}
		ids[e.ID] = true
	}
}

// --- identity (entity-identity-safety) --------------------------------------

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestJavaOverloadIdentityIsDistinct(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		callee string
		want   int
	}{
		{"parameter types", "  void visit(Text t) {}\n  void visit(Count c) {}\n", "visit", 2},
		{"constructor vs same-named method", "  Foo(int a) {}\n  void Foo(int a) {}\n", "Foo", 2},
		{"array dimensions", "  void m(int v) {}\n  void m(int[] v) {}\n", "m", 2},
		{"c-style array dimensions", "  void m(int v) {}\n  void m(int v[]) {}\n", "m", 2},
		{"type variable bounds", "  <T extends A> void m(T t) {}\n  <T extends B> void m(T t) {}\n", "m", 2},
		{"varargs vs scalar", "  void m(int v) {}\n  void m(int... v) {}\n", "m", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := parseTree(t, map[string]string{
				"a/Foo.java": "package a;\npublic class Foo {\n" + tt.body + "}\n",
			})
			assertDistinctIDs(t, ents, tt.callee, tt.want)
		})
	}
}

// TestJavaOverloadIDIsStable pins the discriminator's inputs: an edit that
// changes neither the kind nor the erased parameter types keeps the ID.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestJavaOverloadIDIsStable(t *testing.T) {
	const base = "package a;\npublic class Foo {\n  void visit(Text t) {}\n  void visit(Count c) {}\n}\n"
	tests := []struct {
		name   string
		line   int
		edited string
	}{
		{"sibling overload added", 3,
			"package a;\npublic class Foo {\n  void visit(Text t) {}\n  void visit(Count c) {}\n  void visit(Sum s) {}\n}\n"},
		{"sibling parameter renamed", 3,
			"package a;\npublic class Foo {\n  void visit(Text t) {}\n  void visit(Count renamed) {}\n}\n"},
		{"own parameter renamed", 3,
			"package a;\npublic class Foo {\n  void visit(Text renamed) {}\n  void visit(Count c) {}\n}\n"},
		{"overload added before it", 4,
			"package a;\npublic class Foo {\n  void visit(Sum s) {}\n  void visit(Text t) {}\n  void visit(Count c) {}\n}\n"},
	}
	before := callableAt(t, parseTree(t, map[string]string{"a/Foo.java": base}), "visit", 3).ID
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after := callableAt(t, parseTree(t, map[string]string{"a/Foo.java": tt.edited}), "visit", tt.line).ID
			if after != before {
				t.Errorf("visit(Text) ID changed: %q -> %q", before, after)
			}
		})
	}
}

// TestJavaOverloadHashErasesTypeArgumentsAndVariables checks the hash sees the
// erased signature: type arguments drop, and a type variable reads as its first
// bound, whether the method or the class declares it.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestJavaOverloadHashErasesTypeArgumentsAndVariables(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{"type arguments",
			"public class Foo {\n  void m(List<String> v) {}\n}\n",
			"public class Foo {\n  void m(List<Integer> v) {}\n}\n"},
		{"method type variable to its bound",
			"public class Foo {\n  <T extends Number> void m(T v) {}\n}\n",
			"public class Foo {\n  void m(Number v) {}\n}\n"},
		{"class type variable to its bound",
			"public class Foo<T extends Number> {\n  void m(T v) {}\n}\n",
			"public class Foo {\n  void m(Number v) {}\n}\n"},
		{"unbounded type variable to Object",
			"public class Foo<T> {\n  void m(T v) {}\n}\n",
			"public class Foo {\n  void m(Object v) {}\n}\n"},
		{"whitespace and annotations",
			"public class Foo {\n  void m(final @Nonnull Map . Entry <K, V> v) {}\n}\n",
			"public class Foo {\n  void m(Map.Entry v) {}\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := callableAt(t, parseTree(t, map[string]string{"a/Foo.java": "package a;\n" + tt.a}), "m", 3).ID
			b := callableAt(t, parseTree(t, map[string]string{"a/Foo.java": "package a;\n" + tt.b}), "m", 3).ID
			if a != b {
				t.Errorf("IDs differ: %q vs %q", a, b)
			}
		})
	}
}

// --- call edges (code-call-graph) -------------------------------------------

// spec: openspec/specs/code-call-graph/spec.md#Requirement: Java calls to overloaded methods resolve to one overload or none
func TestJavaArityCallResolution(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		// want names the target as (callee, line); an empty callee means the
		// caller has no edges at all.
		callee string
		line   int
	}{
		{"arity selects the overload", map[string]string{
			"a/Reader.java": "package a;\npublic class Reader {\n" +
				"  void read(InputStream in) {}\n" +
				"  void read(InputStream in, int n) {}\n" +
				"  void caller(InputStream in) { read(in, 4); }\n}\n",
		}, "read", 4},
		{"same-arity overloads emit no edge", map[string]string{
			"a/V.java": "package a;\npublic class V {\n" +
				"  void visit(Text t) {}\n" +
				"  void visit(Count c) {}\n" +
				"  void caller(Text x) { visit(x); }\n}\n",
		}, "", 0},
		{"inherited overload selected by arity", map[string]string{
			"a/Base.java": "package a;\npublic class Base {\n  // foo sits on line 4\n  void foo(int a, int b) {}\n}\n",
			"a/Sub.java": "package a;\npublic class Sub extends Base {\n" +
				"  void foo(int a) {}\n" +
				"  void caller() { foo(1, 2); }\n}\n",
		}, "foo", 4},
		{"single candidate with the wrong arity emits no edge", map[string]string{
			"a/Items.java": "package a;\nimport org.ext.ExtList;\npublic class Items extends ExtList {\n" +
				"  void add(Item i) {}\n}\n",
			"a/Svc.java": "package a;\npublic class Svc {\n" +
				"  void caller(Items items, Item item) { items.add(0, item); }\n}\n",
		}, "", 0},
		{"unique in-repo match links past an external ancestor", map[string]string{
			"a/Err.java": "package a;\npublic class Err extends Exception {\n" +
				"  void report(String msg) {}\n}\n",
			"a/Svc.java": "package a;\npublic class Svc {\n" +
				"  void caller(Err err, String msg) { err.report(msg); }\n}\n",
		}, "report", 3},
		{"constructor overloads follow the same rule", map[string]string{
			"a/OWSException.java": "package a;\npublic class OWSException extends Exception {\n" +
				"  OWSException(String msg) {}\n" +
				"  OWSException(String msg, Throwable e) {}\n}\n",
			"a/Svc.java": "package a;\npublic class Svc {\n" +
				"  void caller(String msg, Throwable e) { throw new OWSException(msg, e); }\n}\n",
		}, "OWSException", 4},
		{"constructor arity with no match emits no edge", map[string]string{
			"a/Repo.java": "package a;\npublic class Repo {\n  Repo(String path) {}\n}\n",
			"a/Svc.java":  "package a;\npublic class Svc {\n  void caller() { new Repo(); }\n}\n",
		}, "", 0},
		{"varargs accepts k-1 or more", map[string]string{
			"a/Log.java": "package a;\npublic class Log {\n" +
				"  void log(String fmt, Object... args) {}\n" +
				"  void caller() { log(\"a\", 1, /* c */ 2, 3); }\n}\n",
		}, "log", 3},
		{"comments are not arguments", map[string]string{
			"a/C.java": "package a;\npublic class C {\n" +
				"  void m() {}\n" +
				"  void m(int a) {}\n" +
				"  void caller() { m(/* none */); }\n}\n",
		}, "m", 3},
		{"override: nearest declaration wins", map[string]string{
			"a/Base.java": "package a;\npublic class Base {\n  // run sits on line 4\n  void run(int a) {}\n}\n",
			"a/Sub.java": "package a;\npublic class Sub extends Base {\n" +
				"  void run(int a) {}\n" +
				"  void caller() { run(1); }\n}\n",
		}, "run", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := parseTree(t, tt.files)
			if tt.callee == "" {
				assertNoCalls(t, ents, "caller")
				return
			}
			target := callableAt(t, ents, tt.callee, tt.line)
			assertCallsExactly(t, ents, "caller", target.ID)
		})
	}
}
