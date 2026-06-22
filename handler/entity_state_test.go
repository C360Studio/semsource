package handler

import (
	"strings"
	"testing"

	"github.com/c360studio/semstreams/message"
)

func TestValidateSelfSubjectState(t *testing.T) {
	t.Run("valid relationship object", func(t *testing.T) {
		state := &EntityState{
			ID: "acme.semsource.config.repo.gomod.app",
			Triples: []message.Triple{
				{
					Subject:   "acme.semsource.config.repo.gomod.app",
					Predicate: "source.config.requires",
					Object:    "acme.semsource.config.repo.dependency.dep",
				},
			},
		}
		if err := ValidateSelfSubjectState(state); err != nil {
			t.Fatalf("ValidateSelfSubjectState() error = %v", err)
		}
	})

	t.Run("nil state", func(t *testing.T) {
		if err := ValidateSelfSubjectState(nil); err == nil {
			t.Fatal("expected error for nil state")
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		if err := ValidateSelfSubjectState(&EntityState{}); err == nil {
			t.Fatal("expected error for empty ID")
		}
	})

	t.Run("empty subject", func(t *testing.T) {
		state := &EntityState{
			ID:      "acme.semsource.web.docs.doc.abc123",
			Triples: []message.Triple{{Predicate: "source.doc.content", Object: "text"}},
		}
		if err := ValidateSelfSubjectState(state); err == nil || !strings.Contains(err.Error(), "empty subject") {
			t.Fatalf("expected empty subject error, got %v", err)
		}
	})

	t.Run("foreign subject", func(t *testing.T) {
		state := &EntityState{
			ID: "acme.semsource.web.docs.doc.abc123",
			Triples: []message.Triple{
				{
					Subject:   "acme.semsource.web.docs.doc.other",
					Predicate: "source.doc.content",
					Object:    "text",
				},
			},
		}
		if err := ValidateSelfSubjectState(state); err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("expected subject mismatch error, got %v", err)
		}
	})
}

func TestValidateSelfSubjectStates_IncludesFailingIndex(t *testing.T) {
	states := []*EntityState{
		{
			ID:      "acme.semsource.web.docs.doc.good",
			Triples: []message.Triple{{Subject: "acme.semsource.web.docs.doc.good"}},
		},
		{
			ID:      "acme.semsource.web.docs.doc.bad",
			Triples: []message.Triple{{Subject: "acme.semsource.web.docs.doc.other"}},
		},
	}

	err := ValidateSelfSubjectStates(states)
	if err == nil || !strings.Contains(err.Error(), "state 1") {
		t.Fatalf("expected state index in error, got %v", err)
	}
}

func TestValidateEntityStateID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		state := &EntityState{ID: "acme.semsource.web.docs.doc.abc123"}
		if err := ValidateEntityStateID(state); err != nil {
			t.Fatalf("ValidateEntityStateID() error = %v", err)
		}
	})

	t.Run("nil state", func(t *testing.T) {
		if err := ValidateEntityStateID(nil); err == nil {
			t.Fatal("expected error for nil state")
		}
	})

	t.Run("wrong part count", func(t *testing.T) {
		state := &EntityState{ID: "acme.semsource.web.doc.abc123"}
		if err := ValidateEntityStateID(state); err == nil || !strings.Contains(err.Error(), "want 6") {
			t.Fatalf("expected six-part error, got %v", err)
		}
	})

	t.Run("empty segment", func(t *testing.T) {
		state := &EntityState{ID: "acme.semsource.web..doc.abc123"}
		if err := ValidateEntityStateID(state); err == nil || !strings.Contains(err.Error(), "empty part") {
			t.Fatalf("expected empty segment error, got %v", err)
		}
	})

	t.Run("not NATS KV safe", func(t *testing.T) {
		state := &EntityState{ID: "acme.semsource.web.docs.doc.bad*id"}
		if err := ValidateEntityStateID(state); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatalf("expected NATS KV error, got %v", err)
		}
	})
}

func TestValidateEntityStateIDs_IncludesFailingIndex(t *testing.T) {
	states := []*EntityState{
		{ID: "acme.semsource.web.docs.doc.good"},
		{ID: "acme.semsource.web.docs.doc.bad*id"},
	}

	err := ValidateEntityStateIDs(states)
	if err == nil || !strings.Contains(err.Error(), "state 1") {
		t.Fatalf("expected state index in error, got %v", err)
	}
}
