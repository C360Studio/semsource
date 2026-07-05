package java

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// parseAll parses the given {relPath: source} files under a shared temp root and
// returns every entity keyed by name (last one wins — tests use distinct names).
func parseAll(t *testing.T, files map[string]string) (map[string]*ast.CodeEntity, string) {
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
	return byName, root
}

// TestSameFileExtendsResolvesToDefinition is the empirically-confirmed dangle:
// the extends target must byte-match the base class definition ID, not
// `.type.<referrer>-extends Base`.
func TestSameFileExtendsResolvesToDefinition(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/Zoo.java": "package a;\nclass Animal {}\nclass Dog extends Animal {}\n",
	})
	animal, dog := ents["Animal"], ents["Dog"]
	if animal == nil || dog == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(dog.Extends) != 1 {
		t.Fatalf("Dog.Extends = %v, want 1", dog.Extends)
	}
	if dog.Extends[0] != animal.ID {
		t.Errorf("extends target %q != Animal definition %q", dog.Extends[0], animal.ID)
	}
}

func TestSameFileImplementsResolvesToDefinition(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/M.java": "package a;\ninterface Pet {}\ninterface Runnable {}\nclass Dog implements Pet, Runnable {}\n",
	})
	dog, pet, runnable := ents["Dog"], ents["Pet"], ents["Runnable"]
	if dog == nil || pet == nil || runnable == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(dog.Implements) != 2 {
		t.Fatalf("Dog.Implements = %v, want 2 (Pet, Runnable)", dog.Implements)
	}
	got := map[string]bool{dog.Implements[0]: true, dog.Implements[1]: true}
	if !got[pet.ID] {
		t.Errorf("implements missing Pet definition %q; got %v", pet.ID, dog.Implements)
	}
	if !got[runnable.ID] {
		t.Errorf("implements missing Runnable definition %q; got %v", runnable.ID, dog.Implements)
	}
}

// TestCrossFileExtendsViaImport — the base class is in another package, imported.
func TestCrossFileExtendsViaImport(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/b/Animal.java": "package a.b;\npublic class Animal {}\n",
		"x/Dog.java":      "package x;\nimport a.b.Animal;\npublic class Dog extends Animal {}\n",
	})
	animal, dog := ents["Animal"], ents["Dog"]
	if animal == nil || dog == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(dog.Extends) != 1 || dog.Extends[0] != animal.ID {
		t.Errorf("cross-file extends = %v, want [%s]", dog.Extends, animal.ID)
	}
}

// TestSamePackageExtendsWithoutImport — base class in a sibling file, no import.
func TestSamePackageExtendsWithoutImport(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/b/Animal.java": "package a.b;\npublic class Animal {}\n",
		"a/b/Dog.java":    "package a.b;\npublic class Dog extends Animal {}\n",
	})
	animal, dog := ents["Animal"], ents["Dog"]
	if animal == nil || dog == nil {
		t.Fatalf("missing entities: %v", ents)
	}
	if len(dog.Extends) != 1 || dog.Extends[0] != animal.ID {
		t.Errorf("same-package extends = %v, want [%s]", dog.Extends, animal.ID)
	}
}

// TestUnresolvedFQNStaysExternal — a fully-qualified stdlib/third-party reference
// is not mapped to a wrong in-tree entity.
func TestUnresolvedFQNStaysExternal(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/Svc.java": "package a;\nimport java.util.List;\npublic class Svc extends java.lang.Object {}\n",
	})
	svc := ents["Svc"]
	if svc == nil || len(svc.Extends) != 1 {
		t.Fatalf("Svc.Extends = %v", svc)
	}
	if svc.Extends[0] != "external:java.lang.Object" {
		t.Errorf("extends target %q, want external:java.lang.Object", svc.Extends[0])
	}
}

// TestImportedThirdPartyImplementsStaysExternal — `implements` a type imported
// from an out-of-tree package resolves to external:fqn, not a dangling in-tree ID.
func TestImportedThirdPartyImplementsStaysExternal(t *testing.T) {
	ents, _ := parseAll(t, map[string]string{
		"a/Svc.java": "package a;\nimport java.io.Serializable;\npublic class Svc implements Serializable {}\n",
	})
	svc := ents["Svc"]
	if svc == nil || len(svc.Implements) != 1 {
		t.Fatalf("Svc.Implements = %v", svc)
	}
	if svc.Implements[0] != "external:java.io.Serializable" {
		t.Errorf("implements target %q, want external:java.io.Serializable", svc.Implements[0])
	}
}
