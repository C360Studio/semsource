package golang

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// TestPackageScan_FuncsHarvested — one directory scan yields package-level
// funcs (name → defining relPath): methods and _test.go declarations are
// excluded, and a sibling edit invalidates the cached entry (D1).
func TestPackageScan_FuncsHarvested(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\nfunc Alpha() {}\n")
	write("b.go", "package p\ntype M struct{}\nfunc (M) Beta() {}\n")
	write("c_test.go", "package p\nfunc TestOnly() {}\n")

	p := NewParser("acme", "proj", root)
	funcs := p.packageFuncs(".")
	if funcs["Alpha"] != "a.go" {
		t.Errorf("funcs[Alpha] = %q, want a.go", funcs["Alpha"])
	}
	if _, ok := funcs["Beta"]; ok {
		t.Error("method Beta harvested as a package-level func")
	}
	if _, ok := funcs["TestOnly"]; ok {
		t.Error("_test.go func harvested")
	}

	// A sibling edit (size change) must invalidate the cached scan.
	write("b.go", "package p\ntype M struct{}\nfunc (M) Beta() {}\nfunc Gamma() {}\n")
	if got := p.packageFuncs(".")["Gamma"]; got != "b.go" {
		t.Errorf("after sibling edit, funcs[Gamma] = %q, want b.go", got)
	}
}

// TestGoParser_CrossFileSamePackageCall — an unqualified call to a sibling-file
// func byte-matches the definition's entity ID; same-file resolution is
// unchanged; an undefined name stays inert against the caller's own path.
func TestGoParser_CrossFileSamePackageCall(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"def.go":    "package p\nfunc Callee() {}\nfunc Local() { Callee() }\n",
		"caller.go": "package p\nfunc Caller() { Callee(); Undefined() }\n",
	})
	callee, caller, local := ents["Callee"], ents["Caller"], ents["Local"]
	if callee == nil || caller == nil || local == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if !slices.Contains(caller.Calls, callee.ID) {
		t.Errorf("cross-file call: Caller.Calls = %v, want to contain %s", caller.Calls, callee.ID)
	}
	if !slices.Contains(local.Calls, callee.ID) {
		t.Errorf("same-file call: Local.Calls = %v, want to contain %s", local.Calls, callee.ID)
	}
	inert := ast.NewCodeEntity("acme", "golang", "proj", ast.TypeFunction, "Undefined", "caller.go").ID
	if !slices.Contains(caller.Calls, inert) {
		t.Errorf("undefined call: Caller.Calls = %v, want inert %s", caller.Calls, inert)
	}
}

// TestGoParser_ModuleMapping — nearest-go.mod resolution: root module, nested
// module, no module at all, and invalidation on a go.mod edit (D2).
func TestGoParser_ModuleMapping(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, src string) {
		t.Helper()
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("go.mod", "module example.com/root\n\ngo 1.22\n")
	mk("sub/go.mod", "module example.com/other // nested\n")
	mk("pkg/x/x.go", "package x\n")

	p := NewParser("acme", "proj", root)
	if o := p.moduleOrigin("pkg/x"); o.modulePath != "example.com/root" || o.moduleDir != "." {
		t.Errorf("pkg/x origin = %+v, want root module", o)
	}
	if o := p.moduleOrigin("sub"); o.modulePath != "example.com/other" || o.moduleDir != "sub" {
		t.Errorf("sub origin = %+v, want nested module", o)
	}

	if dir, ok := p.inRepoImportDir("example.com/root/pkg/x", "cmd/main.go"); !ok || dir != "pkg/x" {
		t.Errorf("in-repo mapping = %q,%v, want pkg/x,true", dir, ok)
	}
	if _, ok := p.inRepoImportDir("example.com/rootlike/pkg", "cmd/main.go"); ok {
		t.Error("prefix-lookalike module mapped in-repo")
	}
	if _, ok := p.inRepoImportDir("golang.org/x/mod", "cmd/main.go"); ok {
		t.Error("external import mapped in-repo")
	}

	// A go.mod edit invalidates the cached mapping.
	mk("go.mod", "module example.com/renamed-root\n\ngo 1.22\n")
	if o := p.moduleOrigin("pkg/x"); o.modulePath != "example.com/renamed-root" {
		t.Errorf("after go.mod edit, modulePath = %q, want example.com/renamed-root", o.modulePath)
	}

	bare := NewParser("acme", "proj", t.TempDir())
	if o := bare.moduleOrigin("."); o.modulePath != "" {
		t.Errorf("no-go.mod origin = %+v, want empty", o)
	}
}

// TestGoParser_CrossPackageInRepoCall — a qualified call whose import path lies
// in this module resolves to the defining entity; the standard library stays
// external; a type conversion (no FuncDecl) stays external (D2).
func TestGoParser_CrossPackageInRepoCall(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"go.mod":       "module example.com/app\n",
		"util/util.go": "package util\ntype Widget int\nfunc Sanitize() {}\n",
		"svc/svc.go": "package svc\n\nimport (\n\t\"strings\"\n\n\t\"example.com/app/util\"\n)\n\n" +
			"func Run() {\n\tutil.Sanitize()\n\t_ = strings.Contains(\"a\", \"b\")\n\t_ = util.Widget(1)\n}\n",
	})
	run := ents["Run"]
	if run == nil {
		t.Fatalf("missing Run: %v", ents)
	}
	want := ast.NewCodeEntity("acme", "golang", "proj", ast.TypeFunction, "Sanitize", "util/util.go").ID
	if !slices.Contains(run.Calls, want) {
		t.Errorf("in-repo qualified call: Calls = %v, want to contain %s", run.Calls, want)
	}
	if !slices.Contains(run.Calls, "external:strings.Contains") {
		t.Errorf("stdlib call: Calls = %v, want external:strings.Contains", run.Calls)
	}
	if !slices.Contains(run.Calls, "external:example.com/app/util.Widget") {
		t.Errorf("type conversion: Calls = %v, want external marker, never a guessed edge", run.Calls)
	}
}

// TestGoParser_LocalVarShadowsImportAlias — a method call on a local value
// whose name shadows an import alias must not become an in-repo call edge (D3).
func TestGoParser_LocalVarShadowsImportAlias(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"go.mod":           "module example.com/app\n",
		"client/client.go": "package client\nfunc Get() {}\n",
		"svc/svc.go": "package svc\n\nimport \"example.com/app/client\"\n\n" +
			"type fetcher struct{}\nfunc (fetcher) Get() string { return \"\" }\n\n" +
			"func Run() {\n\tclient := fetcher{}\n\t_ = client.Get()\n}\n",
	})
	run := ents["Run"]
	if run == nil {
		t.Fatalf("missing Run: %v", ents)
	}
	resolved := ast.NewCodeEntity("acme", "golang", "proj", ast.TypeFunction, "Get", "client/client.go").ID
	if slices.Contains(run.Calls, resolved) {
		t.Errorf("shadowed alias produced a wrong in-repo edge: %v", run.Calls)
	}
	if slices.Contains(run.Calls, "external:example.com/app/client.Get") {
		t.Errorf("shadowed alias treated as package-qualified: %v", run.Calls)
	}
	if !slices.Contains(run.Calls, "client.Get") {
		t.Errorf("method call lost its inert raw form: %v", run.Calls)
	}
}
