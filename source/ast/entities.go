package ast

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semstreams/message"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// CodeEntity represents a code artifact extracted from AST parsing.
// It provides methods to convert to graph triples for storage.
type CodeEntity struct {
	// ID is the 6-part entity identifier
	// Format: {org}.semsource.{language}.{system}.{type}.{instance}
	ID string

	// Type classifies the code entity
	Type CodeEntityType

	// Name is the identifier (function name, type name, etc.)
	Name string

	// Path is the file path relative to repo root
	Path string

	// Package is the Go package name (or module path for TypeScript)
	Package string

	// Language indicates the source language (go, typescript, javascript)
	Language string

	// Framework indicates the UI framework (svelte, react, vue) - optional
	Framework string

	// Visibility indicates if exported
	Visibility Visibility

	// Location in source
	StartLine int
	EndLine   int

	// Content hash for change detection
	Hash string

	// Documentation comment
	DocComment string

	// Signature is a rendered, language-flavored signature string
	// (e.g. "submit(item: Job): Promise<string>") suitable for embedding
	// alongside the doc comment in semantic search. Aligns with SCIP's
	// SymbolInformation.signature_documentation concept without changing
	// entity identity.
	Signature string

	// Capability metadata (extracted from doc comments)
	Capability *CapabilityInfo

	// Relationships to other entities (entity IDs)
	ContainedBy string   // parent entity ID
	Contains    []string // child entity IDs
	Imports     []string // import paths
	Implements  []string // interface entity IDs
	Extends     []string // superclass entity IDs (TypeScript/JavaScript)
	Embeds      []string // embedded type entity IDs
	Calls       []string // called function entity IDs
	References  []string // type reference entity IDs
	Returns     []string // return type entity IDs
	Receiver    string   // receiver type entity ID (for methods)
	Parameters  []string // parameter type entity IDs

	// Timestamps
	IndexedAt time.Time
}

// CapabilityInfo holds capability metadata extracted from doc comments
type CapabilityInfo struct {
	// Name is the capability identifier (from @capability tag)
	Name string
	// Description is a human-readable description
	Description string
	// Tools lists tools this code provides or requires
	Tools []string
	// Inputs lists expected input types (from @requires tag)
	Inputs []string
	// Outputs lists expected output types (from @produces tag)
	Outputs []string
}

// NewCodeEntity creates a new code entity with the given parameters.
// The language and project parameters are used to construct the 6-part entity ID.
// language should be the domain name (e.g. "golang", "typescript", "java", "python", "svelte").
func NewCodeEntity(org, language, project string, entityType CodeEntityType, name, path string) *CodeEntity {
	return NewScopedCodeEntity(org, language, project, entityType, nil, name, path)
}

// NewScopedCodeEntity is like NewCodeEntity but inserts enclosing-scope
// segments (class names, receiver types) between the path slug and the entity
// name. This disambiguates siblings sharing a name across different scopes —
// e.g. two methods named "Get" on different receivers in the same Go file,
// or methods named "submit" in two classes in one Java file would otherwise
// collide. Pass an empty scope (nil or []) for top-level entities; in that
// case the resulting ID matches NewCodeEntity exactly.
//
// The project argument is run through entityid.SystemSlug before being used as
// the system segment. This is idempotent on already-clean slugs (such as those
// pre-computed by the ast-source component), so no double-transform harm occurs,
// and it eliminates the raw-passthrough bug for callers that supply a canonical
// module path or module-cache path (e.g. "semstreams@v1.9.0") directly.
func NewScopedCodeEntity(org, language, project string, entityType CodeEntityType, scope []string, name, path string) *CodeEntity {
	instance := BuildScopedInstanceID(path, scope, name, entityType)
	systemSlug := entityid.SystemSlug(project)

	return &CodeEntity{
		ID:         entityid.Build(org, entityid.PlatformSemsource, language, systemSlug, string(entityType), instance),
		Type:       entityType,
		Name:       name,
		Path:       path,
		Language:   language,
		Visibility: determineVisibility(name),
		IndexedAt:  time.Now(),
	}
}

// SanitizePathSegment converts a path to a NATS-safe entity ID segment by
// replacing slashes and dots with hyphens and stripping leading hyphens.
// Exported for reuse in hierarchy construction.
func SanitizePathSegment(path string) string {
	s := strings.ReplaceAll(path, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.TrimPrefix(s, "-")
	return s
}

// BuildInstanceID creates a unique instance identifier from path and name.
// Exported for use by language-specific parser packages.
func BuildInstanceID(path, name string, entityType CodeEntityType) string {
	return BuildScopedInstanceID(path, nil, name, entityType)
}

// BuildScopedInstanceID is like BuildInstanceID but inserts enclosing-scope
// segments (e.g. class names, receiver types) between the path slug and the
// entity name. Each scope segment is run through SanitizePathSegment so
// generic markers like "*", "[T]", or "Foo.Inner" produce single, valid
// graph-ingest segments. Empty scope segments are silently skipped.
func BuildScopedInstanceID(path string, scope []string, name string, entityType CodeEntityType) string {
	parts := []string{SanitizePathSegment(path)}
	for _, s := range scope {
		clean := SanitizePathSegment(s)
		if clean != "" {
			parts = append(parts, clean)
		}
	}
	if name != "" && entityType != TypeFile && entityType != TypePackage {
		parts = append(parts, name)
	}
	return strings.Join(parts, "-")
}

// determineVisibility checks if a Go identifier is exported
func determineVisibility(name string) Visibility {
	if name == "" {
		return VisibilityPrivate
	}
	r := []rune(name)
	if len(r) > 0 && unicode.IsUpper(r[0]) {
		return VisibilityPublic
	}
	return VisibilityPrivate
}

// ComputeHash computes a SHA256 hash of the given content
func ComputeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:8]) // First 8 bytes for brevity
}

// Triples converts the CodeEntity to a slice of message.Triple for graph storage.
// All semantic properties are stored as triples using vocabulary predicates.
func (e *CodeEntity) Triples() []message.Triple {
	triples := make([]message.Triple, 0, 20)
	triples = append(triples, e.identityTriples()...)
	triples = append(triples, e.capabilityTriples()...)
	triples = append(triples, e.relationshipTriples()...)
	triples = append(triples,
		message.Triple{Subject: e.ID, Predicate: DcCreated, Object: e.IndexedAt.Format(time.RFC3339)})
	return triples
}

// IndexingProfile classifies code entities for SemStreams semantic indexing.
func (e *CodeEntity) IndexingProfile() string {
	switch e.Type {
	case TypeFunction, TypeMethod, TypeStruct, TypeInterface, TypeClass,
		TypeEnum, TypeComponent, TypeConst, TypeVar, TypeType:
		return semvocab.IndexingProfileContent
	default:
		return semvocab.IndexingProfileControl
	}
}

// identityTriples returns triples for identity, classification, and location predicates.
func (e *CodeEntity) identityTriples() []message.Triple {
	triples := []message.Triple{
		{Subject: e.ID, Predicate: CodeType, Object: string(e.Type)},
		{Subject: e.ID, Predicate: DcTitle, Object: e.Name},
	}
	if e.Path != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodePath, Object: e.Path})
	}
	if e.Package != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodePackage, Object: e.Package})
	}
	if e.Hash != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeHash, Object: e.Hash})
	}
	lang := e.Language
	if lang == "" {
		lang = "golang" // default for backward compatibility
	}
	triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeLanguage, Object: lang})
	if e.Framework != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeFramework, Object: e.Framework})
	}
	triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeVisibility, Object: string(e.Visibility)})
	// Presence-only ranking marker on exported symbols (task #38): floats the
	// public API surface above internals. Emitted only when public so its mere
	// presence carries the salience — visibility as a value can't be weighted.
	if e.Visibility == VisibilityPublic {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeExported, Object: "true"})
	}
	if e.StartLine > 0 {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeStartLine, Object: e.StartLine})
	}
	if e.EndLine > 0 {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeEndLine, Object: e.EndLine})
	}
	if e.StartLine > 0 && e.EndLine > 0 {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeLines, Object: e.EndLine - e.StartLine + 1})
	}
	if e.DocComment != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeDocComment, Object: e.DocComment})
	}
	if e.Signature != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeSignature, Object: e.Signature})
	}
	return triples
}

// capabilityTriples returns triples for agentic capability metadata.
func (e *CodeEntity) capabilityTriples() []message.Triple {
	if e.Capability == nil {
		return nil
	}
	var triples []message.Triple
	if e.Capability.Name != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCapabilityName, Object: e.Capability.Name})
	}
	if e.Capability.Description != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCapabilityDescription, Object: e.Capability.Description})
	}
	for _, tool := range e.Capability.Tools {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCapabilityTools, Object: tool})
	}
	for _, input := range e.Capability.Inputs {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCapabilityInputs, Object: input})
	}
	for _, output := range e.Capability.Outputs {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCapabilityOutputs, Object: output})
	}
	return triples
}

// relationshipTriples returns triples for structural and semantic relationships.
func (e *CodeEntity) relationshipTriples() []message.Triple {
	var triples []message.Triple
	if e.ContainedBy != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeBelongsTo, Object: e.ContainedBy})
	}
	for _, id := range e.Contains {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeContains, Object: id})
	}
	for _, path := range e.Imports {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeImports, Object: path})
	}
	for _, id := range e.Implements {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeImplements, Object: id})
	}
	for _, id := range e.Extends {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeExtends, Object: id})
	}
	for _, id := range e.Embeds {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeEmbeds, Object: id})
	}
	for _, id := range e.Calls {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeCalls, Object: id})
	}
	for _, id := range e.References {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeReferences, Object: id})
	}
	for _, id := range e.Returns {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeReturns, Object: id})
	}
	if e.Receiver != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeReceiver, Object: e.Receiver})
	}
	for _, id := range e.Parameters {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeParameter, Object: id})
	}
	return triples
}

// EntityState converts the CodeEntity to a graph.EntityState for storage.
func (e *CodeEntity) EntityState() *EntityState {
	return &EntityState{
		ID:              e.ID,
		Triples:         e.Triples(),
		UpdatedAt:       e.IndexedAt,
		IndexingProfile: e.IndexingProfile(),
	}
}

// EntityState mirrors graph.EntityState for local use without importing the full graph package.
// This allows the AST package to prepare data for graph storage.
type EntityState struct {
	ID              string
	Triples         []message.Triple
	UpdatedAt       time.Time
	IndexingProfile string
}

// ParseResult holds the results of parsing a Go file
type ParseResult struct {
	// FileEntity is the entity representing the file itself
	FileEntity *CodeEntity

	// Entities are all entities extracted from the file
	Entities []*CodeEntity

	// Imports are the import paths found in the file
	Imports []string

	// Package is the package name
	Package string

	// Path is the file path
	Path string

	// Hash is the content hash
	Hash string
}

// AllTriples returns all triples from all entities in the parse result
func (r *ParseResult) AllTriples() []message.Triple {
	var triples []message.Triple
	for _, entity := range r.Entities {
		triples = append(triples, entity.Triples()...)
	}
	return triples
}

// AllEntityStates returns all entity states from the parse result
func (r *ParseResult) AllEntityStates() []*EntityState {
	states := make([]*EntityState, 0, len(r.Entities))
	for _, entity := range r.Entities {
		states = append(states, entity.EntityState())
	}
	return states
}
