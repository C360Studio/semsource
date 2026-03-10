// Package source provides types and parsers for document ingestion.
package source

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
