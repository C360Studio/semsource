package git_test

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
	"github.com/c360studio/semstreams/message"
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

func (c *srcCfg) GetType() string             { return c.typ }
func (c *srcCfg) GetPath() string             { return c.path }
func (c *srcCfg) GetPaths() []string          { return nil }
func (c *srcCfg) GetURL() string              { return c.url }
func (c *srcCfg) GetBranch() string           { return "" }
func (c *srcCfg) IsWatchEnabled() bool        { return c.watch }
func (c *srcCfg) GetKeyframeMode() string     { return "" }
func (c *srcCfg) GetKeyframeInterval() string { return "" }
func (c *srcCfg) GetSceneThreshold() float64  { return 0 }

// --------------------------------------------------------------------------
// Interface compliance
// --------------------------------------------------------------------------

var _ handler.SourceHandler = (*githandler.Handler)(nil)

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

// --------------------------------------------------------------------------
// Typed entity struct tests (normalizer-free path)
// --------------------------------------------------------------------------

// findPredicate returns the first Triple with the given predicate from ts, or nil.
func findPredicate(ts []message.Triple, predicate string) *message.Triple {
	for i := range ts {
		if ts[i].Predicate == predicate {
			return &ts[i]
		}
	}
	return nil
}

func TestCommitEntity_Triples(t *testing.T) {
	h := githandler.New(githandler.Config{Org: "acme"})
	states, err := h.IngestEntityStates(context.Background(), &srcCfg{
		typ:  "git",
		path: ".", // current repo — always has at least one commit in CI
	}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error = %v", err)
	}
	if len(states) == 0 {
		t.Fatal("IngestEntityStates() returned no states")
	}

	// Find the first commit state by looking for the sha predicate.
	var commitState *handler.EntityState
	for _, s := range states {
		if findPredicate(s.Triples, "source.git.commit.sha") != nil {
			commitState = s
			break
		}
	}
	if commitState == nil {
		t.Fatal("no commit entity state found in results")
	}

	// ID must be a non-empty 6-part dot-delimited string starting with org.
	parts := strings.Split(commitState.ID, ".")
	if len(parts) < 6 {
		t.Errorf("commit entity ID %q has fewer than 6 parts", commitState.ID)
	}
	if parts[0] != "acme" {
		t.Errorf("commit entity ID org = %q, want acme", parts[0])
	}
	if parts[2] != "git" {
		t.Errorf("commit entity ID domain = %q, want git", parts[2])
	}
	if parts[4] != "commit" {
		t.Errorf("commit entity ID type = %q, want commit", parts[4])
	}

	// Required predicates must be present.
	for _, pred := range []string{
		"source.git.commit.sha",
		"source.git.commit.short_sha",
		"source.git.commit.author",
		"source.git.commit.subject",
	} {
		if findPredicate(commitState.Triples, pred) == nil {
			t.Errorf("commit entity missing predicate %q", pred)
		}
	}
}

func TestAuthorEntity_Triples(t *testing.T) {
	h := githandler.New(githandler.Config{Org: "acme"})
	states, err := h.IngestEntityStates(context.Background(), &srcCfg{
		typ:  "git",
		path: ".",
	}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error = %v", err)
	}

	var authorState *handler.EntityState
	for _, s := range states {
		if findPredicate(s.Triples, "source.git.author.email") != nil {
			authorState = s
			break
		}
	}
	if authorState == nil {
		t.Fatal("no author entity state found in results")
	}

	parts := strings.Split(authorState.ID, ".")
	if len(parts) < 6 {
		t.Errorf("author entity ID %q has fewer than 6 parts", authorState.ID)
	}
	if parts[4] != "author" {
		t.Errorf("author entity ID type = %q, want author", parts[4])
	}

	for _, pred := range []string{"source.git.author.name", "source.git.author.email"} {
		if findPredicate(authorState.Triples, pred) == nil {
			t.Errorf("author entity missing predicate %q", pred)
		}
	}
}

func TestBranchEntity_Triples(t *testing.T) {
	h := githandler.New(githandler.Config{Org: "acme"})
	states, err := h.IngestEntityStates(context.Background(), &srcCfg{
		typ:  "git",
		path: ".",
	}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error = %v", err)
	}

	var branchState *handler.EntityState
	for _, s := range states {
		if findPredicate(s.Triples, "source.git.branch.name") != nil {
			branchState = s
			break
		}
	}
	if branchState == nil {
		t.Fatal("no branch entity state found in results")
	}

	parts := strings.Split(branchState.ID, ".")
	if len(parts) < 6 {
		t.Errorf("branch entity ID %q has fewer than 6 parts", branchState.ID)
	}
	if parts[4] != "branch" {
		t.Errorf("branch entity ID type = %q, want branch", parts[4])
	}

	for _, pred := range []string{"source.git.branch.name", "source.git.branch.head_sha"} {
		if findPredicate(branchState.Triples, pred) == nil {
			t.Errorf("branch entity missing predicate %q", pred)
		}
	}
}

func TestIngestEntityStates_NoOrg_ReturnsError(t *testing.T) {
	h := githandler.New(githandler.DefaultConfig())
	// No path or URL — should fail at repo resolution, not org.
	_, err := h.IngestEntityStates(context.Background(), &srcCfg{typ: "git"}, "acme")
	if err == nil {
		t.Error("IngestEntityStates() with no path/url should return error")
	}
}

func TestIngestEntityStates_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	h := githandler.New(githandler.DefaultConfig())
	_, err := h.IngestEntityStates(ctx, &srcCfg{typ: "git", path: "."}, "acme")
	if err == nil {
		t.Error("IngestEntityStates() with cancelled context should return error")
	}
}
