package graph_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/message"
)

func TestEntityPayload_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := &graph.EntityPayload{
		ID: "acme.semsource.git.my-repo.commit.a1b2c3",
		TripleData: []message.Triple{
			{
				Subject:    "acme.semsource.git.my-repo.commit.a1b2c3",
				Predicate:  "git.commit.sha",
				Object:     "a1b2c3",
				Source:     "semsource",
				Timestamp:  now,
				Confidence: 1.0,
			},
		},
		UpdatedAt: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	restored := &graph.EntityPayload{}
	if err := json.Unmarshal(data, restored); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if len(restored.TripleData) != len(original.TripleData) {
		t.Fatalf("TripleData len = %d, want %d", len(restored.TripleData), len(original.TripleData))
	}
	if restored.TripleData[0].Predicate != "git.commit.sha" {
		t.Errorf("Triple predicate = %q, want %q", restored.TripleData[0].Predicate, "git.commit.sha")
	}
}

func TestEntityPayload_Graphable(t *testing.T) {
	p := &graph.EntityPayload{
		ID: "acme.semsource.golang.my-repo.function.New",
		TripleData: []message.Triple{
			{Subject: "acme.semsource.golang.my-repo.function.New", Predicate: "golang.function.name", Object: "New"},
		},
	}

	if p.EntityID() != p.ID {
		t.Errorf("EntityID() = %q, want %q", p.EntityID(), p.ID)
	}
	if len(p.Triples()) != 1 {
		t.Errorf("Triples() len = %d, want 1", len(p.Triples()))
	}
}

func TestEntityPayload_Schema(t *testing.T) {
	p := &graph.EntityPayload{}
	schema := p.Schema()

	if schema.Domain != "semsource" {
		t.Errorf("Domain = %q, want %q", schema.Domain, "semsource")
	}
	if schema.Category != "entity" {
		t.Errorf("Category = %q, want %q", schema.Category, "entity")
	}
	if schema.Version != "v1" {
		t.Errorf("Version = %q, want %q", schema.Version, "v1")
	}
}

func TestEntityPayload_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		p := &graph.EntityPayload{ID: "acme.semsource.git.repo.commit.abc"}
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		p := &graph.EntityPayload{}
		if err := p.Validate(); err == nil {
			t.Error("expected error for empty ID")
		}
	})
}
