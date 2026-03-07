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
	Properties map[string]any

	// Triples are pre-formed RDF-style triples the handler has already resolved.
	// Optional — handlers may emit triples directly instead of relying on the normalizer.
	Triples []message.Triple

	// Edges are raw directed edges to be resolved by the normalizer into graph edges.
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
}
