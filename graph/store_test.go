package graph_test

import (
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

func makeEntity(id string) graph.GraphEntity {
	now := time.Now()
	return graph.GraphEntity{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: "test.prop", Object: "value", Timestamp: now},
		},
		Edges: []federation.Edge{},
		Provenance: graph.SourceProvenance{
			SourceType: "git",
			SourceID:   "test-source",
			Timestamp:  now,
			Handler:    "TestHandler",
		},
	}
}

func TestStore_Upsert_NewEntity(t *testing.T) {
	s := graph.NewStore()
	e := makeEntity("acme.semsource.git.repo.commit.abc123")

	changed := s.Upsert(e)
	if !changed {
		t.Error("Upsert of new entity: got changed=false, want true")
	}
	if s.Count() != 1 {
		t.Errorf("Count after first Upsert: got %d, want 1", s.Count())
	}
}

func TestStore_Upsert_SameEntityUnchanged(t *testing.T) {
	s := graph.NewStore()
	e := makeEntity("acme.semsource.git.repo.commit.abc123")

	s.Upsert(e)
	changed := s.Upsert(e)
	if changed {
		t.Error("Upsert of identical entity: got changed=true, want false")
	}
	if s.Count() != 1 {
		t.Errorf("Count after duplicate Upsert: got %d, want 1", s.Count())
	}
}

func TestStore_Upsert_UpdatedEntity(t *testing.T) {
	s := graph.NewStore()
	id := "acme.semsource.git.repo.commit.abc123"
	e1 := makeEntity(id)
	s.Upsert(e1)

	// Modify the triples to simulate a change
	e2 := makeEntity(id)
	e2.Triples = append(e2.Triples, message.Triple{
		Subject:   id,
		Predicate: "test.extra",
		Object:    "new-value",
		Timestamp: time.Now(),
	})

	changed := s.Upsert(e2)
	if !changed {
		t.Error("Upsert of modified entity: got changed=false, want true")
	}
}

func TestStore_Remove_Existing(t *testing.T) {
	s := graph.NewStore()
	id := "acme.semsource.git.repo.commit.abc123"
	s.Upsert(makeEntity(id))

	removed := s.Remove(id)
	if !removed {
		t.Error("Remove of existing entity: got removed=false, want true")
	}
	if s.Count() != 0 {
		t.Errorf("Count after Remove: got %d, want 0", s.Count())
	}
}

func TestStore_Remove_Nonexistent(t *testing.T) {
	s := graph.NewStore()

	removed := s.Remove("nonexistent.id")
	if removed {
		t.Error("Remove of nonexistent entity: got removed=true, want false")
	}
}

func TestStore_Snapshot_Empty(t *testing.T) {
	s := graph.NewStore()
	snap := s.Snapshot()
	if snap == nil {
		t.Error("Snapshot of empty store: got nil, want empty slice")
	}
	if len(snap) != 0 {
		t.Errorf("Snapshot of empty store: got %d entities, want 0", len(snap))
	}
}

func TestStore_Snapshot_Contents(t *testing.T) {
	s := graph.NewStore()
	ids := []string{
		"acme.semsource.git.repo.commit.abc123",
		"acme.semsource.git.repo.commit.def456",
		"acme.semsource.git.repo.commit.ghi789",
	}
	for _, id := range ids {
		s.Upsert(makeEntity(id))
	}

	snap := s.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot count: got %d, want 3", len(snap))
	}

	// Verify all IDs are present
	seen := make(map[string]bool)
	for _, e := range snap {
		seen[e.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("Snapshot missing entity %q", id)
		}
	}
}

func TestStore_Snapshot_IsCopy(t *testing.T) {
	s := graph.NewStore()
	id := "acme.semsource.git.repo.commit.abc123"
	s.Upsert(makeEntity(id))

	snap := s.Snapshot()
	// Mutating the snapshot should not affect the store
	snap[0].ID = "mutated"
	if s.Count() != 1 {
		t.Error("Snapshot mutation affected store count")
	}
	snap2 := s.Snapshot()
	if snap2[0].ID == "mutated" {
		t.Error("Snapshot mutation leaked into store")
	}
}

func TestStore_Count(t *testing.T) {
	s := graph.NewStore()
	if s.Count() != 0 {
		t.Errorf("Initial count: got %d, want 0", s.Count())
	}

	for i := range 5 {
		s.Upsert(makeEntity("acme.semsource.git.repo.commit." + string(rune('a'+i))))
	}
	if s.Count() != 5 {
		t.Errorf("Count after 5 upserts: got %d, want 5", s.Count())
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := graph.NewStore()
	const goroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			for op := range opsPerGoroutine {
				id := "acme.semsource.git.repo.commit." + string(rune('a'+g%26)) + string(rune('a'+op%26))
				s.Upsert(makeEntity(id))
				_ = s.Count()
				_ = s.Snapshot()
				if op%5 == 0 {
					s.Remove(id)
				}
			}
		}(g)
	}

	wg.Wait()
	// If the race detector doesn't flag anything, the test passes.
}
