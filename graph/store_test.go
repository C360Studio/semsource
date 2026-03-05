package graph_test

import (
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/message"
)

func makeEntity(id string, triples ...message.Triple) *graph.GraphEntity {
	return &graph.GraphEntity{
		ID:      id,
		Triples: triples,
		Provenance: graph.SourceProvenance{
			SourceType: "test",
			SourceID:   "test-source",
			Timestamp:  time.Now(),
			Handler:    "TestHandler",
		},
	}
}

func TestStore_Upsert_NewEntity(t *testing.T) {
	s := graph.NewStore()

	entity := makeEntity("acme.semsource.git.repo.commit.a1b2c3",
		message.Triple{Subject: "acme.semsource.git.repo.commit.a1b2c3", Predicate: "git.sha", Object: "a1b2c3"},
	)

	changed := s.Upsert(entity)
	if !changed {
		t.Error("Upsert() should return true for new entity")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Upsert_UnchangedEntity(t *testing.T) {
	s := graph.NewStore()

	entity := makeEntity("acme.semsource.git.repo.commit.a1b2c3",
		message.Triple{Subject: "acme.semsource.git.repo.commit.a1b2c3", Predicate: "git.sha", Object: "a1b2c3"},
	)

	s.Upsert(entity)
	changed := s.Upsert(entity)
	if changed {
		t.Error("Upsert() should return false when entity is unchanged")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Upsert_ChangedEntity(t *testing.T) {
	s := graph.NewStore()

	id := "acme.semsource.git.repo.commit.a1b2c3"
	entity1 := makeEntity(id,
		message.Triple{Subject: id, Predicate: "git.sha", Object: "a1b2c3"},
	)
	entity2 := makeEntity(id,
		message.Triple{Subject: id, Predicate: "git.sha", Object: "a1b2c3"},
		message.Triple{Subject: id, Predicate: "git.message", Object: "fix: bug"},
	)

	s.Upsert(entity1)
	changed := s.Upsert(entity2)
	if !changed {
		t.Error("Upsert() should return true when entity content changes")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Remove_Existing(t *testing.T) {
	s := graph.NewStore()

	id := "acme.semsource.git.repo.commit.a1b2c3"
	s.Upsert(makeEntity(id))

	removed := s.Remove(id)
	if !removed {
		t.Error("Remove() should return true for existing entity")
	}
	if s.Count() != 0 {
		t.Errorf("Count() = %d, want 0", s.Count())
	}
}

func TestStore_Remove_NonExistent(t *testing.T) {
	s := graph.NewStore()

	removed := s.Remove("nonexistent.id.here.foo.bar.baz")
	if removed {
		t.Error("Remove() should return false for non-existent entity")
	}
}

func TestStore_Snapshot(t *testing.T) {
	s := graph.NewStore()

	entities := []*graph.GraphEntity{
		makeEntity("acme.semsource.git.repo.commit.a1b2c3"),
		makeEntity("acme.semsource.git.repo.commit.d4e5f6"),
		makeEntity("acme.semsource.git.repo.author.alice"),
	}

	for _, e := range entities {
		s.Upsert(e)
	}

	snap := s.Snapshot()
	if len(snap) != 3 {
		t.Errorf("Snapshot() len = %d, want 3", len(snap))
	}

	// Verify snapshot is independent copy — mutations don't affect store
	snap[0].ID = "mutated"
	snap2 := s.Snapshot()
	for _, e := range snap2 {
		if e.ID == "mutated" {
			t.Error("Snapshot() should return copies, not references")
		}
	}
}

func TestStore_Snapshot_Empty(t *testing.T) {
	s := graph.NewStore()
	snap := s.Snapshot()
	if snap == nil {
		t.Error("Snapshot() should return empty slice, not nil")
	}
	if len(snap) != 0 {
		t.Errorf("Snapshot() len = %d, want 0", len(snap))
	}
}

func TestStore_Count(t *testing.T) {
	s := graph.NewStore()

	if s.Count() != 0 {
		t.Errorf("Count() = %d, want 0", s.Count())
	}

	s.Upsert(makeEntity("acme.semsource.git.repo.commit.a1b2c3"))
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}

	s.Upsert(makeEntity("acme.semsource.git.repo.commit.d4e5f6"))
	if s.Count() != 2 {
		t.Errorf("Count() = %d, want 2", s.Count())
	}

	s.Remove("acme.semsource.git.repo.commit.a1b2c3")
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after remove", s.Count())
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := graph.NewStore()
	const workers = 20
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(workerID int) {
			defer wg.Done()
			for j := range ops {
				id := "acme.semsource.git.repo.commit.test"
				// Vary ID slightly to exercise concurrent inserts
				if j%2 == 0 {
					id = "acme.semsource.git.repo.author.alice"
				}

				switch j % 4 {
				case 0, 1:
					s.Upsert(makeEntity(id))
				case 2:
					s.Remove(id)
				case 3:
					_ = s.Snapshot()
					_ = s.Count()
				}
				_ = workerID
			}
		}(i)
	}

	wg.Wait()
	// No panic or race = success; final count is non-deterministic
}
