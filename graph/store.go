package graph

import (
	"encoding/json"
	"sync"
)

// Store is a goroutine-safe in-memory store for GraphEntity values.
// It is the authoritative source of entity state within a single semsource instance.
// SEED events are produced from Snapshot(); DELTA events are driven by Upsert return values.
type Store struct {
	mu       sync.RWMutex
	entities map[string]*GraphEntity
}

// NewStore returns an initialised, empty Store.
func NewStore() *Store {
	return &Store{
		entities: make(map[string]*GraphEntity),
	}
}

// Upsert stores the entity, replacing any existing entry with the same ID.
// Returns true if the entity was added or its content changed, false if it was
// identical to the existing entry (no-op write). Callers use the return value
// to decide whether to emit a DELTA event.
func (s *Store) Upsert(e GraphEntity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.entities[e.ID]
	if ok && equal(existing, &e) {
		return false
	}

	clone := e
	s.entities[e.ID] = &clone
	return true
}

// Remove deletes the entity with the given ID.
// Returns true if the entity existed, false if it was not found.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entities[id]; !ok {
		return false
	}
	delete(s.entities, id)
	return true
}

// Snapshot returns a shallow copy of all entities as a slice.
// The returned slice and each GraphEntity value are independent copies;
// mutations do not affect the store.
func (s *Store) Snapshot() []GraphEntity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]GraphEntity, 0, len(s.entities))
	for _, e := range s.entities {
		result = append(result, *e)
	}
	return result
}

// Count returns the number of entities currently in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

// equal compares two GraphEntity values by JSON-marshalling both sides.
// This is intentionally simple: correctness over micro-optimisation. The store
// is written infrequently relative to reads, so the allocation cost is acceptable.
func equal(a, b *GraphEntity) bool {
	da, err := json.Marshal(a)
	if err != nil {
		return false
	}
	db, err := json.Marshal(b)
	if err != nil {
		return false
	}
	if len(da) != len(db) {
		return false
	}
	for i := range da {
		if da[i] != db[i] {
			return false
		}
	}
	return true
}
