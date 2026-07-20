package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestPredicatesRegistered(t *testing.T) {
	// Document predicates
	docPredicates := []string{
		DocType,
		DocSummary,
		DocContent,
		DocSection,
		DocChunkIndex,
		DocChunkCount,
		DocMimeType,
		DocFilePath,
		DocFileHash,
		DocBodyStore,
		DocBodyKey,
	}

	for _, pred := range docPredicates {
		t.Run(pred, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(pred)
			if meta.Description == "" {
				t.Errorf("predicate %s not registered or missing description", pred)
			}
		})
	}

	// Repository predicates
	repoPredicates := []string{
		RepoType,
		RepoURL,
	}

	for _, pred := range repoPredicates {
		t.Run(pred, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(pred)
			if meta.Description == "" {
				t.Errorf("predicate %s not registered or missing description", pred)
			}
		})
	}

	// Generic source predicates
	sourcePredicates := []string{
		SourceType,
		SourceStatus,
		SourceError,
	}

	for _, pred := range sourcePredicates {
		t.Run(pred, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(pred)
			if meta.Description == "" {
				t.Errorf("predicate %s not registered or missing description", pred)
			}
		})
	}
}

func TestPredicateIRIMappings(t *testing.T) {
	tests := []struct {
		predicate   string
		expectedIRI string
	}{
		{DocSummary, DcAbstract},
		{DocMimeType, DcFormat},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(tt.predicate)
			if meta == nil {
				t.Fatalf("predicate %s not registered", tt.predicate)
			}
			if meta.StandardIRI != tt.expectedIRI {
				t.Errorf("predicate %s: expected IRI %s, got %s", tt.predicate, tt.expectedIRI, meta.StandardIRI)
			}
		})
	}
}

func TestPredicateDataTypes(t *testing.T) {
	tests := []struct {
		predicate    string
		expectedType string
	}{
		{DocChunkIndex, "int"},
		{DocChunkCount, "int"},
		{DocBodyStore, "string"},
		{DocBodyKey, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(tt.predicate)
			if meta.DataType != tt.expectedType {
				t.Errorf("predicate %s: expected type %s, got %s", tt.predicate, tt.expectedType, meta.DataType)
			}
		})
	}
}

func TestMediaTypeValues(t *testing.T) {
	// The media handlers emit these as string literals; pin the typed constants
	// so the two cannot drift apart silently.
	cases := map[MediaTypeValue]string{
		MediaTypeImage:    "image",
		MediaTypeVideo:    "video",
		MediaTypeKeyframe: "keyframe",
		MediaTypeAudio:    "audio",
		MediaTypeBinary:   "binary",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("media type constant = %q, want %q", got, want)
		}
	}
}
