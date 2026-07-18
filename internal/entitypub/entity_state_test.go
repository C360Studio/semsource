package entitypub

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/message"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

func TestPayloadFromState_PreservesTraceIndexingProfile(t *testing.T) {
	state := &handler.EntityState{
		ID: "acme.semsource.media.videos.keyframe.demo-1s",
		Triples: []message.Triple{
			{
				Subject:   "acme.semsource.media.videos.keyframe.demo-1s",
				Predicate: "source.media.keyframe-of",
				Object:    "acme.semsource.media.videos.video.demo",
			},
		},
		UpdatedAt:       time.Now().UTC(),
		IndexingProfile: semvocab.IndexingProfileTrace,
	}

	payload, err := PayloadFromState(state)
	if err != nil {
		t.Fatalf("PayloadFromState() error = %v", err)
	}
	if payload.IndexingProfile() != semvocab.IndexingProfileTrace {
		t.Fatalf("IndexingProfile() = %q, want %q", payload.IndexingProfile(), semvocab.IndexingProfileTrace)
	}
	if payload.IndexingProfile() == semvocab.IndexingProfileContent {
		t.Fatal("trace entity was converted to content profile")
	}
}

func TestPayloadFromState_RejectsInvalidState(t *testing.T) {
	t.Run("foreign subject", func(t *testing.T) {
		state := &handler.EntityState{
			ID: "acme.semsource.media.videos.keyframe.demo-1s",
			Triples: []message.Triple{
				{
					Subject:   "acme.semsource.media.videos.video.demo",
					Predicate: "source.media.keyframe-of",
					Object:    "acme.semsource.media.videos.video.demo",
				},
			},
			UpdatedAt:       time.Now().UTC(),
			IndexingProfile: semvocab.IndexingProfileTrace,
		}

		_, err := PayloadFromState(state)
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("expected subject validation error, got %v", err)
		}
	})

	t.Run("missing profile", func(t *testing.T) {
		state := &handler.EntityState{
			ID:        "acme.semsource.web.docs.doc.abc123",
			Triples:   []message.Triple{{Subject: "acme.semsource.web.docs.doc.abc123"}},
			UpdatedAt: time.Now().UTC(),
		}

		_, err := PayloadFromState(state)
		if err == nil || !strings.Contains(err.Error(), "indexing profile is required") {
			t.Fatalf("expected indexing profile error, got %v", err)
		}
	})
}
