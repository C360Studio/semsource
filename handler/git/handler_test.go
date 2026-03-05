package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
)

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

// initTempRepo creates a temporary git repository with one commit and returns
// its absolute path.
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial commit")

	return dir
}

// addCommit writes a file and creates a commit in the given repo.
func addCommit(t *testing.T, repoDir, filename, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, filename), []byte(filename), 0644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	run("git", "add", ".")
	run("git", "commit", "-m", message)
}

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
// Ingest
// --------------------------------------------------------------------------

func TestGitHandler_Ingest_ProducesCommitAndAuthorEntities(t *testing.T) {
	repoDir := initTempRepo(t)
	h := githandler.New(githandler.DefaultConfig())

	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities")
	}

	var commits, authors int
	for _, e := range entities {
		switch e.EntityType {
		case "commit":
			commits++
			if e.Domain != "git" {
				t.Errorf("commit Domain = %q, want git", e.Domain)
			}
			if len(e.Instance) < 7 {
				t.Errorf("commit Instance %q too short (want ≥7 chars)", e.Instance)
			}
		case "author":
			authors++
			if e.Domain != "git" {
				t.Errorf("author Domain = %q, want git", e.Domain)
			}
			if e.Instance == "" {
				t.Error("author Instance is empty")
			}
		}
	}

	if commits == 0 {
		t.Error("expected at least one commit entity")
	}
	if authors == 0 {
		t.Error("expected at least one author entity")
	}
}

func TestGitHandler_Ingest_ProducesBranchEntity(t *testing.T) {
	repoDir := initTempRepo(t)
	h := githandler.New(githandler.DefaultConfig())

	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var branches int
	for _, e := range entities {
		if e.EntityType == "branch" {
			branches++
			if e.Domain != "git" {
				t.Errorf("branch Domain = %q, want git", e.Domain)
			}
			if e.Instance == "" {
				t.Error("branch Instance is empty")
			}
		}
	}
	if branches == 0 {
		t.Error("expected at least one branch entity")
	}
}

func TestGitHandler_Ingest_SystemHasNoSlashes(t *testing.T) {
	repoDir := initTempRepo(t)
	h := githandler.New(githandler.DefaultConfig())

	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	for _, e := range entities {
		if e.System == "" {
			t.Errorf("entity %q has empty System", e.EntityType)
		}
		if strings.Contains(e.System, "/") {
			t.Errorf("entity System %q contains slash (not NATS-safe)", e.System)
		}
	}
}

func TestGitHandler_Ingest_MultipleCommits(t *testing.T) {
	repoDir := initTempRepo(t)
	addCommit(t, repoDir, "b.go", "second commit")
	addCommit(t, repoDir, "c.go", "third commit")

	h := githandler.New(githandler.DefaultConfig())
	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var commits int
	for _, e := range entities {
		if e.EntityType == "commit" {
			commits++
		}
	}
	if commits < 3 {
		t.Errorf("expected ≥3 commit entities, got %d", commits)
	}
}

func TestGitHandler_Ingest_InvalidPath_ReturnsError(t *testing.T) {
	h := githandler.New(githandler.DefaultConfig())
	_, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: "/no/such/repo/path"})
	if err == nil {
		t.Error("expected error for invalid repo path, got nil")
	}
}

func TestGitHandler_Ingest_CommitHasFileTouchEdges(t *testing.T) {
	repoDir := initTempRepo(t)
	h := githandler.New(githandler.DefaultConfig())

	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var edgeCount int
	for _, e := range entities {
		edgeCount += len(e.Edges)
	}
	if edgeCount == 0 {
		t.Error("expected at least one file-touch or authored-by edge")
	}
}

// --------------------------------------------------------------------------
// Watch
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

func TestGitHandler_Watch_DetectsNewCommit(t *testing.T) {
	repoDir := initTempRepo(t)

	cfg := githandler.DefaultConfig()
	cfg.PollInterval = 80 * time.Millisecond
	h := githandler.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := h.Watch(ctx, &srcCfg{typ: "git", path: repoDir, watch: true})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if ch == nil {
		t.Fatal("Watch() returned nil channel with watch enabled")
	}

	// Give the watcher one poll cycle to record the baseline HEAD.
	time.Sleep(200 * time.Millisecond)

	// Add a commit to trigger a change.
	addCommit(t, repoDir, "watched.go", "watch trigger commit")

	select {
	case ev := <-ch:
		if ev.Operation != handler.OperationCreate && ev.Operation != handler.OperationModify {
			t.Errorf("unexpected operation %v, want Create or Modify", ev.Operation)
		}
		var hasCommit bool
		for _, e := range ev.Entities {
			if e.EntityType == "commit" {
				hasCommit = true
			}
		}
		if !hasCommit {
			t.Error("watch ChangeEvent should contain commit entities")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for watch event after new commit")
	}
}

func TestGitHandler_Watch_ClosesOnContextCancel(t *testing.T) {
	repoDir := initTempRepo(t)

	cfg := githandler.DefaultConfig()
	cfg.PollInterval = 50 * time.Millisecond
	h := githandler.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := h.Watch(ctx, &srcCfg{typ: "git", path: repoDir, watch: true})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	cancel()

	// Channel must close within a reasonable time after context cancellation.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed — pass
			}
		case <-deadline:
			t.Error("watch channel did not close after context cancellation")
			return
		}
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
