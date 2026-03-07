package ast

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/c360studio/semstreams/message"
)

// CodeEntity represents a code artifact extracted from AST parsing.
// It provides methods to convert to graph triples for storage.
type CodeEntity struct {
	// ID is the 6-part entity identifier
	// Format: {org}.semspec.code.{type}.{project}.{instance}
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
// The project parameter is used to construct the 6-part entity ID.
func NewCodeEntity(org, project string, entityType CodeEntityType, name, path string) *CodeEntity {
	// Build instance identifier from path and name
	instance := BuildInstanceID(path, name, entityType)

	return &CodeEntity{
		ID:         fmt.Sprintf("%s.semspec.code.%s.%s.%s", org, entityType, project, instance),
		Type:       entityType,
		Name:       name,
		Path:       path,
		Visibility: determineVisibility(name),
		IndexedAt:  time.Now(),
	}
}

// BuildInstanceID creates a unique instance identifier from path and name.
// Exported for use by language-specific parser packages.
func BuildInstanceID(path, name string, entityType CodeEntityType) string {
	// Sanitize for use in entity ID (replace invalid characters)
	sanitized := strings.ReplaceAll(path, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	sanitized = strings.TrimPrefix(sanitized, "-")

	if name != "" && entityType != TypeFile && entityType != TypePackage {
		// For functions, types, etc: include name
		return fmt.Sprintf("%s-%s", sanitized, name)
	}
	return sanitized
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
		lang = "go" // default for backward compatibility
	}
	triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeLanguage, Object: lang})
	if e.Framework != "" {
		triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeFramework, Object: e.Framework})
	}
	triples = append(triples, message.Triple{Subject: e.ID, Predicate: CodeVisibility, Object: string(e.Visibility)})
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
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// EntityState mirrors graph.EntityState for local use without importing the full graph package.
// This allows the AST package to prepare data for graph storage.
type EntityState struct {
	ID        string
	Triples   []message.Triple
	UpdatedAt time.Time
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
