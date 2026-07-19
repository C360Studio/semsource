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

	// Version scoping (ADR-0008 #2). Neither is recoverable from the entity ID:
	// both the source identity and the version are folded into the length-capped
	// `system` segment (entityid.ScopedSystemSlug), so correspondence across
	// versions must key on these triples. Emitted together, only for versioned
	// sources — a version-less source has no sibling to correspond to, so
	// version-less entities carry neither and stay byte-identical to prior output.
	CodeProject = "code.artifact.project" // version-independent source identity (raw project)
	CodeVersion = "code.artifact.version" // registered version/ref qualifier (e.g. "v1.9.0")

	// Classification predicates
	CodeType       = "code.artifact.type"       // file|package|function|method|struct|interface|const|var|type
	CodeVisibility = "code.artifact.visibility" // public|private (exported vs unexported in Go)
	// CodeExported is a PRESENCE-only ranking marker: stamped (object "true")
	// ONLY on exported/public symbols. Ranking salience keys on predicate
	// presence (fusionvocab.PredicateSalience → the vocabulary Weight), and it
	// can't condition on a predicate's VALUE — so boosting public over private
	// needs a distinct predicate, not the visibility value. See task #38.
	CodeExported = "code.artifact.exported"
	// CodeTest is the PRESENCE-only demotion complement of CodeExported:
	// stamped (object "true") ONLY on entities from test code (IsTestPath),
	// registered with negative salience so production symbols outrank their
	// tests in NL retrieval. Tests stay indexed and structurally queryable —
	// demoted, never hidden (audit 2026-07-19, search-ranking-and-reach).
	CodeTest = "code.artifact.test"

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

	// Version lineage (ADR-0008 #2). A distinct category from code.relationship.*
	// (which is code-semantic) — lineage relates the SAME logical symbol across
	// versions of one source. Supersession is directional (newer → older) and
	// additive; the inverse is emitted for query convenience. code.lineage.change
	// records whether the body changed across the corresponding pair.
	CodeSupersedes    = "code.lineage.supersedes"    // newer entity → older entity it supersedes
	CodeSupersededBy  = "code.lineage.superseded-by" // older entity → newer entity that supersedes it
	CodeLineageChange = "code.lineage.change"        // "changed" | "unchanged" (body-hash comparison)

	// Metrics
	CodeLines      = "code.metric.lines"      // line count
	CodeStartLine  = "code.metric.start-line" // starting line number
	CodeEndLine    = "code.metric.end-line"   // ending line number
	CodeComplexity = "code.metric.complexity" // cyclomatic complexity (future)

	// Verbatim body handle (ADR-062 hydration contract). The producer offloads
	// an entity's pre-sliced source to a storage.Store at ingest and stamps these
	// two triples; the fusion code lens reads them back into a StorageReference so
	// the engine's BodyResolver can Get the bytes location-independently — no
	// worktree read. Absent when no body was offloaded (e.g. container entities).
	CodeBodyStore = "code.body.store" // storage component instance name (e.g. "objectstore")
	CodeBodyKey   = "code.body.key"   // storage key: the entity's body blob (content hash)

	// Documentation
	CodeDocComment = "code.doc.comment"   // documentation comment
	CodeSignature  = "code.doc.signature" // rendered signature, e.g. "submit(item: Job): Promise<string>"

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
	TypeRepo      CodeEntityType = "repo"
	TypeFolder    CodeEntityType = "folder"
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
