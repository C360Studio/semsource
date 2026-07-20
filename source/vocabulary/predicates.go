package source

import "github.com/c360studio/semstreams/vocabulary"

// DcTitle is the Dublin Core title predicate — the canonical human name of an
// entity, shared across domains (code uses the same IRI). Documents emit it so a
// consumer reads a title from one predicate regardless of source type.
const DcTitle = "dc.terms.title"

// Document metadata predicates for ingested documents.
// These predicates track document metadata extracted during ingestion.
const (
	// DocType identifies the source as a document.
	// Values: "document"
	DocType = "source.doc.type"

	// DocContent is the document text content.
	// Present on both parent entities (full body) and chunk entities (chunk text).
	DocContent = "source.doc.content"

	// DocSection is the section or heading name.
	// Identifies which part of the document this chunk represents.
	DocSection = "source.doc.section"

	// DocChunkIndex is the passage's 0-indexed position in its parent document.
	// Zero-indexed so retraction is a plain comparison: a passage whose index is
	// at or above the parent's DocChunkCount no longer exists in the file.
	DocChunkIndex = "source.doc.chunk-index"

	// DocChunkCount is the total number of chunks in the parent document.
	DocChunkCount = "source.doc.chunk-count"

	// DocMimeType is the document MIME type.
	// Values: text/markdown, application/pdf, text/plain, etc.
	DocMimeType = "source.doc.mime-type"

	// DocFilePath is the original file path in .semspec/sources/docs/.
	DocFilePath = "source.doc.file-path"

	// DocFileHash is the content hash for staleness detection.
	DocFileHash = "source.doc.file-hash"

	// DocBodyStore and DocBodyKey are the verbatim body handle (ADR-062 hydration
	// contract), the doc analogue of ast.CodeBodyStore/CodeBodyKey: a doc producer
	// offloads the passage to a storage.Store and stamps these so the fusion docs
	// lens returns a StorageReference instead of inline content. Absent until the
	// doc body producer lands (tracked follow-up); the lens then yields no body.
	DocBodyStore = "source.doc.body-store" // storage component instance name
	DocBodyKey   = "source.doc.body-key"   // storage key: the passage blob
)

// Web source predicates for external web pages.
const (
	// WebType identifies the source as a web page.
	// Values: "web"
	WebType = "source.web.type"

	// WebURL is the web page URL.
	WebURL = "source.web.url"

	// WebContentType is the HTTP content type (e.g., text/html).
	WebContentType = "source.web.content-type"

	// WebETag is the HTTP ETag for staleness detection.
	WebETag = "source.web.etag"

	// WebContentHash is the SHA256 of fetched content.
	WebContentHash = "source.web.content-hash"

	// WebDomain is the URL hostname for web sources.
	// Example: "docs.anthropic.com", "golang.org", "pkg.go.dev"
	// Used to group web sources by origin and prioritize authoritative sources.
	WebDomain = "source.web.domain"
)

// Repository source predicates for external code sources.
const (
	// RepoType identifies the source as a repository.
	// Values: "repository"
	RepoType = "source.repo.type"

	// RepoURL is the git clone URL.
	RepoURL = "source.repo.url"
)

// Structure predicates for entity relationships.
const (
	// CodeBelongs links a child entity to its parent.
	// Used for document chunks → parent document relationships.
	// Also used for code entities → containing module relationships.
	CodeBelongs = "code.structure.belongs"
)

// Generic source predicates applicable to all source types.
const (
	// SourceType is the source type discriminator.
	// Values: "repository", "document", "web"
	SourceType = "source.meta.type"

	// SourceStatus is the overall source status.
	// Values: pending, indexing, ready, error, stale
	SourceStatus = "source.meta.status"

	// SourceError is the error message if source processing failed.
	SourceError = "source.meta.error"
)

func init() {
	registerStructurePredicates()
	registerDocPredicates()
	registerWebPredicates()
	registerRepoPredicates()
	registerSourcePredicates()
	registerMediaPredicates()
	registerConfigPredicates()
}

func registerStructurePredicates() {
	vocabulary.Register(CodeBelongs,
		vocabulary.WithDescription("Links child entity to parent (chunk to document, code to module)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI("http://purl.obolibrary.org/obo/BFO_0000050")) // BFO part_of

}

func registerDocPredicates() {
	vocabulary.Register(DocType,
		vocabulary.WithDescription("Source type identifier (document)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"docType"))

	vocabulary.Register(DocContent,
		vocabulary.WithDescription("Chunk text content"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"content"))

	vocabulary.Register(DocSection,
		vocabulary.WithDescription("Section or heading name for chunk"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"section"))

	vocabulary.Register(DocChunkIndex,
		vocabulary.WithDescription("Chunk sequence number (1-indexed)"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(Namespace+"chunkIndex"))

	vocabulary.Register(DocChunkCount,
		vocabulary.WithDescription("Total chunks in parent document"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(Namespace+"chunkCount"))

	vocabulary.Register(DocMimeType,
		vocabulary.WithDescription("Document MIME type"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(DcFormat))

	vocabulary.Register(DocFilePath,
		vocabulary.WithDescription("Original file path in sources directory"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"filePath"))

	vocabulary.Register(DocFileHash,
		vocabulary.WithDescription("Content hash for staleness detection"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"fileHash"))

	vocabulary.Register(DocBodyStore,
		vocabulary.WithDescription("Storage component instance containing the verbatim document body"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"bodyStore"))

	vocabulary.Register(DocBodyKey,
		vocabulary.WithDescription("Storage key for the verbatim document body"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"bodyKey"))

}

func registerWebPredicates() {
	// Register web source predicates
	vocabulary.Register(WebType,
		vocabulary.WithDescription("Source type identifier (web)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webType"))

	vocabulary.Register(WebURL,
		vocabulary.WithDescription("Web page URL"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webURL"))

	vocabulary.Register(WebContentType,
		vocabulary.WithDescription("HTTP content type (e.g., text/html)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(DcFormat))

	vocabulary.Register(WebETag,
		vocabulary.WithDescription("HTTP ETag for staleness detection"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"etag"))

	vocabulary.Register(WebContentHash,
		vocabulary.WithDescription("SHA256 of fetched content"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"contentHash"))

	vocabulary.Register(WebDomain,
		vocabulary.WithDescription("URL hostname for grouping web sources by origin"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webDomain"))

}

func registerRepoPredicates() {
	// Register repository source predicates
	vocabulary.Register(RepoType,
		vocabulary.WithDescription("Source type identifier (repository)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"repoType"))

	vocabulary.Register(RepoURL,
		vocabulary.WithDescription("Git clone URL"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"repoURL"))

}

func registerSourcePredicates() {
	// Register generic source predicates
	vocabulary.Register(SourceType,
		vocabulary.WithDescription("Source type discriminator: repository or document"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"sourceType"))

	vocabulary.Register(SourceStatus,
		vocabulary.WithDescription("Overall source status"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"status"))

	vocabulary.Register(SourceError,
		vocabulary.WithDescription("Error message if source processing failed"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"error"))
}
