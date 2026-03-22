package handler

import (
	"time"

	"github.com/c360studio/semstreams/message"
)

// SourceType constants for supported handler kinds.
const (
	SourceTypeGit    = "git"
	SourceTypeAST    = "ast"
	SourceTypeDoc    = "doc"
	SourceTypeConfig = "config"
	SourceTypeURL    = "url"
	SourceTypeImage  = "image"
	SourceTypeVideo  = "video"
	SourceTypeAudio  = "audio"
)

// Domain constants for entity domain classification.
const (
	DomainGolang = "golang"
	DomainGit    = "git"
	DomainWeb    = "web"
	DomainConfig = "config"
	DomainMedia  = "media"
)

// RawEntity is a pre-normalization entity extracted by a source handler.
// The normalizer consumes RawEntity values and produces GraphEntity values
// with fully qualified 6-part deterministic IDs.
type RawEntity struct {
	// SourceType identifies the handler that produced this entity (git, ast, doc, config, url).
	SourceType string

	// Domain is the semantic domain for this entity (golang, git, web, config).
	Domain string

	// System is the canonical source system with dots/slashes replaced by dashes
	// (e.g. "github.com-acme-gcs").
	System string

	// EntityType is the entity kind (function, commit, doc, file, dependency, etc.).
	EntityType string

	// Instance is the symbol name, SHA, content hash, or other intrinsic identifier.
	Instance string

	// Properties holds source-specific metadata. Values must be JSON-serializable.
	// The normalizer converts these to triples with Subject=entityID and
	// Predicate="{domain}.{entityType}.{key}".
	Properties map[string]any

	// Edges are raw directed relationships to be resolved by the normalizer
	// into relationship triples (where Object is the target entity ID).
	Edges []RawEdge
}

// RawEdge is a directed relationship between two entities before ID normalization.
// FromHint and ToHint are instance-level hints (symbol names, SHAs, paths) that
// the normalizer uses to resolve full entity IDs.
type RawEdge struct {
	// FromHint is the Instance-level hint for the source entity.
	FromHint string

	// ToHint is the Instance-level hint for the target entity.
	ToHint string

	// EdgeType describes the relationship (calls, imports, depends, documents, etc.).
	EdgeType string

	// ToType overrides the target entity type when constructing the ToID.
	// When empty, the source entity's EntityType is used (same-type edge).
	ToType string

	// Weight is an optional edge strength. Zero means unweighted.
	Weight float64
}

// ChangeOperation describes the kind of filesystem or source change.
type ChangeOperation string

const (
	// OperationCreate signals a new entity was created (new file, new commit, etc.).
	OperationCreate ChangeOperation = "create"

	// OperationModify signals an existing entity was modified.
	OperationModify ChangeOperation = "modify"

	// OperationDelete signals an entity was removed. Entities may be empty — the
	// normalizer uses Path to issue RETRACT events.
	OperationDelete ChangeOperation = "delete"
)

// EntityState is a handler-level entity ready for direct graph publication.
// Handlers that build typed entity structs (bypassing the normalizer) populate
// this type so processors can convert directly to graph.EntityPayload.
type EntityState struct {
	// ID is the fully-qualified 6-part entity identifier.
	ID string

	// Triples are the semantic property and relationship triples for this entity.
	Triples []message.Triple

	// UpdatedAt is when the entity was last observed / indexed.
	UpdatedAt time.Time

	// StorageRef points to where the full content is stored in ObjectStore.
	// When set, large content (e.g. document body) lives in the store rather
	// than inline in a triple. Nil means all content is in Triples.
	StorageRef *message.StorageReference
}

// ChangeEvent is emitted by a handler's Watch channel when a source change is detected.
type ChangeEvent struct {
	// Path is the filesystem path or URL that changed.
	Path string

	// Operation describes what happened at the path.
	Operation ChangeOperation

	// Timestamp is when the change was detected.
	Timestamp time.Time

	// Entities are the raw entities extracted from the changed source.
	// May be empty for delete operations, where the path itself is the signal.
	Entities []RawEntity

	// EntityStates holds pre-built typed entities for handlers that bypass the
	// normalizer (e.g. git-source). When populated, processors should prefer
	// EntityStates over Entities to avoid a redundant normalizer pass.
	EntityStates []*EntityState
}
