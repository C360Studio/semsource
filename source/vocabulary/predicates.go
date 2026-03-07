package source

import "github.com/c360studio/semstreams/vocabulary"

// Document metadata predicates for ingested documents.
// These predicates track document metadata extracted during ingestion.
const (
	// DocType identifies the source as a document.
	// Values: "document"
	DocType = "source.doc.type"

	// DocCategory classifies the document purpose.
	// Values: sop, spec, datasheet, reference, api
	DocCategory = "source.doc.category"

	// DocAppliesTo specifies file patterns this document applies to.
	// Format: glob patterns like "*.go", "auth/*", "**/*.ts"
	// For SOPs, this determines which files trigger SOP inclusion in reviewer context.
	DocAppliesTo = "source.doc.applies_to"

	// DocSeverity indicates violation severity for SOPs.
	// Values: error (blocks approval), warning (reviewer discretion), info (no enforcement)
	DocSeverity = "source.doc.severity"

	// DocSummary is a short LLM-extracted summary.
	// Used in context assembly when full document doesn't fit in budget.
	DocSummary = "source.doc.summary"

	// DocRequirements are extracted key rules/requirements.
	// Array of strings representing must-check items for reviewers.
	DocRequirements = "source.doc.requirements"

	// DocContent is the document text content.
	// Present on both parent entities (full body) and chunk entities (chunk text).
	DocContent = "source.doc.content"

	// DocSection is the section or heading name.
	// Identifies which part of the document this chunk represents.
	DocSection = "source.doc.section"

	// DocChunkIndex is the chunk sequence number (1-indexed).
	DocChunkIndex = "source.doc.chunk_index"

	// DocChunkCount is the total number of chunks in the parent document.
	DocChunkCount = "source.doc.chunk_count"

	// DocMimeType is the document MIME type.
	// Values: text/markdown, application/pdf, text/plain, etc.
	DocMimeType = "source.doc.mime_type"

	// DocFilePath is the original file path in .semspec/sources/docs/.
	DocFilePath = "source.doc.file_path"

	// DocFileHash is the content hash for staleness detection.
	DocFileHash = "source.doc.file_hash"

	// DocScope specifies when this document applies.
	// Values: plan (planning phase), code (implementation), all (both phases)
	// For SOPs, this determines whether the SOP is checked during plan approval,
	// code review, or both.
	DocScope = "source.doc.scope"

	// DocDomain is the semantic domain this document covers.
	// Values: auth, database, api, security, testing, logging, error-handling,
	//         performance, deployment, messaging, caching, etc.
	// Multiple values allowed (array). Used for domain-aware SOP matching
	// during code review - when touching auth code, find all auth-domain SOPs
	// regardless of file path patterns.
	DocDomain = "source.doc.domain"

	// DocRelatedDomains links to conceptually related domains.
	// Example: auth doc might relate to ["security", "session", "token"]
	// Used for pulling in cross-domain SOPs during review - when touching auth
	// code, also include security-domain SOPs.
	DocRelatedDomains = "source.doc.related_domains"

	// DocKeywords are LLM-extracted semantic keywords for fuzzy matching.
	// Example: ["token refresh", "expiration", "OAuth", "JWT"]
	// Enables semantic search when file patterns and domains don't produce matches.
	DocKeywords = "source.doc.keywords"
)

// Web source predicates for external web pages.
const (
	// WebType identifies the source as a web page.
	// Values: "web"
	WebType = "source.web.type"

	// WebURL is the web page URL.
	WebURL = "source.web.url"

	// WebContentType is the HTTP content type (e.g., text/html).
	WebContentType = "source.web.content_type"

	// WebTitle is the page title extracted from HTML.
	WebTitle = "source.web.title"

	// WebLastFetched is when the content was last fetched (RFC3339).
	WebLastFetched = "source.web.last_fetched"

	// WebETag is the HTTP ETag for staleness detection.
	WebETag = "source.web.etag"

	// WebContentHash is the SHA256 of fetched content.
	WebContentHash = "source.web.content_hash"

	// WebAutoRefresh indicates whether to auto-refresh for updates.
	WebAutoRefresh = "source.web.auto_refresh"

	// WebRefreshInterval is the auto-refresh interval (duration string like "1h").
	WebRefreshInterval = "source.web.refresh_interval"

	// WebChunkCount is the total number of chunks.
	WebChunkCount = "source.web.chunk_count"

	// WebContent is the chunk text content.
	// Only present on chunk entities, not parent entities.
	WebContent = "source.web.content"

	// WebSection is the section or heading name.
	// Identifies which part of the web page this chunk represents.
	WebSection = "source.web.section"

	// WebChunkIndex is the chunk sequence number (1-indexed).
	WebChunkIndex = "source.web.chunk_index"

	// WebDomain is the URL hostname for web sources.
	// Example: "docs.anthropic.com", "golang.org", "pkg.go.dev"
	// Used to group web sources by origin and prioritize authoritative sources.
	WebDomain = "source.web.domain"

	// WebCategory classifies the web page purpose.
	// Values: sop, spec, datasheet, reference, api
	WebCategory = "source.web.category"

	// WebAppliesTo specifies file patterns this web SOP applies to.
	// Format: glob patterns like "*.go", "auth/*", "**/*.ts"
	WebAppliesTo = "source.web.applies_to"

	// WebSeverity indicates violation severity for web SOPs.
	// Values: error (blocks approval), warning (reviewer discretion), info (no enforcement)
	WebSeverity = "source.web.severity"

	// WebScope specifies when this web source applies.
	// Values: plan (planning phase), code (implementation), all (both phases)
	WebScope = "source.web.scope"

	// WebSummary is a short LLM-extracted summary.
	// Used in context assembly when full content doesn't fit in budget.
	WebSummary = "source.web.summary"

	// WebRequirements are extracted key rules/requirements.
	// Array of strings representing must-check items for reviewers.
	WebRequirements = "source.web.requirements"

	// WebSemanticDomain is the semantic domain this web source covers.
	// Values: auth, database, api, security, testing, logging, error-handling,
	//         performance, deployment, messaging, caching, etc.
	// Multiple values allowed (array). Used for domain-aware SOP matching.
	WebSemanticDomain = "source.web.semantic_domain"

	// WebRelatedDomains links to conceptually related domains.
	// Example: auth doc might relate to ["security", "session", "token"]
	// Used for pulling in cross-domain SOPs during review.
	WebRelatedDomains = "source.web.related_domains"

	// WebKeywords are LLM-extracted semantic keywords for fuzzy matching.
	// Example: ["token refresh", "expiration", "OAuth", "JWT"]
	// Enables semantic search when file patterns and domains don't produce matches.
	WebKeywords = "source.web.keywords"

	// WebAnalysisSkipped indicates LLM analysis was skipped (timeout/error).
	// When true, the web source lacks semantic metadata and should be treated
	// as a basic reference without SOP capabilities.
	WebAnalysisSkipped = "source.web.analysis_skipped"
)

// Repository source predicates for external code sources.
const (
	// RepoType identifies the source as a repository.
	// Values: "repository"
	RepoType = "source.repo.type"

	// RepoURL is the git clone URL.
	RepoURL = "source.repo.url"

	// RepoBranch is the branch name to track.
	RepoBranch = "source.repo.branch"

	// RepoStatus is the repository indexing status.
	// Values: pending, indexing, ready, error, stale
	RepoStatus = "source.repo.status"

	// RepoLanguages are the programming languages detected.
	// Array of strings like ["go", "typescript", "python"].
	RepoLanguages = "source.repo.languages"

	// RepoEntityCount is the number of entities indexed from this repo.
	RepoEntityCount = "source.repo.entity_count"

	// RepoLastIndexed is the timestamp of last successful indexing (RFC3339).
	RepoLastIndexed = "source.repo.last_indexed"

	// RepoAutoPull indicates whether to auto-pull for updates.
	RepoAutoPull = "source.repo.auto_pull"

	// RepoPullInterval is the auto-pull interval (duration string like "1h").
	RepoPullInterval = "source.repo.pull_interval"

	// RepoLastCommit is the SHA of the last indexed commit.
	RepoLastCommit = "source.repo.last_commit"

	// RepoError is the error message if indexing failed.
	RepoError = "source.repo.error"
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

	// SourceName is the display name for the source.
	SourceName = "source.meta.name"

	// SourceStatus is the overall source status.
	// Values: pending, indexing, ready, error, stale
	SourceStatus = "source.meta.status"

	// SourceProject is the project entity ID for grouping related sources.
	// Format: c360.semspec.workflow.project.project.{project-slug}
	// Multiple sources (repos + docs) can belong to a project for coordinated context assembly.
	SourceProject = "source.meta.project"

	// SourceAddedBy is the user/agent who added this source.
	SourceAddedBy = "source.meta.added_by"

	// SourceAddedAt is the timestamp when the source was added (RFC3339).
	SourceAddedAt = "source.meta.added_at"

	// SourceError is the error message if source processing failed.
	SourceError = "source.meta.error"

	// SourceAuthority indicates this source is authoritative for its domain.
	// Used to prioritize official documentation over blog posts/examples.
	// For web sources, this may be inferred from the domain (e.g., golang.org
	// is authoritative for Go, docs.anthropic.com for Claude).
	SourceAuthority = "source.meta.authority"
)

func init() {
	registerStructurePredicates()
	registerDocPredicates()
	registerWebPredicates()
	registerRepoPredicates()
	registerSourcePredicates()
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

	vocabulary.Register(DocCategory,
		vocabulary.WithDescription("Document classification: sop, spec, datasheet, reference, api"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(DcType))

	vocabulary.Register(DocAppliesTo,
		vocabulary.WithDescription("File patterns this document applies to (glob patterns)"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"appliesTo"))

	vocabulary.Register(DocSeverity,
		vocabulary.WithDescription("Violation severity: error, warning, info"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"severity"))

	vocabulary.Register(DocSummary,
		vocabulary.WithDescription("Short extracted summary for context assembly"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(DcAbstract))

	vocabulary.Register(DocRequirements,
		vocabulary.WithDescription("Extracted key requirements for review validation"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"requirements"))

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

	vocabulary.Register(DocScope,
		vocabulary.WithDescription("Document scope: plan (planning phase), code (implementation), all (both)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"scope"))

	vocabulary.Register(DocDomain,
		vocabulary.WithDescription("Semantic domain(s) this document covers: auth, database, api, security, testing, etc."),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"domain"))

	vocabulary.Register(DocRelatedDomains,
		vocabulary.WithDescription("Conceptually related domains for cross-domain SOP matching"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"relatedDomains"))

	vocabulary.Register(DocKeywords,
		vocabulary.WithDescription("Extracted semantic keywords for fuzzy matching"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"keywords"))

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

	vocabulary.Register(WebTitle,
		vocabulary.WithDescription("Page title extracted from HTML"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(vocabulary.DcTitle))

	vocabulary.Register(WebLastFetched,
		vocabulary.WithDescription("Timestamp of last fetch (RFC3339)"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI(Namespace+"lastFetched"))

	vocabulary.Register(WebETag,
		vocabulary.WithDescription("HTTP ETag for staleness detection"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"etag"))

	vocabulary.Register(WebContentHash,
		vocabulary.WithDescription("SHA256 of fetched content"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"contentHash"))

	vocabulary.Register(WebAutoRefresh,
		vocabulary.WithDescription("Whether to auto-refresh for updates"),
		vocabulary.WithDataType("bool"),
		vocabulary.WithIRI(Namespace+"autoRefresh"))

	vocabulary.Register(WebRefreshInterval,
		vocabulary.WithDescription("Auto-refresh interval (duration string)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"refreshInterval"))

	vocabulary.Register(WebChunkCount,
		vocabulary.WithDescription("Total chunks in parent web source"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(Namespace+"webChunkCount"))

	vocabulary.Register(WebContent,
		vocabulary.WithDescription("Chunk text content for web source"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webContent"))

	vocabulary.Register(WebSection,
		vocabulary.WithDescription("Section or heading name for web chunk"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webSection"))

	vocabulary.Register(WebChunkIndex,
		vocabulary.WithDescription("Chunk sequence number (1-indexed) for web source"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(Namespace+"webChunkIndex"))

	vocabulary.Register(WebDomain,
		vocabulary.WithDescription("URL hostname for grouping web sources by origin"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webDomain"))

	vocabulary.Register(WebCategory,
		vocabulary.WithDescription("Web page classification: sop, spec, datasheet, reference, api"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webCategory"))

	vocabulary.Register(WebAppliesTo,
		vocabulary.WithDescription("File patterns this web SOP applies to (glob patterns)"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"webAppliesTo"))

	vocabulary.Register(WebSeverity,
		vocabulary.WithDescription("Violation severity for web SOPs: error, warning, info"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webSeverity"))

	vocabulary.Register(WebScope,
		vocabulary.WithDescription("Web source scope: plan (planning phase), code (implementation), all (both)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webScope"))

	vocabulary.Register(WebSummary,
		vocabulary.WithDescription("Short extracted summary for context assembly"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"webSummary"))

	vocabulary.Register(WebRequirements,
		vocabulary.WithDescription("Extracted key requirements for review validation"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"webRequirements"))

	vocabulary.Register(WebSemanticDomain,
		vocabulary.WithDescription("Semantic domain(s) this web source covers: auth, database, api, security, etc."),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"webSemanticDomain"))

	vocabulary.Register(WebRelatedDomains,
		vocabulary.WithDescription("Conceptually related domains for cross-domain SOP matching"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"webRelatedDomains"))

	vocabulary.Register(WebKeywords,
		vocabulary.WithDescription("Extracted semantic keywords for fuzzy matching"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"webKeywords"))

	vocabulary.Register(WebAnalysisSkipped,
		vocabulary.WithDescription("Whether LLM analysis was skipped (timeout/error)"),
		vocabulary.WithDataType("bool"),
		vocabulary.WithIRI(Namespace+"webAnalysisSkipped"))

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

	vocabulary.Register(RepoBranch,
		vocabulary.WithDescription("Branch name to track"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"branch"))

	vocabulary.Register(RepoStatus,
		vocabulary.WithDescription("Repository indexing status"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"repoStatus"))

	vocabulary.Register(RepoLanguages,
		vocabulary.WithDescription("Programming languages detected"),
		vocabulary.WithDataType("array"),
		vocabulary.WithIRI(Namespace+"languages"))

	vocabulary.Register(RepoEntityCount,
		vocabulary.WithDescription("Number of entities indexed"),
		vocabulary.WithDataType("int"),
		vocabulary.WithIRI(Namespace+"entityCount"))

	vocabulary.Register(RepoLastIndexed,
		vocabulary.WithDescription("Last successful indexing timestamp (RFC3339)"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI(Namespace+"lastIndexed"))

	vocabulary.Register(RepoAutoPull,
		vocabulary.WithDescription("Whether to auto-pull for updates"),
		vocabulary.WithDataType("bool"),
		vocabulary.WithIRI(Namespace+"autoPull"))

	vocabulary.Register(RepoPullInterval,
		vocabulary.WithDescription("Auto-pull interval (duration string)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"pullInterval"))

	vocabulary.Register(RepoLastCommit,
		vocabulary.WithDescription("SHA of last indexed commit"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"lastCommit"))

	vocabulary.Register(RepoError,
		vocabulary.WithDescription("Error message if indexing failed"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"repoError"))

}

func registerSourcePredicates() {
	// Register generic source predicates
	vocabulary.Register(SourceType,
		vocabulary.WithDescription("Source type discriminator: repository or document"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"sourceType"))

	vocabulary.Register(SourceName,
		vocabulary.WithDescription("Display name for the source"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(vocabulary.DcTitle))

	vocabulary.Register(SourceStatus,
		vocabulary.WithDescription("Overall source status"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"status"))

	vocabulary.Register(SourceProject,
		vocabulary.WithDescription("Project entity ID for grouping related sources"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"project"))

	vocabulary.Register(SourceAddedBy,
		vocabulary.WithDescription("User/agent who added this source"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(vocabulary.ProvWasAttributedTo))

	vocabulary.Register(SourceAddedAt,
		vocabulary.WithDescription("Timestamp when source was added (RFC3339)"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI(vocabulary.ProvGeneratedAtTime))

	vocabulary.Register(SourceError,
		vocabulary.WithDescription("Error message if source processing failed"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"error"))

	vocabulary.Register(SourceAuthority,
		vocabulary.WithDescription("Whether source is authoritative for its domain"),
		vocabulary.WithDataType("bool"),
		vocabulary.WithIRI(Namespace+"authority"))
}
