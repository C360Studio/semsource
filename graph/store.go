package graph

import (
	"encoding/json"
	"sync"

	"github.com/c360studio/semstreams/message"
)

// Store is a goroutine-safe in-memory store for GraphEntity values.
// It uses content-based change detection to avoid spurious DELTA events.
type Store struct {
	mu       sync.RWMutex
	entities map[string]*GraphEntity
	// hashes stores a JSON fingerprint of each entity for change detection.
	hashes map[string]string
}

// NewStore creates and returns an empty Store.
func NewStore() *Store {
	return &Store{
		entities: make(map[string]*GraphEntity),
		hashes:   make(map[string]string),
	}
}

// Upsert inserts or updates an entity in the store.
// Returns true if the entity was new or its content changed; false if unchanged.
func (s *Store) Upsert(entity *GraphEntity) bool {
	if entity == nil {
		return false
	}

	// Compute fingerprint outside the lock to minimize lock hold time.
	hash := hashEntity(entity)

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.hashes[entity.ID]; ok && existing == hash {
		return false
	}

	// Deep copy to prevent external mutation of stored values.
	clone := cloneEntity(entity)
	s.entities[entity.ID] = clone
	s.hashes[entity.ID] = hash
	return true
}

// Remove deletes the entity with the given ID from the store.
// Returns true if the entity existed and was removed; false if not found.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entities[id]; !ok {
		return false
	}
	delete(s.entities, id)
	delete(s.hashes, id)
	return true
}

// Snapshot returns a copy of all entities currently in the store.
// The returned slice is independent of the store — mutations do not affect the store.
func (s *Store) Snapshot() []GraphEntity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]GraphEntity, 0, len(s.entities))
	for _, e := range s.entities {
		result = append(result, *cloneEntity(e))
	}
	return result
}

// Count returns the number of entities in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

// hashEntity returns a JSON fingerprint of the entity's semantic content.
// Excludes provenance timestamps and triple metadata (Timestamp, Confidence,
// Context, Datatype, ExpiresAt) to avoid false positives when the same entity
// is re-normalized with fresh timestamps.
func hashEntity(e *GraphEntity) string {
	// stableTriple captures only the semantic fields of a Triple.
	type stableTriple struct {
		Subject   string `json:"s"`
		Predicate string `json:"p"`
		Object    any    `json:"o"`
		Source    string `json:"src"`
	}

	type stable struct {
		ID      string         `json:"id"`
		Triples []stableTriple `json:"triples,omitempty"`
		Edges   []GraphEdge    `json:"edges,omitempty"`
	}

	st := make([]stableTriple, len(e.Triples))
	for i, t := range e.Triples {
		st[i] = stableTriple{
			Subject:   t.Subject,
			Predicate: t.Predicate,
			Object:    t.Object,
			Source:    t.Source,
		}
	}

	s := stable{ID: e.ID, Triples: st, Edges: e.Edges}
	data, err := json.Marshal(s)
	if err != nil {
		// Fall back to empty string — forces update on next upsert.
		return ""
	}
	return string(data)
}

// cloneEntity returns a deep copy of a GraphEntity.
func cloneEntity(e *GraphEntity) *GraphEntity {
	clone := &GraphEntity{
		ID:         e.ID,
		Provenance: e.Provenance,
	}

	if e.Triples != nil {
		clone.Triples = make([]message.Triple, len(e.Triples))
		copy(clone.Triples, e.Triples)
	}

	if e.Edges != nil {
		clone.Edges = make([]GraphEdge, len(e.Edges))
		copy(clone.Edges, e.Edges)
	}

	return clone
}
