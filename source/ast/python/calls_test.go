package python

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parsePyFiles(t *testing.T, files map[string]string) map[string]*ast.CodeEntity {
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
	byName := make(map[string]*ast.CodeEntity)
	p := NewParser("acme", "proj", root)
	for rel := range files {
		res, err := p.ParseFile(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, e := range res.Entities {
			byName[e.Name] = e
		}
	}
	return byName
}

func hasCall(e *ast.CodeEntity, id string) bool {
	for _, c := range e.Calls {
		if c == id {
			return true
		}
	}
	return false
}

// TestSameFileFunctionCall — a call to a same-file module-level function resolves
// to that function's own entity ID (ref-id == def-id, not substring).
func TestSameFileFunctionCall(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "def helper():\n    pass\n\ndef run():\n    helper()\n",
	})
	helper, run := ents["helper"], ents["run"]
	if helper == nil || run == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if !hasCall(run, helper.ID) {
		t.Errorf("run.Calls = %v, want to contain helper %q", run.Calls, helper.ID)
	}
}

// TestCrossFileImportedCall — `from pkg.util import helper` then `helper()` resolves
// to helper's definition in pkg/util.py.
func TestCrossFileImportedCall(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"pkg/__init__.py": "",
		"pkg/util.py":     "def helper():\n    pass\n",
		"pkg/app.py":      "from pkg.util import helper\n\ndef run():\n    helper()\n",
	})
	helper, run := ents["helper"], ents["run"]
	if helper == nil || run == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if !hasCall(run, helper.ID) {
		t.Errorf("cross-file call: run.Calls = %v, want to contain helper %q", run.Calls, helper.ID)
	}
}

// TestModuleQualifiedImportedCall — `import pkg.util` then `pkg.util.helper()`.
func TestModuleQualifiedImportedCall(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"pkg/__init__.py": "",
		"pkg/util.py":     "def helper():\n    pass\n",
		"pkg/app.py":      "import pkg.util\n\ndef run():\n    pkg.util.helper()\n",
	})
	helper, run := ents["helper"], ents["run"]
	if helper == nil || run == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if !hasCall(run, helper.ID) {
		t.Errorf("module-qualified call: run.Calls = %v, want to contain helper %q", run.Calls, helper.ID)
	}
}

// TestSelfMethodCall — `self.b()` inside a class resolves to sibling method b's
// scoped entity ID.
func TestSelfMethodCall(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "class Svc:\n    def a(self):\n        self.b()\n\n    def b(self):\n        pass\n",
	})
	a, b := ents["a"], ents["b"]
	if a == nil || b == nil {
		t.Fatalf("missing methods: %v", ents)
	}
	if !hasCall(a, b.ID) {
		t.Errorf("a.Calls = %v, want to contain method b %q", a.Calls, b.ID)
	}
}

// TestImportedClassInstantiationIsInert — `from pkg.base import Base; Base()` must
// NOT emit a call edge: Base is a class, not a function, so a `.function.` target
// would be a phantom. Confirms the fail-inert guard (moduleFuncs confirmation).
func TestImportedClassInstantiationIsInert(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"pkg/__init__.py": "",
		"pkg/base.py":     "class Base:\n    pass\n",
		"pkg/app.py":      "from pkg.base import Base\n\ndef run():\n    return Base()\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (imported class instantiation is inert)", run.Calls)
	}
}

// TestSelfInheritedMethodIsInert — `self.missing()` where the method is not defined
// on this class (inherited/mixin/typo) must not fabricate a scoped-method edge.
func TestSelfInheritedMethodIsInert(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "class Svc:\n    def a(self):\n        self.missing()\n",
	})
	a := ents["a"]
	if a == nil {
		t.Fatalf("missing method a: %v", ents)
	}
	if len(a.Calls) != 0 {
		t.Errorf("a.Calls = %v, want empty (method not defined on this class is inert)", a.Calls)
	}
}

// --- local-value shadow suppression (spec: function-typed parameters/locals
// never become callees) -------------------------------------------------------

// TestParamShadowsModuleLevelFunctionIsInert — the reviewer's motivating shape:
// `transform` is a PARAMETER of run, so calling it must not resolve against
// the unrelated module-level `transform` function of the same name — that
// would be a call through the parameter's VALUE, not a reference to the
// definition.
func TestParamShadowsModuleLevelFunctionIsInert(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "def transform(x):\n    return x\n\ndef run(transform):\n    transform(5)\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (transform is a parameter, not the module-level function)", run.Calls)
	}
}

// TestNoParamShadowStillResolves is the control for the previous test: remove
// the shadowing parameter and an equivalent call (here inside a list
// comprehension, to also confirm the walk still reaches comprehension bodies)
// must resolve — proving the suppression is surgical to an actual shadow.
func TestNoParamShadowStillResolves(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "def transform(x):\n    return x\n\ndef run(items):\n    return [transform(i) for i in items]\n",
	})
	transform, run := ents["transform"], ents["run"]
	if transform == nil || run == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if !hasCall(run, transform.ID) {
		t.Errorf("run.Calls = %v, want to contain transform %q", run.Calls, transform.ID)
	}
}

// TestLocalReassignmentShadowsModuleFunction — a local assignment MID-BODY
// (not a parameter) shadows the same way: reassigning `transform` to a lambda
// rebinds the name to a local value for the rest of the function.
func TestLocalReassignmentShadowsModuleFunction(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "def transform(x):\n    return x\n\ndef run():\n    transform = lambda x: x * 2\n    transform(5)\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (transform is reassigned to a local lambda)", run.Calls)
	}
}

// TestTupleUnpackShadowsModuleFunction exercises collectPyAssignTargets'
// pattern-unpacking path, not just the plain-identifier fast path.
func TestTupleUnpackShadowsModuleFunction(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "def transform(x):\n    return x\n\ndef run():\n    transform, other = get_pair()\n    transform(5)\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (transform is tuple-unpacked to a local)", run.Calls)
	}
}

// TestParamShadowsImportIsInert covers the cross-file case: `transform` is
// imported from pkg.util AND shadowed by run's own parameter of the same
// name — the parameter must win, not the import.
func TestParamShadowsImportIsInert(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"pkg/__init__.py": "",
		"pkg/util.py":     "def transform(x):\n    return x\n",
		"pkg/app.py":      "from pkg.util import transform\n\ndef run(transform):\n    transform(5)\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (transform is a parameter, not the imported function)", run.Calls)
	}
}

// TestParamShadowsModuleAliasSuppressesExternalMarker — the module-qualified
// form of the same bug: `os` shadowed by a parameter must not be treated as
// the imported module of the same name, and must not even produce the
// "external:" marker a genuine os.getcwd() call would (TestExternalAndInertCalls
// below pins that unshadowed case).
func TestParamShadowsModuleAliasSuppressesExternalMarker(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "import os\n\ndef run(os):\n    os.getcwd()\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if len(run.Calls) != 0 {
		t.Errorf("run.Calls = %v, want empty (os is a parameter, not the imported module)", run.Calls)
	}
}

// TestExternalAndInertCalls — an out-of-tree module call stays external; a builtin
// and a bare undefined name emit no call edge.
func TestExternalAndInertCalls(t *testing.T) {
	ents := parsePyFiles(t, map[string]string{
		"m.py": "import os\n\ndef run(items):\n    os.getcwd()\n    len(items)\n    undefined_thing()\n",
	})
	run := ents["run"]
	if run == nil {
		t.Fatalf("missing run: %v", ents)
	}
	if !hasCall(run, "external:os.getcwd") {
		t.Errorf("run.Calls = %v, want to contain external:os.getcwd", run.Calls)
	}
	for _, c := range run.Calls {
		if c != "external:os.getcwd" {
			t.Errorf("unexpected inert call edge %q in %v (builtin/undefined should be dropped)", c, run.Calls)
		}
	}
}
