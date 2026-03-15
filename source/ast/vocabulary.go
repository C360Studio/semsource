package ast

import "github.com/c360studio/semstreams/vocabulary"

// CodeNamespace is the base IRI prefix for code vocabulary terms.
const CodeNamespace = "https://semspec.dev/ontology/code/"

// AgenticNamespace is the base IRI prefix for agentic capability terms.
const AgenticNamespace = "https://semspec.dev/ontology/agentic/"

func init() {
	registerArtifactPredicates()
	registerStructurePredicates()
	registerDependencyPredicates()
	registerRelationshipPredicates()
	registerMetricPredicates()
	registerDocPredicates()
	registerCapabilityPredicates()
	registerDublinCorePredicates()
}

func registerArtifactPredicates() {
	vocabulary.Register(CodePath,
		vocabulary.WithDescription("File path relative to repo root"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"path"))

	vocabulary.Register(CodeHash,
		vocabulary.WithDescription("Content hash for change detection"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"hash"))

	vocabulary.Register(CodeLanguage,
		vocabulary.WithDescription("Programming language: go, typescript, javascript, java, python, svelte"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"language"))

	vocabulary.Register(CodeFramework,
		vocabulary.WithDescription("UI framework: svelte, react, vue (optional)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"framework"))

	vocabulary.Register(CodePackage,
		vocabulary.WithDescription("Package or module name"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"package"))

	vocabulary.Register(CodeType,
		vocabulary.WithDescription("Entity type: file, package, function, method, struct, interface, const, var, type, class, enum, component"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"type"))

	vocabulary.Register(CodeVisibility,
		vocabulary.WithDescription("Symbol visibility: public (exported) or private (unexported)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"visibility"))
}

func registerStructurePredicates() {
	vocabulary.Register(CodeContains,
		vocabulary.WithDescription("Parent contains child (file → functions)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"contains"))

	vocabulary.Register(CodeBelongsTo,
		vocabulary.WithDescription("Child belongs to parent (function → file)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI("http://purl.obolibrary.org/obo/BFO_0000050"))
}

func registerDependencyPredicates() {
	vocabulary.Register(CodeImports,
		vocabulary.WithDescription("Import dependency path"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"imports"))

	vocabulary.Register(CodeExports,
		vocabulary.WithDescription("Exported symbol name"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"exports"))
}

func registerRelationshipPredicates() {
	vocabulary.Register(CodeImplements,
		vocabulary.WithDescription("Struct implements interface"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"implements"))

	vocabulary.Register(CodeExtends,
		vocabulary.WithDescription("Class extends superclass (TypeScript/JavaScript)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"extends"))

	vocabulary.Register(CodeEmbeds,
		vocabulary.WithDescription("Struct embeds type (Go embedding)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"embeds"))

	vocabulary.Register(CodeCalls,
		vocabulary.WithDescription("Function calls another function"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"calls"))

	vocabulary.Register(CodeReferences,
		vocabulary.WithDescription("References another code entity (type reference)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(CodeNamespace+"references"))

	vocabulary.Register(CodeReturns,
		vocabulary.WithDescription("Function return type"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"returns"))

	vocabulary.Register(CodeReceiver,
		vocabulary.WithDescription("Method receiver type"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"receiver"))

	vocabulary.Register(CodeParameter,
		vocabulary.WithDescription("Function parameter type"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"parameter"))
}

func registerMetricPredicates() {
	vocabulary.Register(CodeLines,
		vocabulary.WithDescription("Source line count"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(CodeNamespace+"lines"))

	vocabulary.Register(CodeStartLine,
		vocabulary.WithDescription("Starting line number in source file"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(CodeNamespace+"startLine"))

	vocabulary.Register(CodeEndLine,
		vocabulary.WithDescription("Ending line number in source file"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(CodeNamespace+"endLine"))

	vocabulary.Register(CodeComplexity,
		vocabulary.WithDescription("Cyclomatic complexity score"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(CodeNamespace+"complexity"))
}

func registerDocPredicates() {
	vocabulary.Register(CodeDocComment,
		vocabulary.WithDescription("Documentation comment text"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(CodeNamespace+"docComment"))
}

func registerCapabilityPredicates() {
	vocabulary.Register(CodeCapabilityName,
		vocabulary.WithDescription("Agentic capability identifier"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(AgenticNamespace+"name"))

	vocabulary.Register(CodeCapabilityDescription,
		vocabulary.WithDescription("Human-readable capability description"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(AgenticNamespace+"description"))

	vocabulary.Register(CodeCapabilityTools,
		vocabulary.WithDescription("Tools this code provides or uses"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(AgenticNamespace+"tools"))

	vocabulary.Register(CodeCapabilityInputs,
		vocabulary.WithDescription("Expected input types for the capability"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(AgenticNamespace+"inputs"))

	vocabulary.Register(CodeCapabilityOutputs,
		vocabulary.WithDescription("Expected output types from the capability"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(AgenticNamespace+"outputs"))
}

func registerDublinCorePredicates() {
	vocabulary.Register(DcTitle,
		vocabulary.WithDescription("Human-readable entity name"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI("http://purl.org/dc/terms/title"))

	vocabulary.Register(DcCreated,
		vocabulary.WithDescription("Creation timestamp"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI("http://purl.org/dc/terms/created"))

	vocabulary.Register(DcModified,
		vocabulary.WithDescription("Last modification timestamp"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI("http://purl.org/dc/terms/modified"))
}
