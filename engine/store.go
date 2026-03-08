package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/federation"
)

// entityStore is a goroutine-safe in-memory store for federation entities.
// It is the authoritative source of entity state within a single semsource instance.
type entityStore struct {
	mu       sync.RWMutex
	entities map[string]*federation.Entity
}

func newEntityStore() *entityStore {
	return &entityStore{
		entities: make(map[string]*federation.Entity),
	}
}

// Upsert stores the entity, replacing any existing entry with the same ID.
// Returns true if the entity was added or its content changed.
func (s *entityStore) Upsert(e *federation.Entity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.entities[e.ID]
	if ok && entityEqual(existing, e) {
		return false
	}

	clone := *e
	s.entities[e.ID] = &clone
	return true
}

// Remove deletes the entity with the given ID.
func (s *entityStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entities, id)
}

// Snapshot returns a copy of all entities.
func (s *entityStore) Snapshot() []*federation.Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*federation.Entity, 0, len(s.entities))
	for _, e := range s.entities {
		clone := *e
		result = append(result, &clone)
	}
	return result
}

// entityEqual compares two entities by semantic content: ID + triples (subject,
// predicate, object). Timestamps and provenance are excluded because the
// normalizer generates fresh timestamps on every pass — comparing them would
// defeat deduplication for unchanged entities.
func entityEqual(a, b *federation.Entity) bool {
	if a.ID != b.ID {
		return false
	}
	if len(a.Triples) != len(b.Triples) {
		return false
	}
	return tripleFingerprint(a) == tripleFingerprint(b)
}

// tripleFingerprint builds a deterministic string from the entity's triples,
// sorted by subject+predicate+object. This ignores timestamp and confidence.
func tripleFingerprint(e *federation.Entity) string {
	parts := make([]string, len(e.Triples))
	for i, t := range e.Triples {
		parts[i] = fmt.Sprintf("%s\x00%s\x00%v", t.Subject, t.Predicate, t.Object)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x01")
}
