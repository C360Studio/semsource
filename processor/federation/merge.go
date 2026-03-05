package federation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/c360studio/semsource/graph"
)

const (
	// publicNamespace is the reserved namespace for open-source / intrinsic entities.
	publicNamespace = "public"
)

// Merger applies federation merge policy to GraphEntities and GraphEvents.
// It is safe for concurrent use — all state is derived from the immutable Config.
type Merger struct {
	cfg Config
}

// NewMerger creates a Merger from the given Config.
// Callers should validate the Config before calling NewMerger.
func NewMerger(cfg Config) *Merger {
	return &Merger{cfg: cfg}
}

// MergeEntity applies federation merge policy to a single incoming entity.
//
// Rules (spec S7.4):
//   - public.* entities are accepted and merged unconditionally
//   - {org}.* entities are accepted only when org == cfg.LocalNamespace
//   - All other entities are rejected (cross-org overwrite prevented)
//
// existing may be nil for a brand-new entity.
// On success returns the merged entity and nil error.
// On rejection returns nil and a descriptive error.
func (m *Merger) MergeEntity(incoming graph.GraphEntity, existing *graph.GraphEntity) (*graph.GraphEntity, error) {
	org := entityOrg(incoming.ID)
	if org == "" {
		return nil, fmt.Errorf("federation: entity ID %q has no org segment", incoming.ID)
	}

	if org != publicNamespace && org != m.cfg.LocalNamespace {
		return nil, fmt.Errorf("federation: cross-org overwrite rejected: entity %q belongs to org %q, local namespace is %q",
			incoming.ID, org, m.cfg.LocalNamespace)
	}

	// Start with a copy of incoming.
	merged := incoming

	if existing == nil {
		return &merged, nil
	}

	// Edge union: combine existing and incoming edges, deduplicating by key.
	merged.Edges = unionEdges(existing.Edges, incoming.Edges)

	// Provenance append: move existing primary provenance into AdditionalProvenance,
	// then prepend existing.AdditionalProvenance chain. Incoming provenance is primary.
	additionalFromExisting := make([]graph.SourceProvenance, 0,
		1+len(existing.AdditionalProvenance))
	// Append the existing chain first (oldest to newest order).
	additionalFromExisting = append(additionalFromExisting, existing.AdditionalProvenance...)
	// Append the existing primary provenance (most recent of the old set).
	additionalFromExisting = append(additionalFromExisting, existing.Provenance)

	// Merge with any AdditionalProvenance the incoming entity already carries.
	merged.AdditionalProvenance = append(additionalFromExisting, incoming.AdditionalProvenance...)

	return &merged, nil
}

// ApplyEvent applies federation merge policy to an incoming GraphEvent.
//
// For SEED and DELTA events, each entity is run through MergeEntity:
//   - Accepted entities are included in the output event.
//   - Rejected entities (cross-org) are silently filtered.
//
// For RETRACT events, each entity ID is checked:
//   - IDs belonging to the local namespace or public namespace pass through.
//   - Cross-org retraction IDs are filtered (cannot retract another org's entities).
//
// For HEARTBEAT events, the event passes through unchanged.
//
// existing is the caller's current entity store (may be nil). When non-nil,
// it is used to look up the existing entity for edge-union and provenance-append.
// The map is read-only — ApplyEvent never modifies it.
func (m *Merger) ApplyEvent(event *graph.GraphEvent, existing map[string]*graph.GraphEntity) (*graph.GraphEvent, error) {
	if event == nil {
		return nil, errors.New("federation: nil event")
	}

	// Copy the event to avoid mutating the caller's value.
	out := *event

	switch event.Type {
	case graph.EventTypeHEARTBEAT:
		// Pass through unchanged.
		return &out, nil

	case graph.EventTypeSEED, graph.EventTypeDELTA:
		out.Entities = m.mergeEntities(event.Entities, existing)

	case graph.EventTypeRETRACT:
		out.Retractions = m.filterRetractions(event.Retractions)

	default:
		return nil, fmt.Errorf("federation: unknown event type %q", event.Type)
	}

	return &out, nil
}

// mergeEntities filters and merges a slice of entities, returning only those
// that pass the merge policy. Cross-org entities are silently dropped.
func (m *Merger) mergeEntities(entities []graph.GraphEntity, store map[string]*graph.GraphEntity) []graph.GraphEntity {
	if len(entities) == 0 {
		return nil
	}

	result := make([]graph.GraphEntity, 0, len(entities))
	for _, e := range entities {
		var existingPtr *graph.GraphEntity
		if store != nil {
			if ex, ok := store[e.ID]; ok {
				existingPtr = ex
			}
		}
		merged, err := m.MergeEntity(e, existingPtr)
		if err != nil {
			// Cross-org or malformed — skip silently (log would go here in production).
			continue
		}
		result = append(result, *merged)
	}
	return result
}

// filterRetractions returns only entity IDs that the local namespace is
// allowed to retract: its own org entities and public.* entities.
func (m *Merger) filterRetractions(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	result := make([]string, 0, len(ids))
	for _, id := range ids {
		org := entityOrg(id)
		if org == publicNamespace || org == m.cfg.LocalNamespace {
			result = append(result, id)
		}
		// Cross-org retraction — silently drop.
	}
	return result
}

// unionEdges combines two edge slices, deduplicating by (FromID, ToID, EdgeType).
// The result preserves all edges from both slices; the first occurrence wins for
// duplicate keys (existing takes precedence on Weight/Properties).
func unionEdges(existing, incoming []graph.GraphEdge) []graph.GraphEdge {
	type edgeKey struct{ from, to, edgeType string }

	seen := make(map[edgeKey]bool, len(existing)+len(incoming))
	result := make([]graph.GraphEdge, 0, len(existing)+len(incoming))

	for _, e := range existing {
		k := edgeKey{e.FromID, e.ToID, e.EdgeType}
		if !seen[k] {
			seen[k] = true
			result = append(result, e)
		}
	}
	for _, e := range incoming {
		k := edgeKey{e.FromID, e.ToID, e.EdgeType}
		if !seen[k] {
			seen[k] = true
			result = append(result, e)
		}
	}
	return result
}

// entityOrg extracts the org segment (first dot-separated part) of a 6-part
// entity ID. Returns empty string if the ID has no dot separator.
func entityOrg(id string) string {
	if i := strings.Index(id, "."); i >= 0 {
		return id[:i]
	}
	return ""
}
