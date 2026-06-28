package entitypub

import (
	"fmt"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/source/ontology"
)

// PayloadFromState converts a handler-produced entity state into the Graphable
// payload used by graph-ingest. It stamps the entity's BFO/CCO ontology class
// (ADR-0005) so results are rankable by ontology and exportable as RDF later.
func PayloadFromState(state *handler.EntityState) (*graph.EntityPayload, error) {
	if state == nil {
		return nil, fmt.Errorf("entity state is nil")
	}

	payload := &graph.EntityPayload{
		ID:                  state.ID,
		TripleData:          ontology.StampClass(state.ID, state.Triples, state.UpdatedAt),
		UpdatedAt:           state.UpdatedAt,
		Storage:             state.StorageRef,
		IndexingProfileHint: state.IndexingProfile,
	}
	if err := ValidatePayload(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// ValidatePayload applies SemSource's publish-boundary graph contract checks.
func ValidatePayload(payload *graph.EntityPayload) error {
	if payload == nil {
		return fmt.Errorf("entity payload is nil")
	}
	if err := entityid.ValidateNATSKVKey(payload.ID); err != nil {
		return fmt.Errorf("validate entity payload ID: %w", err)
	}
	parts := strings.Split(payload.ID, ".")
	if len(parts) != 6 {
		return fmt.Errorf("entity payload ID %q has %d parts, want 6", payload.ID, len(parts))
	}
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("entity payload ID %q has empty part %d", payload.ID, i)
		}
	}
	for i, triple := range payload.TripleData {
		if triple.Subject == "" {
			return fmt.Errorf("triple %d has empty subject for entity %q", i, payload.ID)
		}
		if triple.Subject != payload.ID {
			return fmt.Errorf("triple %d subject %q does not match entity %q", i, triple.Subject, payload.ID)
		}
	}
	if err := payload.Validate(); err != nil {
		return fmt.Errorf("validate entity payload: %w", err)
	}
	return nil
}
