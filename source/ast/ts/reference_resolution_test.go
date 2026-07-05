package ts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parseAll(t *testing.T, files map[string]string) map[string]*ast.CodeEntity {
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
	p := NewParser("acme", "test", root)
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

func TestTSSameFileClassExtends(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"m.ts": "export class Base {}\nexport class Derived extends Base {}\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Extends) != 1 || derived.Extends[0] != base.ID {
		t.Errorf("extends = %v, want [%s]", derived.Extends, base.ID)
	}
}

func TestTSSameFileClassImplements(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"m.ts": "export interface Serializable {}\nexport class Svc implements Serializable {}\n",
	})
	iface, svc := ents["Serializable"], ents["Svc"]
	if iface == nil || svc == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(svc.Implements) != 1 || svc.Implements[0] != iface.ID {
		t.Errorf("implements = %v, want [%s]", svc.Implements, iface.ID)
	}
}

func TestTSInterfaceExtendsInterface(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"m.ts": "export interface Base {}\nexport interface Extended extends Base {}\n",
	})
	base, ext := ents["Base"], ents["Extended"]
	if base == nil || ext == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(ext.Extends) != 1 || ext.Extends[0] != base.ID {
		t.Errorf("interface extends = %v, want [%s]", ext.Extends, base.ID)
	}
}

func TestTSCrossFileExtendsViaRelativeImport(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"base.ts":   "export class Base {}\n",
		"client.ts": "import { Base } from './base';\nexport class Derived extends Base {}\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Extends) != 1 || derived.Extends[0] != base.ID {
		t.Errorf("cross-file extends = %v, want [%s]", derived.Extends, base.ID)
	}
}

// TestTSAliasedImportResolvesToOrigin — `import { Base as B }` then `extends B`
// must target Base's definition (built with the origin name), not `B`.
func TestTSAliasedImportResolvesToOrigin(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"base.ts":   "export class Base {}\n",
		"client.ts": "import { Base as B } from './base';\nexport class Derived extends B {}\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Extends) != 1 || derived.Extends[0] != base.ID {
		t.Errorf("aliased extends = %v, want [%s]", derived.Extends, base.ID)
	}
}

// TestTSCrossFileSubdirImport — resolves a relative specifier into a subdirectory.
func TestTSCrossFileSubdirImport(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"lib/base.ts": "export class Base {}\n",
		"client.ts":   "import { Base } from './lib/base';\nexport class Derived extends Base {}\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Extends) != 1 || derived.Extends[0] != base.ID {
		t.Errorf("subdir extends = %v, want [%s]", derived.Extends, base.ID)
	}
}

// TestTSBareSpecifierStaysExternal — `import { Component } from 'react'` then
// `extends Component` must not map to a wrong in-tree entity.
func TestTSBareSpecifierStaysExternal(t *testing.T) {
	ents := parseAll(t, map[string]string{
		"c.ts": "import { Component } from 'react';\nexport class Widget extends Component {}\n",
	})
	widget := ents["Widget"]
	if widget == nil || len(widget.Extends) != 1 {
		t.Fatalf("Widget.Extends = %v", widget)
	}
	if widget.Extends[0] != "external:Component" {
		t.Errorf("extends target %q, want external:Component", widget.Extends[0])
	}
}
