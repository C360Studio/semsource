package golang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parseAllGo(t *testing.T, project string, files map[string]string) map[string]*ast.CodeEntity {
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
	p := NewParser("acme", project, root)
	for rel := range files {
		if !strings.HasSuffix(rel, ".go") {
			continue // support files (go.mod) shape resolution but are not parsed
		}
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

// TestGoSameFileStructEmbed — an embedded local struct must resolve to the
// struct's definition ID (kind `struct`), not the hard-coded `.type.` segment.
func TestGoSameFileStructEmbed(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"a.go": "package p\ntype Base struct{}\ntype Derived struct{ Base }\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Embeds) != 1 || derived.Embeds[0] != base.ID {
		t.Errorf("embeds = %v, want [%s]", derived.Embeds, base.ID)
	}
}

// TestGoSameFileInterfaceEmbed — an embedded local interface resolves with kind
// `interface`.
func TestGoSameFileInterfaceEmbed(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"a.go": "package p\ntype Reader interface{ Read() }\ntype ReadWriter interface{ Reader }\n",
	})
	reader, rw := ents["Reader"], ents["ReadWriter"]
	if reader == nil || rw == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(rw.Embeds) != 1 || rw.Embeds[0] != reader.ID {
		t.Errorf("interface embeds = %v, want [%s]", rw.Embeds, reader.ID)
	}
}

// TestGoCrossFileEmbed — the embedded struct is defined in a sibling file of the
// same package; the edge must target that sibling's definition ID.
func TestGoCrossFileEmbed(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"base.go":    "package p\ntype Base struct{}\n",
		"derived.go": "package p\ntype Derived struct{ Base }\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Embeds) != 1 || derived.Embeds[0] != base.ID {
		t.Errorf("cross-file embeds = %v, want [%s]", derived.Embeds, base.ID)
	}
}

// TestGoPackageQualifiedStaysExternal — `pkg.Type` remains an external reference.
func TestGoPackageQualifiedStaysExternal(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		"a.go": "package p\nimport \"context\"\ntype Server struct{ ctx context.Context }\n",
	})
	server := ents["Server"]
	if server == nil {
		t.Fatalf("missing Server: %v", ents)
	}
	found := false
	for _, ref := range server.References {
		if ref == "external:context.Context" {
			found = true
		}
	}
	if !found {
		t.Errorf("References = %v, want to contain external:context.Context", server.References)
	}
}

// TestGoEmbedPrefersProductionOverTestType — a production embed must resolve to
// the production sibling's definition, never a same-named type in a _test.go file
// (which production code cannot legally reference). Guards the packageTypes
// _test.go exclusion.
func TestGoEmbedPrefersProductionOverTestType(t *testing.T) {
	ents := parseAllGo(t, "proj", map[string]string{
		// _test.go sorts before server.go in ReadDir order — without the exclusion
		// its Server would shadow the production one.
		"aaa_test.go": "package p\ntype Server struct{}\n",
		"server.go":   "package p\ntype Server struct{}\n",
		"app.go":      "package p\ntype App struct{ Server }\n",
	})
	app := ents["App"]
	if app == nil || len(app.Embeds) != 1 {
		t.Fatalf("App.Embeds = %v", app)
	}
	if !strings.Contains(app.Embeds[0], "server-go-Server") {
		t.Errorf("embed target %q, want it built against production server.go, not a _test.go file", app.Embeds[0])
	}
	if strings.Contains(app.Embeds[0], "test") {
		t.Errorf("embed target %q resolved to a _test.go type", app.Embeds[0])
	}
}

// TestGoRawProjectSlugResolves — the raw-project bug: a reference against a project
// name needing a slug (a module-cache path with '@') must still match the
// definition, which runs project through SystemSlug.
func TestGoRawProjectSlugResolves(t *testing.T) {
	ents := parseAllGo(t, "semstreams@v1.9.0", map[string]string{
		"a.go": "package p\ntype Base struct{}\ntype Derived struct{ Base }\n",
	})
	base, derived := ents["Base"], ents["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(derived.Embeds) != 1 || derived.Embeds[0] != base.ID {
		t.Errorf("embeds = %v, want [%s] (SystemSlug parity)", derived.Embeds, base.ID)
	}
}
