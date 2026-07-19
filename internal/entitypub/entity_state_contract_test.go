package entitypub

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
)

// TestValidatePayload_EnforcesGraphIngestSegmentContract pins the audit
// finding: IDs that graph-ingest rejects must be rejected at the producer,
// not published and silently Termed downstream.
func TestValidatePayload_EnforcesGraphIngestSegmentContract(t *testing.T) {
	bad := []string{
		"acme.semsource.svelte.myapp.component.src-routes-+page-svelte-+page",
		"acme.semsource.typescript.myapp.function.src-routes-[slug]-+page-ts-load",
		"acme.semsource.typescript.myapp.const.src-app-ts-clicks$",
		"acme.semsource.golang.myrepo.function._examples-demo-go-Demo",
	}
	for _, id := range bad {
		payload := &graph.EntityPayload{
			ID:                  id,
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileContent,
		}
		err := ValidatePayload(payload)
		if err == nil {
			t.Errorf("ValidatePayload accepted %q, which graph-ingest would reject", id)
			continue
		}
		if !strings.Contains(err.Error(), "graph-ingest segment contract") {
			t.Errorf("ValidatePayload(%q) error lacks contract attribution: %v", id, err)
		}
	}
}

// TestValidatePayload_AcceptsCleanIDs is the control: sanitized/legacy-valid
// IDs pass.
func TestValidatePayload_AcceptsCleanIDs(t *testing.T) {
	good := []string{
		"acme.semsource.golang.myrepo.function.entityid-entityid-go-Build",
		"c360.semsource.svelte.workspace.component.src-routes-page-svelte-page-a1b2c3d4",
	}
	for _, id := range good {
		payload := &graph.EntityPayload{
			ID:                  id,
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileContent,
		}
		if err := ValidatePayload(payload); err != nil {
			t.Errorf("ValidatePayload rejected clean ID %q: %v", id, err)
		}
	}
}
