// Package graph provides core graph entity types for the SemSource knowledge graph.
package graph

import (
	"time"

	"github.com/c360studio/semstreams/message"
)

// GraphEntity represents a normalized knowledge graph entity produced by SemSource.
// It aligns with semstreams/graph.EntityState but carries SemSource-specific provenance.
//
// The ID field is the 6-part entity identifier: org.platform.domain.system.type.instance
// which must be a valid NATS KV key.
type GraphEntity struct {
	// ID is the deterministic 6-part entity identifier.
	ID string `json:"id"`

	// Triples are the single source of truth for all semantic properties.
	Triples []message.Triple `json:"triples"`

	// Edges represent explicit relationships to other entities.
	Edges []GraphEdge `json:"edges,omitempty"`

	// Provenance records the primary (most recent) origin of this entity.
	Provenance SourceProvenance `json:"provenance"`

	// AdditionalProvenance accumulates provenance records from prior merges.
	// The FederationProcessor appends previous Provenance here on each merge.
	// This field is always appended, never replaced.
	AdditionalProvenance []SourceProvenance `json:"additional_provenance,omitempty"`
}

// GraphEdge represents a directed relationship between two graph entities.
type GraphEdge struct {
	// FromID is the source entity's 6-part ID.
	FromID string `json:"from_id"`

	// ToID is the target entity's 6-part ID.
	ToID string `json:"to_id"`

	// EdgeType describes the semantic relationship (e.g., "authored_by", "imports", "calls").
	EdgeType string `json:"edge_type"`

	// Weight is an optional edge weight (0.0 = unweighted, positive = weighted).
	Weight float64 `json:"weight,omitempty"`

	// Properties holds any additional edge metadata.
	Properties map[string]any `json:"properties,omitempty"`
}

// SourceProvenance records the origin of an entity or event.
type SourceProvenance struct {
	// SourceType identifies the class of source (e.g., "git", "ast", "url", "doc", "config").
	SourceType string `json:"source_type"`

	// SourceID is the unique identifier for the specific source instance.
	SourceID string `json:"source_id"`

	// Timestamp records when this provenance record was created.
	Timestamp time.Time `json:"timestamp"`

	// Handler is the name of the handler that produced this entity.
	Handler string `json:"handler"`
}
