package svelte

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/c360studio/semsource/source/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParser_RealPlanCard tests parsing the actual PlanCard.svelte from the UI
func TestParser_RealPlanCard(t *testing.T) {
	// Get path to the real PlanCard.svelte
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot get current file path")
	}

	// Navigate from processor/ast/svelte to ui/src/lib/components/board/PlanCard.svelte
	projectRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "..")
	planCardPath := filepath.Join(projectRoot, "ui", "src", "lib", "components", "board", "PlanCard.svelte")

	parser := NewParser("testorg", "ui", filepath.Join(projectRoot, "ui"))
	result, err := parser.ParseFile(context.Background(), planCardPath)
	if err != nil {
		t.Skipf("PlanCard.svelte not found or cannot be parsed: %v", err)
	}
	require.NotNil(t, result)

	// Find the component entity
	var componentEntity *ast.CodeEntity
	for _, entity := range result.Entities {
		if entity.Type == ast.TypeComponent && entity.Name == "PlanCard" {
			componentEntity = entity
			break
		}
	}
	require.NotNil(t, componentEntity, "PlanCard component should be found")

	// Verify DocComment contains rune information
	assert.Contains(t, componentEntity.DocComment, "Props:", "Should have props")
	assert.Contains(t, componentEntity.DocComment, "Derived:", "Should have derived values")

	// Verify imports
	expectedImports := []string{
		"$lib/components/shared/Icon.svelte",
		"./PipelineIndicator.svelte",
		"./ModeIndicator.svelte",
		"./AgentBadge.svelte",
	}
	for _, imp := range expectedImports {
		assert.Contains(t, result.Imports, imp, "Should import %s", imp)
	}
}
