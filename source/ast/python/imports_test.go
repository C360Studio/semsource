package python

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/c360studio/semsource/source/ast"
)

// parseRoot parses Python source and returns its root node + bytes for the
// binding-extraction unit tests.
func parseRoot(t *testing.T, src string) (*sitter.Node, []byte) {
	t.Helper()
	p := sitter.NewParser()
	p.SetLanguage(python.GetLanguage())
	tree, err := p.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode(), []byte(src)
}

func TestExtractImportBindings(t *testing.T) {
	src := `import pkg.base
import pkg.util as u
from pkg.models import Client
from pkg.models import Response as Resp
from .local import Thing
from ..shared import Widget
from os import *
`
	root, content := parseRoot(t, src)
	got := extractImportBindings(root, content)

	want := map[string]importBinding{
		"pkg.base": {module: "pkg.base"},
		"u":        {module: "pkg.util"},
		"Client":   {module: "pkg.models", origin: "Client"},
		"Resp":     {module: "pkg.models", origin: "Response"},
		"Thing":    {module: "local", origin: "Thing", level: 1},
		"Widget":   {module: "shared", origin: "Widget", level: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("binding count = %d, want %d: %+v", len(got), len(want), got)
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("missing binding %q", k)
			continue
		}
		if g != w {
			t.Errorf("binding %q = %+v, want %+v", k, g, w)
		}
	}
	// Star import must NOT create a binding.
	if _, ok := got["*"]; ok {
		t.Error("star import should not produce a binding")
	}
}

func TestModuleToRelPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "pkg/base.py", "class BaseClient: pass\n")
	mustWrite(t, root, "pkg/sub/__init__.py", "")
	mustWrite(t, root, "pkg/client.py", "")

	p := NewParser("acme", "proj", root)

	cases := []struct {
		name    string
		module  string
		fromRel string
		level   int
		wantRel string
		wantOK  bool
	}{
		{"absolute .py", "pkg.base", "app/main.py", 0, "pkg/base.py", true},
		{"absolute __init__", "pkg.sub", "app/main.py", 0, filepath.Join("pkg", "sub", "__init__.py"), true},
		{"relative level 1", "base", "pkg/client.py", 1, "pkg/base.py", true},
		{"out of tree", "numpy", "app/main.py", 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRel, gotOK := p.moduleToRelPath(tc.module, tc.fromRel, tc.level)
			if gotOK != tc.wantOK || (tc.wantOK && gotRel != tc.wantRel) {
				t.Errorf("moduleToRelPath(%q, %q, %d) = (%q, %v), want (%q, %v)",
					tc.module, tc.fromRel, tc.level, gotRel, gotOK, tc.wantRel, tc.wantOK)
			}
		})
	}
}

// TestParseFile_CrossFileInheritance is the task #44 end-to-end regression: a
// subclass importing its base from ANOTHER module must resolve its extends edge to
// that base class's real entity id (not a dangling referrer-local id).
func TestParseFile_CrossFileInheritance(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "pkg/__init__.py", "")
	basePath := mustWrite(t, root, "pkg/base.py", "class BaseClient:\n    pass\n")
	clientPath := mustWrite(t, root, "pkg/client.py",
		"from pkg.base import BaseClient\n\nclass AsyncClient(BaseClient):\n    pass\n")

	p := NewParser("acme", "proj", root)

	baseRes, err := p.ParseFile(context.Background(), basePath)
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	clientRes, err := p.ParseFile(context.Background(), clientPath)
	if err != nil {
		t.Fatalf("parse client: %v", err)
	}

	baseID := classID(t, baseRes, "BaseClient")
	async := findClass(clientRes, "AsyncClient")
	if async == nil {
		t.Fatal("AsyncClient not found")
	}
	if len(async.Extends) != 1 {
		t.Fatalf("Extends = %v, want 1", async.Extends)
	}
	if async.Extends[0] != baseID {
		t.Errorf("cross-file extends = %q, want %q (BaseClient in pkg/base.py)", async.Extends[0], baseID)
	}
}

// TestParseFile_CrossFileInheritance_OrderIndependent covers the spec's
// order-independent-resolution scenario: the referrer resolving BEFORE the file
// that defines the base still produces the correct definition id, because the
// target id is intrinsic (derived from the module path, not the base's parse
// state).
func TestParseFile_CrossFileInheritance_OrderIndependent(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "pkg/__init__.py", "")
	basePath := mustWrite(t, root, "pkg/base.py", "class BaseClient:\n    pass\n")
	clientPath := mustWrite(t, root, "pkg/client.py",
		"from pkg.base import BaseClient\n\nclass AsyncClient(BaseClient):\n    pass\n")

	p := NewParser("acme", "proj", root)

	// Parse the referrer FIRST (base not yet parsed).
	clientRes, err := p.ParseFile(context.Background(), clientPath)
	if err != nil {
		t.Fatalf("parse client: %v", err)
	}
	baseRes, err := p.ParseFile(context.Background(), basePath)
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}

	async := findClass(clientRes, "AsyncClient")
	if async == nil || len(async.Extends) != 1 {
		t.Fatalf("AsyncClient/Extends unexpected: %+v", async)
	}
	if async.Extends[0] != classID(t, baseRes, "BaseClient") {
		t.Errorf("referrer-first extends = %q, want the BaseClient id", async.Extends[0])
	}
}

// TestParseFile_UnresolvedReferencesStayInert confirms references that cannot be
// resolved in-tree are never mapped to a wrong entity.
func TestParseFile_UnresolvedReferencesStayInert(t *testing.T) {
	root := t.TempDir()
	// A base imported from a third-party package that does not exist in-tree, and a
	// star-import that must not resolve.
	clientPath := mustWrite(t, root, "app/client.py",
		"from requests import Session\nfrom mystuff import *\n\nclass MyClient(Session):\n    pass\n")

	p := NewParser("acme", "proj", root)
	res, err := p.ParseFile(context.Background(), clientPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	my := findClass(res, "MyClient")
	if my == nil || len(my.Extends) != 1 {
		t.Fatalf("MyClient/Extends unexpected: %+v", my)
	}
	// Session is third-party (not in-tree) → must NOT resolve to any acme.* entity.
	if got := my.Extends[0]; len(got) > 6 && got[:6] == "acme.s" {
		t.Errorf("third-party base wrongly resolved to in-tree entity: %q", got)
	}
}

func mustWrite(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return full
}

func findClass(res *ast.ParseResult, name string) *ast.CodeEntity {
	for _, e := range res.Entities {
		if e.Type == ast.TypeClass && e.Name == name {
			return e
		}
	}
	return nil
}

func classID(t *testing.T, res *ast.ParseResult, name string) string {
	t.Helper()
	e := findClass(res, name)
	if e == nil {
		t.Fatalf("class %q not found", name)
	}
	return e.ID
}
