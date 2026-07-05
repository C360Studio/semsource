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
