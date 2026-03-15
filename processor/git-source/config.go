// Package gitsource provides the git-source processor component for semsource.
// It ingests git repositories (local or remote), walks commit history,
// and publishes commit/author/branch entity payloads to the NATS graph ingestion stream.
package gitsource

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the git-source processor component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// RepoPath is the local filesystem path to the git repository.
	// Either RepoPath or RepoURL must be provided.
	RepoPath string `json:"repo_path" schema:"type:string,description:Local path to git repository (use instead of repo_url for local repos),category:basic"`

	// RepoURL is the remote URL to clone (https/git/ssh).
	// Either RepoPath or RepoURL must be provided.
	// When set, WorkspaceDir is required.
	RepoURL string `json:"repo_url" schema:"type:string,description:Remote git repository URL to clone (https/git/ssh),category:basic"`

	// Org is the organization namespace used in entity ID construction.
	Org string `json:"org" schema:"type:string,description:Organization namespace for entity IDs (e.g. acme),category:basic,required:true"`

	// Branch is the branch to track. Used only for display/metadata; HEAD
	// is determined by the actual state of the repository.
	Branch string `json:"branch" schema:"type:string,description:Branch name to track,category:basic,default:main"`

	// PollInterval controls how often the component polls for new commits.
	// Accepts Go duration strings (e.g. "60s", "5m"). Default: "60s".
	PollInterval string `json:"poll_interval" schema:"type:string,description:Polling interval for new commits (e.g. 60s 5m),category:advanced,default:60s"`

	// WorkspaceDir is the base directory used when auto-cloning remote
	// repositories (required when RepoURL is set and RepoPath is empty).
	WorkspaceDir string `json:"workspace_dir" schema:"type:string,description:Base directory for cloning remote repositories,category:advanced"`

	// GitToken is a personal access token for authenticating HTTPS clones
	// of private repositories. SSH URLs ignore this field.
	GitToken string `json:"git_token" schema:"type:string,description:Personal access token for private HTTPS repositories,category:advanced"`

	// WatchEnabled controls whether the component polls for new commits.
	// Defaults to true.
	WatchEnabled *bool `json:"watch_enabled,omitempty" schema:"type:bool,description:Enable polling for new commits,category:advanced,default:true"`

	// MaxCommits caps the number of commits ingested per ingest call.
	// 0 means unlimited (default).
	MaxCommits int `json:"max_commits" schema:"type:int,description:Maximum commits to ingest per run (0 for unlimited),category:advanced,default:0"`

	// BranchSlug is set by multi-branch expansion to scope entity IDs per branch.
	// When non-empty, the system slug in entity IDs includes the branch qualifier.
	BranchSlug string `json:"branch_slug,omitempty" schema:"type:string,description:Branch slug for multi-branch entity ID scoping,category:advanced"`

	// StreamName is the JetStream stream name for publishing entities.
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.RepoPath == "" && c.RepoURL == "" {
		return fmt.Errorf("either repo_path or repo_url is required")
	}
	if c.RepoURL != "" && c.RepoPath == "" && c.WorkspaceDir == "" {
		return fmt.Errorf("workspace_dir is required when using repo_url without repo_path")
	}
	if c.Org == "" {
		return fmt.Errorf("org is required")
	}
	if c.PollInterval != "" {
		d, err := time.ParseDuration(c.PollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll_interval format: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("poll_interval must be positive")
		}
	}
	if c.MaxCommits < 0 {
		return fmt.Errorf("max_commits must be >= 0")
	}
	return nil
}

func ptrBool(v bool) *bool { return &v }

// DefaultConfig returns the default configuration for the git-source processor.
func DefaultConfig() Config {
	outputDefs := []component.PortDefinition{
		{
			Name:        "graph.ingest",
			Type:        "jetstream",
			Subject:     "graph.ingest.entity",
			StreamName:  "GRAPH",
			Required:    true,
			Description: "Entity state updates for graph ingestion",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Outputs: outputDefs,
		},
		Branch:       "main",
		PollInterval: "60s",
		WatchEnabled: ptrBool(true),
		MaxCommits:   0,
		StreamName:   "GRAPH",
	}
}
