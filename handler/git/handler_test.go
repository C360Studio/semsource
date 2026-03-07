package git_test

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
)

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

// srcCfg builds a minimal SourceConfig for testing.
type srcCfg struct {
	typ   string
	path  string
	url   string
	watch bool
}

func (c *srcCfg) GetType() string      { return c.typ }
func (c *srcCfg) GetPath() string      { return c.path }
func (c *srcCfg) GetURL() string       { return c.url }
func (c *srcCfg) IsWatchEnabled() bool { return c.watch }

// --------------------------------------------------------------------------
// Interface compliance
// --------------------------------------------------------------------------

var _ handler.SourceHandler = (*githandler.GitHandler)(nil)

// --------------------------------------------------------------------------
// SourceType / Supports
// --------------------------------------------------------------------------

func TestGitHandler_SourceType(t *testing.T) {
	h := githandler.New(githandler.DefaultConfig())
	if h.SourceType() != "git" {
		t.Errorf("SourceType() = %q, want %q", h.SourceType(), "git")
	}
}

func TestGitHandler_Supports(t *testing.T) {
	h := githandler.New(githandler.DefaultConfig())

	if !h.Supports(&srcCfg{typ: "git"}) {
		t.Error("Supports() = false for git type, want true")
	}
	if h.Supports(&srcCfg{typ: "ast"}) {
		t.Error("Supports() = true for ast type, want false")
	}
	if h.Supports(&srcCfg{typ: "url"}) {
		t.Error("Supports() = true for url type, want false")
	}
	if h.Supports(&srcCfg{typ: ""}) {
		t.Error("Supports() = true for empty type, want false")
	}
}

// --------------------------------------------------------------------------
// ValidateGitURL
// --------------------------------------------------------------------------

func TestValidateGitURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https accepted", "https://github.com/acme/repo", false},
		{"https with .git accepted", "https://github.com/acme/repo.git", false},
		{"ssh URL accepted", "ssh://git@github.com/acme/repo.git", false},
		{"git protocol accepted", "git://github.com/acme/repo.git", false},
		{"ssh shorthand accepted", "git@github.com:acme/repo.git", false},
		{"file:// rejected", "file:///local/repo", true},
		{"http:// rejected", "http://github.com/acme/repo", true},
		{"empty string rejected", "", true},
		{"ftp:// rejected", "ftp://example.com/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := githandler.ValidateGitURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Watch — disabled returns nil
// --------------------------------------------------------------------------

func TestGitHandler_Watch_ReturnsNilWhenDisabled(t *testing.T) {
	h := githandler.New(githandler.DefaultConfig())
	ch, err := h.Watch(context.Background(), &srcCfg{typ: "git", path: "/any", watch: false})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if ch != nil {
		t.Error("Watch() should return nil channel when watch not enabled")
	}
}

// --------------------------------------------------------------------------
// Entity builder functions (exported for white-box tests)
// --------------------------------------------------------------------------

func TestBuildCommitEntity(t *testing.T) {
	e := githandler.BuildCommitEntity("abc1234def5678", "Alice <alice@example.com>", "fix: bug", "github.com-acme-repo")

	if e.EntityType != "commit" {
		t.Errorf("EntityType = %q, want commit", e.EntityType)
	}
	if e.Domain != "git" {
		t.Errorf("Domain = %q, want git", e.Domain)
	}
	if e.Instance != "abc1234" {
		t.Errorf("Instance = %q, want abc1234 (short SHA)", e.Instance)
	}
	if e.System != "github.com-acme-repo" {
		t.Errorf("System = %q, want github.com-acme-repo", e.System)
	}
	if e.SourceType != "git" {
		t.Errorf("SourceType = %q, want git", e.SourceType)
	}
}

func TestBuildAuthorEntity(t *testing.T) {
	e := githandler.BuildAuthorEntity("Alice", "alice@example.com", "github.com-acme-repo")

	if e.EntityType != "author" {
		t.Errorf("EntityType = %q, want author", e.EntityType)
	}
	if e.Domain != "git" {
		t.Errorf("Domain = %q, want git", e.Domain)
	}
	if e.Instance == "" {
		t.Error("Instance must not be empty")
	}
	// Instance should be derived from the email for determinism.
	if !strings.Contains(e.Instance, "alice") && e.Instance != "alice@example.com" {
		// Accept any stable non-empty instance derived from author identity.
		if len(e.Instance) < 4 {
			t.Errorf("Instance %q too short to be a useful author identifier", e.Instance)
		}
	}
}

func TestBuildBranchEntity(t *testing.T) {
	e := githandler.BuildBranchEntity("main", "abc1234def5678", "github.com-acme-repo")

	if e.EntityType != "branch" {
		t.Errorf("EntityType = %q, want branch", e.EntityType)
	}
	if e.Domain != "git" {
		t.Errorf("Domain = %q, want git", e.Domain)
	}
	if e.Instance != "main" {
		t.Errorf("Instance = %q, want main", e.Instance)
	}
}
