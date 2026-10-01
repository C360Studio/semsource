package sourcelifecycle

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semstreams/types"
)

func TestRemovalExactSourceScope(t *testing.T) {
	a := entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}
	docs := types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"paths":["/wrong"],"project":"Manuals"}`)}
	scope, err := ScopeFor(a, "docs", docs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Selectors) != 1 || scope.Selectors[0].Prefix != "acme.test-a1b2c3.Manuals.web." {
		t.Fatalf("scope=%+v", scope)
	}
	ast := types.ComponentConfig{Name: "ast-source", Enabled: true, Config: json.RawMessage(`{"watch_paths":[{"project":"Manuals"}]}`)}
	if err := CheckExclusive(scope, map[string]types.ComponentConfig{"ast": ast}); err != nil {
		t.Fatalf("AST sibling collided with docs: %v", err)
	}
	if err := CheckExclusive(scope, map[string]types.ComponentConfig{"other-docs": docs}); err == nil {
		t.Fatal("indistinguishable docs source accepted")
	}
}
