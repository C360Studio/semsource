package graph_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

func TestGraphEventPayload_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := &graph.GraphEventPayload{
		Event: federation.Event{
			Type:      federation.EventTypeDELTA,
			SourceID:  "my-source",
			Namespace: "acme",
			Timestamp: now,
			Entities: []federation.Entity{
				{
					ID: "acme.semsource.git.my-repo.commit.a1b2c3",
					Triples: []message.Triple{
						{
							Subject:   "acme.semsource.git.my-repo.commit.a1b2c3",
							Predicate: "git.commit.sha",
							Object:    "a1b2c3",
							Timestamp: now,
						},
					},
					Edges: []federation.Edge{
						{
							FromID:   "acme.semsource.git.my-repo.commit.a1b2c3",
							ToID:     "acme.semsource.git.my-repo.author.alice",
							EdgeType: "authored_by",
							Weight:   1.0,
						},
					},
					Provenance: federation.Provenance{
						SourceType: "git",
						SourceID:   "my-source",
						Timestamp:  now,
						Handler:    "GitHandler",
					},
				},
			},
			Provenance: federation.Provenance{
				SourceType: "git",
				SourceID:   "my-source",
				Timestamp:  now,
				Handler:    "GitHandler",
			},
		},
	}

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	restored := &graph.GraphEventPayload{}
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if restored.Event.Type != original.Event.Type {
		t.Errorf("Type mismatch: got %v, want %v", restored.Event.Type, original.Event.Type)
	}
	if restored.Event.SourceID != original.Event.SourceID {
		t.Errorf("SourceID mismatch: got %v, want %v", restored.Event.SourceID, original.Event.SourceID)
	}
	if len(restored.Event.Entities) != len(original.Event.Entities) {
		t.Fatalf("Entities count mismatch: got %d, want %d", len(restored.Event.Entities), len(original.Event.Entities))
	}
	if restored.Event.Entities[0].ID != original.Event.Entities[0].ID {
		t.Errorf("Entity ID mismatch: got %v, want %v", restored.Event.Entities[0].ID, original.Event.Entities[0].ID)
	}
}

func TestGraphEventPayload_Schema(t *testing.T) {
	p := &graph.GraphEventPayload{}
	schema := p.Schema()

	if schema.Domain != "semsource" {
		t.Errorf("Schema Domain = %q, want %q", schema.Domain, "semsource")
	}
	if schema.Category != "graph_event" {
		t.Errorf("Schema Category = %q, want %q", schema.Category, "graph_event")
	}
	if schema.Version != "v1" {
		t.Errorf("Schema Version = %q, want %q", schema.Version, "v1")
	}
}

func TestGraphEventPayload_Validate(t *testing.T) {
	now := time.Now()

	t.Run("valid payload", func(t *testing.T) {
		p := &graph.GraphEventPayload{
			Event: federation.Event{
				Type:      federation.EventTypeSEED,
				SourceID:  "my-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "my-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
		}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("invalid event", func(t *testing.T) {
		p := &graph.GraphEventPayload{
			Event: federation.Event{},
		}
		if err := p.Validate(); err == nil {
			t.Error("Validate() expected error for empty event")
		}
	})
}

func TestGraphEventPayload_PayloadRegistration(t *testing.T) {
	p := &graph.GraphEventPayload{}
	schema := p.Schema()

	event := federation.Event{
		Type:      federation.EventTypeHEARTBEAT,
		SourceID:  "heartbeat-source",
		Namespace: "acme",
		Timestamp: time.Now(),
		Provenance: federation.Provenance{
			SourceType: "internal",
			SourceID:   "heartbeat-source",
			Timestamp:  time.Now(),
			Handler:    "Engine",
		},
	}

	payload := &graph.GraphEventPayload{Event: event}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	restored := &graph.GraphEventPayload{}
	if err := json.Unmarshal(data, restored); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if restored.Schema() != schema {
		t.Errorf("Schema mismatch after round-trip")
	}
}
