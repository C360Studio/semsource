package handler_test

import (
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
)

func TestRawEntity_Construction(t *testing.T) {
	t.Run("valid entity with all fields", func(t *testing.T) {
		e := handler.RawEntity{
			SourceType: "ast",
			Domain:     "golang",
			System:     "github.com-acme-gcs",
			EntityType: "function",
			Instance:   "NewController",
			Properties: map[string]any{
				"file": "internal/controller/controller.go",
				"line": 42,
			},
		}

		if e.SourceType != "ast" {
			t.Errorf("expected SourceType=ast, got %s", e.SourceType)
		}
		if e.Domain != "golang" {
			t.Errorf("expected Domain=golang, got %s", e.Domain)
		}
		if e.System != "github.com-acme-gcs" {
			t.Errorf("expected System=github.com-acme-gcs, got %s", e.System)
		}
		if e.EntityType != "function" {
			t.Errorf("expected EntityType=function, got %s", e.EntityType)
		}
		if e.Instance != "NewController" {
			t.Errorf("expected Instance=NewController, got %s", e.Instance)
		}
		if len(e.Properties) != 2 {
			t.Errorf("expected 2 properties, got %d", len(e.Properties))
		}
	})

	t.Run("entity with no properties is valid", func(t *testing.T) {
		e := handler.RawEntity{
			SourceType: "git",
			Domain:     "git",
			System:     "github.com-acme-gcs",
			EntityType: "commit",
			Instance:   "a3f9b2",
		}

		if e.Properties != nil {
			t.Errorf("expected nil properties for zero-value entity")
		}
	})

	t.Run("source type constants are correct", func(t *testing.T) {
		tests := []struct {
			name string
			got  string
			want string
		}{
			{"SourceTypeGit", handler.SourceTypeGit, "git"},
			{"SourceTypeAST", handler.SourceTypeAST, "ast"},
			{"SourceTypeDoc", handler.SourceTypeDoc, "doc"},
			{"SourceTypeConfig", handler.SourceTypeConfig, "config"},
			{"SourceTypeURL", handler.SourceTypeURL, "url"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.got != tt.want {
					t.Errorf("expected %s, got %s", tt.want, tt.got)
				}
			})
		}
	})

	t.Run("domain constants are correct", func(t *testing.T) {
		tests := []struct {
			name string
			got  string
			want string
		}{
			{"DomainGolang", handler.DomainGolang, "golang"},
			{"DomainGit", handler.DomainGit, "git"},
			{"DomainWeb", handler.DomainWeb, "web"},
			{"DomainConfig", handler.DomainConfig, "config"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.got != tt.want {
					t.Errorf("expected %s, got %s", tt.want, tt.got)
				}
			})
		}
	})
}

func TestRawEdge_Construction(t *testing.T) {
	t.Run("edge with all fields", func(t *testing.T) {
		e := handler.RawEdge{
			FromHint: "NewController",
			ToHint:   "ProcessData",
			EdgeType: "calls",
			Weight:   1.0,
		}

		if e.FromHint != "NewController" {
			t.Errorf("expected FromHint=NewController, got %s", e.FromHint)
		}
		if e.ToHint != "ProcessData" {
			t.Errorf("expected ToHint=ProcessData, got %s", e.ToHint)
		}
		if e.EdgeType != "calls" {
			t.Errorf("expected EdgeType=calls, got %s", e.EdgeType)
		}
		if e.Weight != 1.0 {
			t.Errorf("expected Weight=1.0, got %f", e.Weight)
		}
	})

	t.Run("edge with zero weight is valid", func(t *testing.T) {
		e := handler.RawEdge{
			FromHint: "A",
			ToHint:   "B",
			EdgeType: "depends",
		}
		if e.Weight != 0.0 {
			t.Errorf("expected zero weight, got %f", e.Weight)
		}
	})
}

func TestChangeOperation_Values(t *testing.T) {
	tests := []struct {
		name string
		got  handler.ChangeOperation
		want handler.ChangeOperation
	}{
		{"create", handler.OperationCreate, "create"},
		{"modify", handler.OperationModify, "modify"},
		{"delete", handler.OperationDelete, "delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, tt.got)
			}
		})
	}
}

func TestChangeEvent_Construction(t *testing.T) {
	t.Run("create event with entities", func(t *testing.T) {
		now := time.Now()
		ev := handler.ChangeEvent{
			Path:      "/project/internal/controller.go",
			Operation: handler.OperationCreate,
			Timestamp: now,
			Entities: []handler.RawEntity{
				{
					SourceType: "ast",
					Domain:     "golang",
					System:     "github.com-acme-gcs",
					EntityType: "function",
					Instance:   "NewController",
				},
			},
		}

		if ev.Path != "/project/internal/controller.go" {
			t.Errorf("unexpected Path: %s", ev.Path)
		}
		if ev.Operation != handler.OperationCreate {
			t.Errorf("unexpected Operation: %s", ev.Operation)
		}
		if !ev.Timestamp.Equal(now) {
			t.Errorf("unexpected Timestamp")
		}
		if len(ev.Entities) != 1 {
			t.Errorf("expected 1 entity, got %d", len(ev.Entities))
		}
	})

	t.Run("delete event with no entities is valid", func(t *testing.T) {
		ev := handler.ChangeEvent{
			Path:      "/project/removed.go",
			Operation: handler.OperationDelete,
			Timestamp: time.Now(),
		}
		if len(ev.Entities) != 0 {
			t.Errorf("expected 0 entities for delete event, got %d", len(ev.Entities))
		}
	})

	t.Run("modify event with multiple entities", func(t *testing.T) {
		ev := handler.ChangeEvent{
			Path:      "/project/pkg/service.go",
			Operation: handler.OperationModify,
			Timestamp: time.Now(),
			Entities: []handler.RawEntity{
				{SourceType: "ast", Domain: "golang", System: "github.com-acme-gcs", EntityType: "function", Instance: "Process"},
				{SourceType: "ast", Domain: "golang", System: "github.com-acme-gcs", EntityType: "function", Instance: "Validate"},
			},
		}
		if len(ev.Entities) != 2 {
			t.Errorf("expected 2 entities, got %d", len(ev.Entities))
		}
	})
}

func TestRawEntity_WithEdges(t *testing.T) {
	t.Run("entity with edges", func(t *testing.T) {
		e := handler.RawEntity{
			SourceType: "ast",
			Domain:     "golang",
			System:     "github.com-acme-gcs",
			EntityType: "function",
			Instance:   "NewController",
			Edges: []handler.RawEdge{
				{FromHint: "NewController", ToHint: "ProcessData", EdgeType: "calls", Weight: 1.0},
				{FromHint: "NewController", ToHint: "Repository", EdgeType: "depends", Weight: 0.5},
			},
		}
		if len(e.Edges) != 2 {
			t.Errorf("expected 2 edges, got %d", len(e.Edges))
		}
		if e.Edges[0].EdgeType != "calls" {
			t.Errorf("expected first edge type=calls, got %s", e.Edges[0].EdgeType)
		}
	})
}
