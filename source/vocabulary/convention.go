package source

import "github.com/c360studio/semstreams/vocabulary"

// Convention predicates for patterns learned from approved code.
// Conventions are extracted by the reviewer and stored in the graph
// for assembly into future reviewer contexts.
const (
	// ConventionName is the short name for the convention.
	// Example: "Error wrapping with context"
	ConventionName = "semspec.convention.name"

	// ConventionPattern is the pattern description.
	// Natural language description of what the convention specifies.
	// Example: "Use fmt.Errorf with %w for error wrapping"
	ConventionPattern = "semspec.convention.pattern"

	// ConventionAppliesTo specifies file patterns this convention applies to.
	// Format: glob patterns like "*.go", "handlers/*.go"
	ConventionAppliesTo = "semspec.convention.applies_to"

	// ConventionSourceTask is the task ID where this convention was learned.
	// Links the convention to its origin for provenance.
	ConventionSourceTask = "semspec.convention.source_task"

	// ConventionApprovedBy is the reviewer model that extracted this convention.
	// Example: "claude-sonnet-4-20250514"
	ConventionApprovedBy = "semspec.convention.approved_by"

	// ConventionConfidence is the extraction confidence score (0.0-1.0).
	ConventionConfidence = "semspec.convention.confidence"

	// ConventionStatus is the convention status.
	// Values: active, deprecated, superseded
	ConventionStatus = "semspec.convention.status"
)

// ConventionNamespace is the IRI namespace for convention terms.
const ConventionNamespace = "https://semspec.dev/ontology/convention/"

func init() {
	vocabulary.Register(ConventionName,
		vocabulary.WithDescription("Short name for the learned convention"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(ConventionNamespace+"name"))

	vocabulary.Register(ConventionPattern,
		vocabulary.WithDescription("Pattern description in natural language"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(ConventionNamespace+"pattern"))

	vocabulary.Register(ConventionAppliesTo,
		vocabulary.WithDescription("File patterns this convention applies to (glob)"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(ConventionNamespace+"appliesTo"))

	vocabulary.Register(ConventionSourceTask,
		vocabulary.WithDescription("Task ID where this convention was learned"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(ConventionNamespace+"sourceTask"))

	vocabulary.Register(ConventionApprovedBy,
		vocabulary.WithDescription("Reviewer model that extracted this convention"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(ConventionNamespace+"approvedBy"))

	vocabulary.Register(ConventionConfidence,
		vocabulary.WithDescription("Extraction confidence score (0.0-1.0)"),
		vocabulary.WithDataType("float"),
		vocabulary.WithIRI(ConventionNamespace+"confidence"))

	vocabulary.Register(ConventionStatus,
		vocabulary.WithDescription("Convention status: active, deprecated, superseded"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(ConventionNamespace+"status"))
}
