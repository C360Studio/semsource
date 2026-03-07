// Package graph provides core graph types and in-memory storage for semsource.
package graph

import (
	"time"

	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

// GraphEntity is a normalized knowledge graph entity used internally by semsource.
// It mirrors federation.Entity but is sovereign to this service; handlers produce
// GraphEntity values that the normalizer converts to federation.Entity for emission.
type GraphEntity struct {
	// ID is the deterministic 6-part entity identifier:
	// org.platform.domain.system.type.instance
	ID string `json:"id"`

	// Triples are the single source of truth for all semantic properties.
	Triples []message.Triple `json:"triples"`

	// Edges represent explicit directed relationships to other entities.
	Edges []federation.Edge `json:"edges,omitempty"`

	// Provenance records the origin of this entity.
	Provenance SourceProvenance `json:"provenance"`
}

// GraphEdge represents a directed relationship between two graph entities.
// Aligns with federation.Edge for straightforward conversion.
type GraphEdge struct {
	// FromID is the source entity's 6-part ID.
	FromID string `json:"from_id"`

	// ToID is the target entity's 6-part ID.
	ToID string `json:"to_id"`

	// EdgeType describes the semantic relationship (e.g., "authored_by", "imports").
	EdgeType string `json:"edge_type"`

	// Weight is an optional edge weight (0.0 = unweighted, positive = weighted).
	Weight float64 `json:"weight,omitempty"`

	// Properties holds any additional edge metadata.
	Properties map[string]any `json:"properties,omitempty"`
}

// SourceProvenance records the origin of a graph entity or event.
type SourceProvenance struct {
	// SourceType identifies the class of source (e.g., "git", "ast", "url").
	SourceType string `json:"source_type"`

	// SourceID is the unique identifier for the specific source instance.
	SourceID string `json:"source_id"`

	// Timestamp records when this provenance record was created.
	Timestamp time.Time `json:"timestamp"`

	// Handler is the name of the handler that produced this entity.
	Handler string `json:"handler"`
}

// ToFederationProvenance converts a SourceProvenance to a federation.Provenance
// for emission via the semstreams transport.
func (sp SourceProvenance) ToFederationProvenance() federation.Provenance {
	return federation.Provenance{
		SourceType: sp.SourceType,
		SourceID:   sp.SourceID,
		Timestamp:  sp.Timestamp,
		Handler:    sp.Handler,
	}
}

// ToFederationEntity converts a GraphEntity to a federation.Entity for emission.
func (ge GraphEntity) ToFederationEntity() federation.Entity {
	return federation.Entity{
		ID:         ge.ID,
		Triples:    ge.Triples,
		Edges:      ge.Edges,
		Provenance: ge.Provenance.ToFederationProvenance(),
	}
}
