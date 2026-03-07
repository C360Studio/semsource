package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestPredicatesRegistered(t *testing.T) {
	// Document predicates
	docPredicates := []string{
		DocType,
		DocCategory,
		DocAppliesTo,
		DocSeverity,
		DocSummary,
		DocRequirements,
		DocContent,
		DocSection,
		DocChunkIndex,
		DocChunkCount,
		DocMimeType,
		DocFilePath,
		DocFileHash,
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
		RepoBranch,
		RepoStatus,
		RepoLanguages,
		RepoEntityCount,
		RepoLastIndexed,
		RepoAutoPull,
		RepoPullInterval,
		RepoLastCommit,
		RepoError,
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
		SourceName,
		SourceStatus,
		SourceProject,
		SourceAddedBy,
		SourceAddedAt,
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
		{DocCategory, DcType},
		{DocSummary, DcAbstract},
		{DocMimeType, DcFormat},
		{SourceName, vocabulary.DcTitle},
		{SourceAddedBy, vocabulary.ProvWasAttributedTo},
		{SourceAddedAt, vocabulary.ProvGeneratedAtTime},
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
		{DocCategory, "string"},
		{DocSeverity, "string"},
		{DocAppliesTo, "array"},
		{DocRequirements, "array"},
		{DocChunkIndex, "int"},
		{DocChunkCount, "int"},
		{RepoEntityCount, "int"},
		{RepoAutoPull, "bool"},
		{RepoLastIndexed, "datetime"},
		{SourceAddedAt, "datetime"},
		{SourceAddedBy, "entity_id"},
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

func TestEnumValues(t *testing.T) {
	// Verify enum constants have expected string values
	t.Run("DocCategoryTypes", func(t *testing.T) {
		if DocCategorySOP != "sop" {
			t.Error("DocCategorySOP should be 'sop'")
		}
		if DocCategorySpec != "spec" {
			t.Error("DocCategorySpec should be 'spec'")
		}
	})

	t.Run("DocSeverityTypes", func(t *testing.T) {
		if DocSeverityError != "error" {
			t.Error("DocSeverityError should be 'error'")
		}
		if DocSeverityWarning != "warning" {
			t.Error("DocSeverityWarning should be 'warning'")
		}
		if DocSeverityInfo != "info" {
			t.Error("DocSeverityInfo should be 'info'")
		}
	})

	t.Run("SourceStatusTypes", func(t *testing.T) {
		if SourceStatusPending != "pending" {
			t.Error("SourceStatusPending should be 'pending'")
		}
		if SourceStatusReady != "ready" {
			t.Error("SourceStatusReady should be 'ready'")
		}
	})

	t.Run("SourceTypeValues", func(t *testing.T) {
		if SourceTypeRepository != "repository" {
			t.Error("SourceTypeRepository should be 'repository'")
		}
		if SourceTypeDocument != "document" {
			t.Error("SourceTypeDocument should be 'document'")
		}
	})
}
