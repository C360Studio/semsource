package source

// Namespace is the base IRI prefix for source vocabulary terms.
const Namespace = "https://semspec.dev/ontology/source/"

// EntityNamespace is the base IRI for source entity instances.
const EntityNamespace = "https://semspec.dev/entity/source/"

// Standard ontology IRI constants for mappings.
const (
	// DcAbstract is the Dublin Core abstract property.
	DcAbstract = "http://purl.org/dc/terms/abstract"

	// DcFormat is the Dublin Core format property.
	DcFormat = "http://purl.org/dc/terms/format"

	// DcType is the Dublin Core type property.
	DcType = "http://purl.org/dc/terms/type"
)

// Class IRIs define the types of source entities.
const (
	// ClassDocument represents an ingested document source.
	// Extends: bfo:GenericallyDependentContinuant, cco:InformationContentEntity, prov:Entity
	ClassDocument = Namespace + "Document"

	// ClassDocumentChunk represents a chunk of a document.
	// Extends: ClassDocument
	ClassDocumentChunk = Namespace + "DocumentChunk"

	// ClassRepository represents an external repository source.
	// Extends: bfo:IndependentContinuant, cco:InformationBearingArtifact
	ClassRepository = Namespace + "Repository"

	// ClassSOP represents a Standard Operating Procedure document.
	// Extends: ClassDocument, cco:DirectiveInformationContentEntity
	ClassSOP = Namespace + "SOP"

	// ClassSpecDoc represents a technical specification document.
	// Extends: ClassDocument, cco:DescriptiveInformationContentEntity
	ClassSpecDoc = Namespace + "SpecificationDocument"

	// ClassDatasheet represents a datasheet document.
	// Extends: ClassDocument, cco:DescriptiveInformationContentEntity
	ClassDatasheet = Namespace + "Datasheet"

	// ClassWebSource represents a web URL source.
	// Extends: bfo:GenericallyDependentContinuant, cco:InformationContentEntity, prov:Entity
	ClassWebSource = Namespace + "WebSource"

	// ClassWebChunk represents a chunk of a web source.
	// Extends: ClassWebSource
	ClassWebChunk = Namespace + "WebChunk"
)

// Object Property IRIs define relationships between source entities.
const (
	// PropHasChunk links a document to its chunks.
	// Domain: ClassDocument, Range: ClassDocumentChunk
	PropHasChunk = Namespace + "hasChunk"

	// PropChunkOf links a chunk to its parent document.
	// Domain: ClassDocumentChunk, Range: ClassDocument
	// Note: Uses code.structure.belongs predicate, mapped to BFO part_of
	PropChunkOf = Namespace + "chunkOf"

	// PropAppliesTo links a document to code patterns it covers.
	// Domain: ClassDocument, Range: xsd:string (glob patterns)
	PropAppliesTo = Namespace + "appliesTo"

	// PropReferencesRule links an SOP to constitution rules.
	// Domain: ClassSOP, Range: semspec:ConstitutionRule
	PropReferencesRule = Namespace + "referencesRule"

	// PropDerivedFrom links extracted metadata to its source document.
	// Domain: any, Range: ClassDocument
	// Note: Uses prov:wasDerivedFrom
	PropDerivedFrom = Namespace + "derivedFrom"
)

// Data Property IRIs define literal-valued attributes.
const (
	// PropCategory is the document category (sop, spec, datasheet, reference).
	PropCategory = Namespace + "category"

	// PropSeverity is the SOP severity level (error, warning, info).
	PropSeverity = Namespace + "severity"

	// PropSection is the section/heading name for a chunk.
	PropSection = Namespace + "section"

	// PropChunkIndex is the chunk sequence number.
	PropChunkIndex = Namespace + "chunkIndex"

	// PropChunkCount is the total chunks in a document.
	PropChunkCount = Namespace + "chunkCount"

	// PropRepoURL is the git clone URL for a repository.
	PropRepoURL = Namespace + "repoURL"

	// PropBranch is the branch name for a repository.
	PropBranch = Namespace + "branch"

	// PropLastCommit is the last indexed commit SHA.
	PropLastCommit = Namespace + "lastCommit"

	// PropWebURL is the web page URL for a web source.
	PropWebURL = Namespace + "webURL"

	// PropWebTitle is the page title extracted from HTML.
	PropWebTitle = Namespace + "webTitle"

	// PropWebContentType is the HTTP content type.
	PropWebContentType = Namespace + "webContentType"

	// PropWebETag is the HTTP ETag for staleness detection.
	PropWebETag = Namespace + "webETag"

	// PropWebContentHash is the SHA256 of fetched content.
	PropWebContentHash = Namespace + "webContentHash"

	// PropWebLastFetched is the timestamp of last fetch.
	PropWebLastFetched = Namespace + "webLastFetched"

	// PropWebAutoRefresh indicates whether to auto-refresh.
	PropWebAutoRefresh = Namespace + "webAutoRefresh"

	// PropWebRefreshInterval is the auto-refresh interval.
	PropWebRefreshInterval = Namespace + "webRefreshInterval"
)
