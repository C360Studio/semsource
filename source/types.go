// Package source provides types and parsers for document ingestion.
package source

import (
	"time"

	vocab "github.com/c360studio/semsource/source/vocabulary"
)

// Source represents a knowledge source (document or repository).
type Source struct {
	// ID is the unique identifier for this source.
	ID string `json:"id"`

	// Name is the display name.
	Name string `json:"name"`

	// Type discriminates between document and repository sources.
	Type vocab.TypeValue `json:"type"`

	// Status tracks the processing state.
	Status vocab.StatusType `json:"status"`

	// ProjectID is the entity ID of the parent project.
	// Format: c360.semspec.workflow.project.project.{project-slug}
	// Required - defaults to the "default" project if not specified.
	ProjectID string `json:"project_id"`

	// AddedBy identifies who added this source.
	AddedBy string `json:"added_by,omitempty"`

	// AddedAt is when the source was added.
	AddedAt time.Time `json:"added_at"`

	// Error holds any processing error message.
	Error string `json:"error,omitempty"`
}

// DocumentSource represents an ingested document with LLM-extracted metadata.
type DocumentSource struct {
	Source

	// Filename is the original filename.
	Filename string `json:"filename"`

	// MimeType is the document MIME type.
	MimeType string `json:"mime_type"`

	// FilePath is the path relative to .semspec/sources/docs/.
	FilePath string `json:"file_path"`

	// FileHash is the content hash for staleness detection.
	FileHash string `json:"file_hash,omitempty"`

	// Category classifies the document (sop, spec, datasheet, reference, api).
	Category vocab.DocCategoryType `json:"category"`

	// AppliesTo lists file patterns this document applies to.
	AppliesTo []string `json:"applies_to,omitempty"`

	// Severity indicates violation severity for SOPs.
	Severity vocab.DocSeverityType `json:"severity,omitempty"`

	// Summary is the LLM-extracted summary.
	Summary string `json:"summary,omitempty"`

	// Requirements are extracted key rules.
	Requirements []string `json:"requirements,omitempty"`

	// Domain is the semantic domain(s) this document covers.
	Domain []string `json:"domain,omitempty"`

	// RelatedDomains are conceptually related domains.
	RelatedDomains []string `json:"related_domains,omitempty"`

	// Keywords are extracted semantic terms for fuzzy matching.
	Keywords []string `json:"keywords,omitempty"`

	// ChunkCount is the total number of chunks.
	ChunkCount int `json:"chunk_count,omitempty"`
}

// RepositorySource represents an external git repository.
type RepositorySource struct {
	Source

	// URL is the git clone URL.
	URL string `json:"url"`

	// Branch is the branch name to track.
	Branch string `json:"branch"`

	// Languages are the detected programming languages.
	Languages []string `json:"languages,omitempty"`

	// EntityCount is the number of indexed entities.
	EntityCount int `json:"entity_count,omitempty"`

	// LastIndexed is when the repo was last indexed.
	LastIndexed *time.Time `json:"last_indexed,omitempty"`

	// LastCommit is the SHA of the last indexed commit.
	LastCommit string `json:"last_commit,omitempty"`

	// AutoPull indicates whether to auto-pull for updates.
	AutoPull bool `json:"auto_pull,omitempty"`

	// PullInterval is the auto-pull interval.
	PullInterval string `json:"pull_interval,omitempty"`
}

// WebSource represents a web URL source for documentation and reference pages.
type WebSource struct {
	Source

	// URL is the web page URL.
	URL string `json:"url"`

	// ContentType is the HTTP content type (e.g., text/html).
	ContentType string `json:"content_type,omitempty"`

	// Title is the page title extracted from HTML.
	Title string `json:"title,omitempty"`

	// LastFetched is when the content was last fetched.
	LastFetched *time.Time `json:"last_fetched,omitempty"`

	// ETag is the HTTP ETag for staleness detection.
	ETag string `json:"etag,omitempty"`

	// ContentHash is the SHA256 of fetched content.
	ContentHash string `json:"content_hash,omitempty"`

	// AutoRefresh indicates whether to auto-refresh for updates.
	AutoRefresh bool `json:"auto_refresh,omitempty"`

	// RefreshInterval is the auto-refresh interval (e.g., "1h", "24h").
	RefreshInterval string `json:"refresh_interval,omitempty"`

	// ChunkCount is the number of indexed chunks.
	ChunkCount int `json:"chunk_count,omitempty"`
}

// Chunk represents a section of a document for context assembly.
type Chunk struct {
	// ParentID is the ID of the parent document.
	ParentID string `json:"parent_id"`

	// Index is the chunk sequence number (0-indexed internally, 1-indexed for display).
	Index int `json:"index"`

	// Section is the heading or section name.
	Section string `json:"section,omitempty"`

	// Content is the chunk text.
	Content string `json:"content"`

	// TokenCount is the estimated token count.
	TokenCount int `json:"token_count"`
}

// Document represents a parsed document with its content and metadata.
type Document struct {
	// ID is the document identifier (typically derived from file path).
	ID string `json:"id"`

	// Filename is the original filename.
	Filename string `json:"filename"`

	// Content is the raw document content.
	Content string `json:"content"`

	// Frontmatter contains parsed YAML frontmatter if present.
	Frontmatter map[string]any `json:"frontmatter,omitempty"`

	// Body is the content without frontmatter.
	Body string `json:"body"`
}

// HasFrontmatter returns true if the document has parsed frontmatter.
func (d *Document) HasFrontmatter() bool {
	return len(d.Frontmatter) > 0
}

// FrontmatterAsAnalysis converts frontmatter to AnalysisResult if valid.
// Returns nil if frontmatter doesn't contain analysis fields.
func (d *Document) FrontmatterAsAnalysis() *AnalysisResult {
	if !d.HasFrontmatter() {
		return nil
	}

	result := &AnalysisResult{}

	// Extract category
	if cat, ok := d.Frontmatter["category"].(string); ok {
		result.Category = cat
	}

	// Extract applies_to
	if appliesTo, ok := d.Frontmatter["applies_to"].([]any); ok {
		for _, v := range appliesTo {
			if s, ok := v.(string); ok {
				result.AppliesTo = append(result.AppliesTo, s)
			}
		}
	} else if appliesTo, ok := d.Frontmatter["applies_to"].([]string); ok {
		result.AppliesTo = appliesTo
	}

	// Extract severity
	if sev, ok := d.Frontmatter["severity"].(string); ok {
		result.Severity = sev
	}

	// Extract scope (optional override for LLM inference)
	if scope, ok := d.Frontmatter["scope"].(string); ok {
		result.Scope = scope
	}

	// Extract summary
	if sum, ok := d.Frontmatter["summary"].(string); ok {
		result.Summary = sum
	}

	// Extract requirements
	if reqs, ok := d.Frontmatter["requirements"].([]any); ok {
		for _, v := range reqs {
			if s, ok := v.(string); ok {
				result.Requirements = append(result.Requirements, s)
			}
		}
	} else if reqs, ok := d.Frontmatter["requirements"].([]string); ok {
		result.Requirements = reqs
	}

	// Extract domain
	if domains, ok := d.Frontmatter["domain"].([]any); ok {
		for _, v := range domains {
			if s, ok := v.(string); ok {
				result.Domain = append(result.Domain, s)
			}
		}
	} else if domains, ok := d.Frontmatter["domain"].([]string); ok {
		result.Domain = domains
	}

	// Extract related_domains
	if related, ok := d.Frontmatter["related_domains"].([]any); ok {
		for _, v := range related {
			if s, ok := v.(string); ok {
				result.RelatedDomains = append(result.RelatedDomains, s)
			}
		}
	} else if related, ok := d.Frontmatter["related_domains"].([]string); ok {
		result.RelatedDomains = related
	}

	// Extract keywords
	if kw, ok := d.Frontmatter["keywords"].([]any); ok {
		for _, v := range kw {
			if s, ok := v.(string); ok {
				result.Keywords = append(result.Keywords, s)
			}
		}
	} else if kw, ok := d.Frontmatter["keywords"].([]string); ok {
		result.Keywords = kw
	}

	// Return nil if no useful fields were extracted
	if result.Category == "" && len(result.AppliesTo) == 0 {
		return nil
	}

	return result
}

// AnalysisResult contains LLM-extracted document metadata.
type AnalysisResult struct {
	// Category classifies the document type.
	Category string `json:"category"`

	// AppliesTo lists file patterns this document applies to.
	AppliesTo []string `json:"applies_to"`

	// Severity indicates violation severity for SOPs.
	Severity string `json:"severity,omitempty"`

	// Scope specifies when the document applies: plan, code, or all.
	// Inferred from content or explicitly set via frontmatter.
	Scope string `json:"scope,omitempty"`

	// Summary is a brief description.
	Summary string `json:"summary,omitempty"`

	// Requirements are extracted key rules.
	Requirements []string `json:"requirements,omitempty"`

	// Domain is the semantic domain(s) this document covers.
	// Used for domain-aware SOP matching during code review.
	Domain []string `json:"domain,omitempty"`

	// RelatedDomains are conceptually related domains.
	// Used for pulling in cross-domain SOPs during review.
	RelatedDomains []string `json:"related_domains,omitempty"`

	// Keywords are extracted semantic terms for fuzzy matching.
	Keywords []string `json:"keywords,omitempty"`
}

// IsValid checks if the analysis result has required fields.
func (a *AnalysisResult) IsValid() bool {
	return a != nil && a.Category != ""
}

// IngestRequest is the payload for document ingestion requests.
type IngestRequest struct {
	// Path is the file path to ingest (relative to sources_dir or absolute).
	Path string `json:"path"`

	// MimeType is optional; if not provided, it will be inferred from extension.
	MimeType string `json:"mime_type,omitempty"`

	// ProjectID is the entity ID of the target project.
	// Format: c360.semspec.workflow.project.project.{project-slug}
	// Defaults to "default" project if not specified.
	ProjectID string `json:"project_id,omitempty"`

	// AddedBy is the user/agent who triggered the ingestion.
	AddedBy string `json:"added_by,omitempty"`
}

// AddRepositoryRequest is the payload for adding a repository source.
type AddRepositoryRequest struct {
	// URL is the git clone URL.
	URL string `json:"url"`

	// Branch is the branch name to track (optional, defaults to default branch).
	Branch string `json:"branch,omitempty"`

	// ProjectID is the entity ID of the target project.
	// Format: c360.semspec.workflow.project.project.{project-slug}
	// Defaults to "default" project if not specified.
	ProjectID string `json:"project_id,omitempty"`

	// AutoPull indicates whether to automatically pull for updates.
	AutoPull bool `json:"auto_pull,omitempty"`

	// PullInterval is the interval for auto-pulling (e.g., "1h", "30m").
	PullInterval string `json:"pull_interval,omitempty"`
}

// UpdateRepositoryRequest is the payload for updating repository settings.
type UpdateRepositoryRequest struct {
	// AutoPull updates the auto-pull setting.
	AutoPull *bool `json:"auto_pull,omitempty"`

	// PullInterval updates the pull interval.
	PullInterval *string `json:"pull_interval,omitempty"`

	// ProjectID updates the project entity ID.
	ProjectID *string `json:"project_id,omitempty"`
}

// AddWebSourceRequest is the payload for adding a web source.
type AddWebSourceRequest struct {
	// URL is the web page URL (must be HTTPS).
	URL string `json:"url"`

	// ProjectID is the entity ID of the target project.
	// Format: c360.semspec.workflow.project.project.{project-slug}
	// Defaults to "default" project if not specified.
	ProjectID string `json:"project_id,omitempty"`

	// AutoRefresh indicates whether to automatically refresh for updates.
	AutoRefresh bool `json:"auto_refresh,omitempty"`

	// RefreshInterval is the interval for auto-refreshing (e.g., "1h", "24h").
	RefreshInterval string `json:"refresh_interval,omitempty"`
}

// UpdateWebSourceRequest is the payload for updating web source settings.
type UpdateWebSourceRequest struct {
	// AutoRefresh updates the auto-refresh setting.
	AutoRefresh *bool `json:"auto_refresh,omitempty"`

	// RefreshInterval updates the refresh interval.
	RefreshInterval *string `json:"refresh_interval,omitempty"`

	// ProjectID updates the project entity ID.
	ProjectID *string `json:"project_id,omitempty"`
}

// WebSourceResponse is the JSON response for web source operations.
type WebSourceResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

// RefreshResponse is the JSON response for web source refresh operations.
type RefreshResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash,omitempty"`
	Changed     bool   `json:"changed"`
	Message     string `json:"message,omitempty"`
}

// RepositoryResponse is the JSON response for repository operations.
type RepositoryResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// PullResponse is the JSON response for repository pull operations.
type PullResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	LastCommit string `json:"last_commit,omitempty"`
	Message    string `json:"message,omitempty"`
}

// CategoryType returns the category as a vocabulary enum.
func (a *AnalysisResult) CategoryType() vocab.DocCategoryType {
	switch a.Category {
	case "sop":
		return vocab.DocCategorySOP
	case "spec":
		return vocab.DocCategorySpec
	case "datasheet":
		return vocab.DocCategoryDatasheet
	case "reference":
		return vocab.DocCategoryReference
	case "api":
		return vocab.DocCategoryAPI
	default:
		return vocab.DocCategoryReference
	}
}

// SeverityType returns the severity as a vocabulary enum.
func (a *AnalysisResult) SeverityType() vocab.DocSeverityType {
	switch a.Severity {
	case "error":
		return vocab.DocSeverityError
	case "warning":
		return vocab.DocSeverityWarning
	case "info":
		return vocab.DocSeverityInfo
	default:
		return vocab.DocSeverityInfo
	}
}

// ScopeType returns the scope as a vocabulary enum.
// Defaults to DocScopeCode if not specified (backward compatible).
func (a *AnalysisResult) ScopeType() vocab.DocScopeType {
	switch a.Scope {
	case "plan":
		return vocab.DocScopePlan
	case "code":
		return vocab.DocScopeCode
	case "all":
		return vocab.DocScopeAll
	default:
		return vocab.DocScopeCode
	}
}
