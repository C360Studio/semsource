package ast_test

import (
	"testing"

	semtypes "github.com/c360studio/semstreams/pkg/types"

	"github.com/c360studio/semsource/source/ast"
)

// TestBuildScopedInstanceID_LegacyShapesUnchanged pins byte-stability for the
// shapes every existing graph depends on: clean Go paths and identifiers must
// produce exactly the historical instance strings.
func TestBuildScopedInstanceID_LegacyShapesUnchanged(t *testing.T) {
	cases := []struct {
		path, name string
		entityType ast.CodeEntityType
		want       string
	}{
		{"entityid/entityid.go", "Build", ast.TypeFunction, "entityid-entityid-go-Build"},
		{"entityid/entityid.go", "SanitizeInstance", ast.TypeFunction, "entityid-entityid-go-SanitizeInstance"},
		{"processor/ast-source/component.go", "publishEntity", ast.TypeMethod, "processor-ast-source-component-go-publishEntity"},
		{"entityid/entityid.go", "", ast.TypeFile, "entityid-entityid-go"},
	}
	for _, c := range cases {
		if got := ast.BuildInstanceID(c.path, c.name, c.entityType); got != c.want {
			t.Errorf("BuildInstanceID(%q, %q) = %q, want %q (existing IDs must not change)",
				c.path, c.name, got, c.want)
		}
	}
}

// TestBuildScopedInstanceID_AuditShapesAreValid covers the audit's
// silently-dropped fixtures: SvelteKit route trees, $-identifiers, and
// underscore-prefixed directories must now yield contract-valid instances.
func TestBuildScopedInstanceID_AuditShapesAreValid(t *testing.T) {
	cases := []struct {
		path, name string
		entityType ast.CodeEntityType
	}{
		{"src/routes/+page.svelte", "+page", ast.TypeComponent},
		{"src/routes/+layout.svelte", "+layout", ast.TypeComponent},
		{"src/routes/+page.svelte", "", ast.TypeFile},
		{"src/routes/[slug]/+page.ts", "load", ast.TypeFunction},
		{"src/routes/(group)/+page.ts", "load", ast.TypeFunction},
		{"src/routes/@modal/+page.ts", "load", ast.TypeFunction},
		{"src/app.ts", "clicks$", ast.TypeConst},
		{"_examples/demo/demo.go", "Demo", ast.TypeFunction},
	}
	for _, c := range cases {
		instance := ast.BuildInstanceID(c.path, c.name, c.entityType)
		probe := "org.semsource.svelte.myapp." + string(c.entityType) + "." + instance
		if err := semtypes.ValidateEntityID(probe); err != nil {
			t.Errorf("BuildInstanceID(%q, %q) = %q fails graph-ingest contract: %v",
				c.path, c.name, instance, err)
		}
	}
}

// TestNewScopedCodeEntity_AuditShapesPassSubstrateValidation drives the full
// entity constructor for the audit fixtures, mirroring the production path the
// parsers use.
func TestNewScopedCodeEntity_AuditShapesPassSubstrateValidation(t *testing.T) {
	cases := []struct {
		language, name, path string
		entityType           ast.CodeEntityType
	}{
		{"svelte", "+page", "src/routes/+page.svelte", ast.TypeComponent},
		{"typescript", "load", "src/routes/[slug]/+page.ts", ast.TypeFunction},
		{"typescript", "clicks$", "src/app.ts", ast.TypeConst},
		{"golang", "Demo", "_examples/demo/demo.go", ast.TypeFunction},
	}
	for _, c := range cases {
		e := ast.NewCodeEntity("acme", c.language, "myapp", c.entityType, c.name, c.path)
		if err := semtypes.ValidateEntityID(e.ID); err != nil {
			t.Errorf("NewCodeEntity(%s %q in %q) ID %q fails graph-ingest contract: %v",
				c.language, c.name, c.path, e.ID, err)
		}
	}
}

// TestBuildScopedInstanceID_Deterministic pins repeated-indexing stability for
// sanitized shapes.
func TestBuildScopedInstanceID_Deterministic(t *testing.T) {
	for range 3 {
		a := ast.BuildInstanceID("src/routes/+page.svelte", "+page", ast.TypeComponent)
		b := ast.BuildInstanceID("src/routes/+page.svelte", "+page", ast.TypeComponent)
		if a != b {
			t.Fatalf("nondeterministic instance: %q vs %q", a, b)
		}
	}
}

// TestEdgeEndpointParity pins that a reference to a sanitized symbol built via
// the same helpers byte-matches the symbol's own entity ID — the property that
// keeps Contains/Calls edges attached after sanitization.
func TestEdgeEndpointParity(t *testing.T) {
	node := ast.NewCodeEntity("acme", "svelte", "myapp", ast.TypeComponent, "+page", "src/routes/+page.svelte")
	// Edge builders construct endpoint IDs through BuildInstanceID with the
	// same inputs (see svelte componentNameToEntityID).
	edgeInstance := ast.BuildInstanceID("src/routes/+page.svelte", "+page", ast.TypeComponent)
	if got := node.ID; got[len(got)-len(edgeInstance):] != edgeInstance {
		t.Errorf("edge endpoint instance %q does not match node ID %q", edgeInstance, node.ID)
	}
}

// TestNewCodeEntity_OverlongSymbolsStillLand is the regression for the field
// failure this test file's sanitizers could not catch: a JS destructuring
// const under a deep path produced a 265-byte ID that graph-ingest rejected
// outright ("entity will never land"). Per-segment sanitization cannot see the
// total, so the AST path could emit individually-legal fragments that together
// blew the byte budget. Assert against the real graph validator, not a
// mirrored regex — the byte cap is the half a regex does not express.
func TestNewCodeEntity_OverlongSymbolsStillLand(t *testing.T) {
	// The shape observed in the field: the whole destructuring pattern becomes
	// one symbol name, under a nested dependency path.
	pattern := "cliConfigArray, configArrayFactory, finalizeCache, " +
		"loadConfigFile, normalizeOptions, resolveExtends, translateESLintRC"
	deepPath := "ui/node_modules/@eslint/eslintrc/lib/config-array/" +
		"config-array-factory/normalize/extends/resolver/index.js"

	entity := ast.NewScopedCodeEntity("c360", "javascript", "workspace", ast.TypeConst,
		[]string{"ConfigArrayFactory", "normalizeExtends", "resolveModule"}, pattern, deepPath)

	if err := semtypes.ValidateEntityID(entity.ID); err != nil {
		t.Errorf("AST entity ID would never land (%d bytes): %v\nID: %q",
			len(entity.ID), err, entity.ID)
	}
}

// TestNewCodeEntity_OverlongSiblingsStayDistinct guards the failure mode the
// length bound could introduce: two symbols sharing a long prefix must not
// truncate onto a single ID, which would silently merge two entities.
func TestNewCodeEntity_OverlongSiblingsStayDistinct(t *testing.T) {
	deepPath := "ui/node_modules/@eslint/eslintrc/lib/config-array/" +
		"config-array-factory/normalize/extends/resolver/index.js"
	scope := []string{"ConfigArrayFactory", "normalizeExtends", "resolveModule"}
	long := "cliConfigArray, configArrayFactory, finalizeCache, loadConfigFile, normalize"

	a := ast.NewScopedCodeEntity("c360", "javascript", "workspace", ast.TypeConst,
		scope, long+"Alpha", deepPath)
	b := ast.NewScopedCodeEntity("c360", "javascript", "workspace", ast.TypeConst,
		scope, long+"Beta", deepPath)

	if a.ID == b.ID {
		t.Errorf("distinct overlong symbols merged onto one ID: %q", a.ID)
	}
}

// TestSanitizePathSegment_DistinctRoutesStayDistinct pins the anti-collision
// property at the path level: "+page.svelte" and "page.svelte" in one
// directory must not merge.
func TestSanitizePathSegment_DistinctRoutesStayDistinct(t *testing.T) {
	a := ast.SanitizePathSegment("src/routes/+page.svelte")
	b := ast.SanitizePathSegment("src/routes/page.svelte")
	if a == b {
		t.Errorf("path collision: %q", a)
	}
}
