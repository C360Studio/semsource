package svelte

import (
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
)

func TestComponentReferenceMatchesDeclarationUnderEffectiveAuthority(t *testing.T) {
	authority := entityid.Authority{Org: "acme", Platform: "catalog-a1b2c3"}
	parser := NewParser(authority, "github.com/acme/components", t.TempDir())
	declaration := ast.NewCodeEntity(authority, "svelte", "github.com/acme/components", ast.TypeComponent, "Panel", "Panel.svelte")
	reference := parser.componentNameToEntityID("Panel", "Panel.svelte")
	if reference != declaration.ID {
		t.Fatalf("reference=%q declaration=%q", reference, declaration.ID)
	}
}
