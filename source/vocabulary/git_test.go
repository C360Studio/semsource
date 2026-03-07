package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestGitDecisionPredicatesRegistered(t *testing.T) {
	predicates := []string{
		DecisionType,
		DecisionFile,
		DecisionCommit,
		DecisionMessage,
		DecisionBranch,
		DecisionAgent,
		DecisionLoop,
		DecisionProject,
		DecisionTimestamp,
		DecisionRepository,
		DecisionOperation,
	}

	for _, pred := range predicates {
		t.Run(pred, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(pred)
			if meta == nil {
				t.Fatalf("predicate %s not registered", pred)
			}
			if meta.Description == "" {
				t.Errorf("predicate %s missing description", pred)
			}
		})
	}
}

func TestGitDecisionPredicateIRIMappings(t *testing.T) {
	tests := []struct {
		predicate   string
		expectedIRI string
	}{
		{DecisionAgent, vocabulary.ProvWasAttributedTo},
		{DecisionTimestamp, vocabulary.ProvGeneratedAtTime},
		{DecisionType, Namespace + "decisionType"},
		{DecisionFile, Namespace + "decisionFile"},
		{DecisionCommit, Namespace + "decisionCommit"},
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

func TestGitDecisionPredicateDataTypes(t *testing.T) {
	tests := []struct {
		predicate    string
		expectedType string
	}{
		{DecisionType, "string"},
		{DecisionFile, "string"},
		{DecisionCommit, "string"},
		{DecisionMessage, "string"},
		{DecisionBranch, "string"},
		{DecisionAgent, "entity_id"},
		{DecisionLoop, "entity_id"},
		{DecisionProject, "entity_id"},
		{DecisionTimestamp, "datetime"},
		{DecisionRepository, "string"},
		{DecisionOperation, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			meta := vocabulary.GetPredicateMetadata(tt.predicate)
			if meta == nil {
				t.Fatalf("predicate %s not registered", tt.predicate)
			}
			if meta.DataType != tt.expectedType {
				t.Errorf("predicate %s: expected type %s, got %s", tt.predicate, tt.expectedType, meta.DataType)
			}
		})
	}
}

func TestDecisionTypeEnumValues(t *testing.T) {
	tests := []struct {
		value    DecisionTypeValue
		expected string
	}{
		{DecisionTypeFeat, "feat"},
		{DecisionTypeFix, "fix"},
		{DecisionTypeRefactor, "refactor"},
		{DecisionTypeDocs, "docs"},
		{DecisionTypeTest, "test"},
		{DecisionTypeChore, "chore"},
		{DecisionTypePerf, "perf"},
		{DecisionTypeCI, "ci"},
		{DecisionTypeBuild, "build"},
		{DecisionTypeRevert, "revert"},
		{DecisionTypeStyle, "style"},
	}

	for _, tt := range tests {
		t.Run(string(tt.value), func(t *testing.T) {
			if string(tt.value) != tt.expected {
				t.Errorf("DecisionTypeValue %v should be '%s'", tt.value, tt.expected)
			}
		})
	}
}

func TestFileOperationTypeEnumValues(t *testing.T) {
	tests := []struct {
		value    FileOperationType
		expected string
	}{
		{FileOperationAdd, "add"},
		{FileOperationModify, "modify"},
		{FileOperationDelete, "delete"},
		{FileOperationRename, "rename"},
	}

	for _, tt := range tests {
		t.Run(string(tt.value), func(t *testing.T) {
			if string(tt.value) != tt.expected {
				t.Errorf("FileOperationType %v should be '%s'", tt.value, tt.expected)
			}
		})
	}
}

func TestDecisionClassIRIs(t *testing.T) {
	if ClassDecision != Namespace+"Decision" {
		t.Errorf("ClassDecision should be '%sDecision', got '%s'", Namespace, ClassDecision)
	}
	if ClassFileDecision != Namespace+"FileDecision" {
		t.Errorf("ClassFileDecision should be '%sFileDecision', got '%s'", Namespace, ClassFileDecision)
	}
}
