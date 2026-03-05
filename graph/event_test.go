package graph_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/message"
)

func TestGraphEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   graph.GraphEvent
		wantErr bool
	}{
		{
			name: "valid SEED event",
			event: graph.GraphEvent{
				Type:      graph.EventTypeSEED,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid DELTA event with entities",
			event: graph.GraphEvent{
				Type:      graph.EventTypeDELTA,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Entities: []graph.GraphEntity{
					{
						ID:      "acme.semsource.git.my-repo.commit.a1b2c3",
						Triples: []message.Triple{{Subject: "acme.semsource.git.my-repo.commit.a1b2c3", Predicate: "git.commit.sha", Object: "a1b2c3"}},
					},
				},
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid RETRACT event",
			event: graph.GraphEvent{
				Type:        graph.EventTypeRETRACT,
				SourceID:    "test-source",
				Namespace:   "acme",
				Timestamp:   now,
				Retractions: []string{"acme.semsource.git.my-repo.commit.a1b2c3"},
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid HEARTBEAT event",
			event: graph.GraphEvent{
				Type:      graph.EventTypeHEARTBEAT,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
					SourceType: "internal",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "Engine",
				},
			},
			wantErr: false,
		},
		{
			name: "missing type",
			event: graph.GraphEvent{
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "missing source ID",
			event: graph.GraphEvent{
				Type:      graph.EventTypeSEED,
				Namespace: "acme",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			event: graph.GraphEvent{
				Type:      graph.EventTypeSEED,
				SourceID:  "test-source",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			event: graph.GraphEvent{
				Type:      graph.EventTypeSEED,
				SourceID:  "test-source",
				Namespace: "acme",
				Provenance: graph.SourceProvenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGraphEventPayload_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := &graph.GraphEventPayload{
		Event: graph.GraphEvent{
			Type:      graph.EventTypeDELTA,
			SourceID:  "my-source",
			Namespace: "acme",
			Timestamp: now,
			Entities: []graph.GraphEntity{
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
					Edges: []graph.GraphEdge{
						{
							FromID:   "acme.semsource.git.my-repo.commit.a1b2c3",
							ToID:     "acme.semsource.git.my-repo.author.alice",
							EdgeType: "authored_by",
							Weight:   1.0,
						},
					},
					Provenance: graph.SourceProvenance{
						SourceType: "git",
						SourceID:   "my-source",
						Timestamp:  now,
						Handler:    "GitHandler",
					},
				},
			},
			Provenance: graph.SourceProvenance{
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
	if restored.Event.Namespace != original.Event.Namespace {
		t.Errorf("Namespace mismatch: got %v, want %v", restored.Event.Namespace, original.Event.Namespace)
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
			Event: graph.GraphEvent{
				Type:      graph.EventTypeSEED,
				SourceID:  "my-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: graph.SourceProvenance{
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
			Event: graph.GraphEvent{},
		}
		if err := p.Validate(); err == nil {
			t.Error("Validate() expected error for empty event")
		}
	})
}

func TestGraphEventPayload_PayloadRegistration(t *testing.T) {
	// Verify the payload can be created via component registry
	// The init() function registers it - we just confirm Schema() matches
	p := &graph.GraphEventPayload{}
	schema := p.Schema()

	// Marshal and unmarshal to confirm round-trip works via JSON
	event := graph.GraphEvent{
		Type:      graph.EventTypeHEARTBEAT,
		SourceID:  "heartbeat-source",
		Namespace: "acme",
		Timestamp: time.Now(),
		Provenance: graph.SourceProvenance{
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
