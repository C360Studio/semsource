// Package ast provides Go AST parsing and code entity extraction for the knowledge graph.
package ast

// Vocabulary predicates for code entities.
// Uses three-part dotted notation: domain.category.property
const (
	// Identity predicates
	CodePath      = "code.artifact.path"      // file path relative to repo root
	CodeHash      = "code.artifact.hash"      // content hash for change detection
	CodeLanguage  = "code.artifact.language"  // go, typescript, etc.
	CodeFramework = "code.artifact.framework" // svelte, react, vue (optional)
	CodePackage   = "code.artifact.package"   // package name

	// Classification predicates
	CodeType       = "code.artifact.type"       // file|package|function|method|struct|interface|const|var|type
	CodeVisibility = "code.artifact.visibility" // public|private (exported vs unexported in Go)

	// Structure relationships
	CodeContains  = "code.structure.contains" // parent → child (file → functions)
	CodeBelongsTo = "code.structure.belongs"  // child → parent (function → file)

	// Dependency relationships
	CodeImports = "code.dependency.imports" // → other code entity (import path)
	CodeExports = "code.dependency.exports" // exported symbols

	// Semantic relationships
	CodeImplements = "code.relationship.implements" // struct → interface
	CodeExtends    = "code.relationship.extends"    // class → superclass (TS/JS)
	CodeEmbeds     = "code.relationship.embeds"     // struct → embedded type
	CodeCalls      = "code.relationship.calls"      // function → called function
	CodeReferences = "code.relationship.references" // → any code entity (type reference)
	CodeReturns    = "code.relationship.returns"    // function → return type
	CodeReceiver   = "code.relationship.receiver"   // method → receiver type
	CodeParameter  = "code.relationship.parameter"  // function → parameter type

	// Metrics
	CodeLines      = "code.metric.lines"      // line count
	CodeStartLine  = "code.metric.start_line" // starting line number
	CodeEndLine    = "code.metric.end_line"   // ending line number
	CodeComplexity = "code.metric.complexity" // cyclomatic complexity (future)

	// Documentation
	CodeDocComment = "code.doc.comment" // documentation comment

	// Capability predicates (for agentic vocabulary integration)
	CodeCapabilityName        = "agentic.capability.name"        // capability identifier
	CodeCapabilityDescription = "agentic.capability.description" // human-readable description
	CodeCapabilityTools       = "agentic.capability.tools"       // tools this code provides/uses
	CodeCapabilityInputs      = "agentic.capability.inputs"      // expected input types
	CodeCapabilityOutputs     = "agentic.capability.outputs"     // expected output types

	// Standard metadata (Dublin Core aligned)
	DcTitle    = "dc.terms.title"    // human-readable name
	DcCreated  = "dc.terms.created"  // creation timestamp
	DcModified = "dc.terms.modified" // modification timestamp
)

// CodeEntityType represents the type of code entity
type CodeEntityType string

// TypeFile and related constants enumerate the kinds of code entity
// that can be extracted during AST parsing.
const (
	TypeFile      CodeEntityType = "file"
	TypePackage   CodeEntityType = "package"
	TypeFunction  CodeEntityType = "function"
	TypeMethod    CodeEntityType = "method"
	TypeStruct    CodeEntityType = "struct"
	TypeInterface CodeEntityType = "interface"
	TypeClass     CodeEntityType = "class"     // TypeScript/JavaScript class
	TypeEnum      CodeEntityType = "enum"      // TypeScript enum
	TypeComponent CodeEntityType = "component" // Svelte/React component
	TypeConst     CodeEntityType = "const"
	TypeVar       CodeEntityType = "var"
	TypeType      CodeEntityType = "type" // type alias or definition
)

// Visibility indicates whether a symbol is exported
type Visibility string

// VisibilityPublic and VisibilityPrivate indicate whether a symbol is exported.
const (
	VisibilityPublic  Visibility = "public"  // exported (uppercase first letter)
	VisibilityPrivate Visibility = "private" // unexported (lowercase first letter)
)
