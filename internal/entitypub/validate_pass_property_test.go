package entitypub

import (
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/ast"
)

// TestValidatePassImpliesPublishGatePass is the init-config-validation
// property: any namespace the config validator accepts produces entity IDs
// the publish gate accepts, across representative source shapes — so
// `semsource validate` is a real pre-flight and a green config can never be
// rejected later purely for ID shape.
func TestValidatePassImpliesPublishGatePass(t *testing.T) {
	namespaces := []string{"acme", "c360", "Acme-Corp", "org_1", "a", "quickstart"}
	shapes := []struct {
		language, name, path string
		entityType           ast.CodeEntityType
	}{
		{"golang", "Build", "entityid/entityid.go", ast.TypeFunction},
		{"svelte", "+page", "src/routes/+page.svelte", ast.TypeComponent},
		{"typescript", "clicks$", "src/app.ts", ast.TypeConst},
	}
	for _, ns := range namespaces {
		if err := config.ValidateNamespace(ns); err != nil {
			t.Fatalf("precondition: ValidateNamespace(%q) = %v", ns, err)
		}
		for _, s := range shapes {
			entity := ast.NewCodeEntity(ns, s.language, "myrepo", s.entityType, s.name, s.path)
			payload := &graph.EntityPayload{
				ID:                  entity.ID,
				UpdatedAt:           time.Now(),
				IndexingProfileHint: graph.IndexingProfileContent,
			}
			if err := ValidatePayload(payload); err != nil {
				t.Errorf("namespace %q, shape %s/%s: validate-passing config produced "+
					"publish-gate-failing ID %q: %v", ns, s.language, s.name, entity.ID, err)
			}
		}
	}
}
