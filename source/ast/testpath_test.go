package ast_test

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"

	"github.com/c360studio/semsource/source/ast"
)

func TestIsTestPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"entityid/entityid_test.go", true},
		{"entityid/entityid.go", false},
		{"ui/src/lib/components/GraphPanel.test.ts", true},
		{"ui/src/lib/components/GraphPanel.svelte", false},
		{"src/api.spec.ts", true},
		{"src/__tests__/api.ts", true},
		{"pkg/test_helpers.py", true},
		{"pkg/helpers_test.py", true},
		{"pkg/tests/fixtures.py", true},
		{"pkg/helpers.py", false},
		{"src/test/java/com/acme/FooTest.java", true},
		{"src/main/java/com/acme/Foo.java", false},
		{"app/FooTest.java", true},
		// Conservative: unknown shapes are NOT test code.
		{"testdata/sample.go", false},
		{"docs/testing.md", false},
	}
	for _, tt := range tests {
		if got := ast.IsTestPath(tt.path); got != tt.want {
			t.Errorf("IsTestPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// TestCodeEntity_TestMarkerEmission pins the demotion marker: present (and
// only present) on entities from test paths.
func TestCodeEntity_TestMarkerEmission(t *testing.T) {
	testEntity := ast.NewCodeEntity("acme", "golang", "repo", ast.TypeFunction,
		"TestBuild", "entityid/entityid_test.go")
	if !hasTriple(testEntity, ast.CodeTest, "true") {
		t.Error("test-file entity lacks the code.artifact.test marker")
	}
	prodEntity := ast.NewCodeEntity("acme", "golang", "repo", ast.TypeFunction,
		"Build", "entityid/entityid.go")
	if hasTriple(prodEntity, ast.CodeTest, "true") {
		t.Error("production entity carries the code.artifact.test marker")
	}
}

func hasTriple(e *ast.CodeEntity, predicate, object string) bool {
	for _, tr := range e.Triples() {
		if tr.Predicate == predicate && tr.Object == object {
			return true
		}
	}
	return false
}

// TestCodeTest_SalienceWeight pins the −2.0 registration (the demotion
// complement of the +2.0 exported boost).
func TestCodeTest_SalienceWeight(t *testing.T) {
	meta := vocabulary.GetPredicateMetadata(ast.CodeTest)
	if meta == nil {
		t.Fatal("code.artifact.test is not registered")
	}
	if meta.Weight != -2.0 {
		t.Errorf("code.artifact.test weight = %v, want -2.0", meta.Weight)
	}
}
