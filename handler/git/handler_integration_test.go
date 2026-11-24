//go:build integration

package git_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
)

// --------------------------------------------------------------------------
// Test helpers (git binary required)
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

// --------------------------------------------------------------------------
// Ingest (requires git binary)
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

// TestGitHandler_Ingest_BranchNameWithSpecialChars reproduces the failure mode
// reported by semspec: a branch name containing '/' and '.' (e.g. the
// requirement-executor's `semspec/requirement-<id>` convention) must produce
// a valid graph-ingest segment — not a passthrough that breaks 6-part parsing.
func TestGitHandler_Ingest_BranchNameWithSpecialChars(t *testing.T) {
	repoDir := initTempRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	// Exact pattern from the bug report.
	branchName := "semspec/requirement-requirement.ec55314ae0f5.1"
	run("git", "checkout", "-b", branchName)

	h := githandler.New(githandler.DefaultConfig())
	entities, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: repoDir})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	segRE := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
	found := false
	for _, e := range entities {
		if e.EntityType != "branch" {
			continue
		}
		if name, _ := e.Properties["name"].(string); name == branchName {
			found = true
			if !segRE.MatchString(e.Instance) {
				t.Errorf("branch Instance %q is not a valid entity-ID segment", e.Instance)
			}
		}
	}
	if !found {
		t.Errorf("did not find branch entity for %q", branchName)
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
// Watch (requires git binary)
// --------------------------------------------------------------------------

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

// TestGitHandler_Watch_CarriesEntityStates: git-source publishes a watch
// event's EntityStates, not its legacy Entities, so a watch event without them
// means commits made after startup never reach the graph (#270). Commits are
// added until an event arrives, because the poll loop records its baseline
// HEAD asynchronously and a commit made before that is not a change.
//
// spec: openspec/specs/typed-source-change-events/spec.md#Requirement: Git watch changes carry typed entity state
func TestGitHandler_Watch_CarriesEntityStates(t *testing.T) {
	repoDir := initTempRepo(t)
	cfg := githandler.DefaultConfig()
	cfg.PollInterval = 50 * time.Millisecond
	cfg.Authority = testAuthority("acme")
	h := githandler.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := h.Watch(ctx, &srcCfg{typ: "git", path: repoDir, watch: true})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	commit := time.NewTicker(200 * time.Millisecond)
	defer commit.Stop()
	for n := 0; ; n++ {
		select {
		case ev := <-ch:
			var commits int
			for _, s := range ev.EntityStates {
				if findPredicate(s.Triples, "source.git.commit-sha") == nil {
					continue
				}
				commits++
				if !strings.HasPrefix(s.ID, "acme.test-platform.") {
					t.Errorf("commit state %q is not under the configured authority", s.ID)
				}
			}
			if commits == 0 {
				t.Fatalf("watch event after a new commit carries no commit EntityStates (%d states)", len(ev.EntityStates))
			}
			return
		case <-commit.C:
			addCommit(t, repoDir, fmt.Sprintf("watched-%d.go", n), "watch trigger commit")
		case <-ctx.Done():
			t.Fatal("timed out waiting for a watch event after new commits")
		}
	}
}

// TestGitHandler_Watch_RetriesHeadChangeWhenStatesFail: when building a HEAD
// change's entity states fails, the poll loop must not emit a state-less event
// or advance past that HEAD; it logs the failure and retries on the next tick
// (#355). An authority that fails validation makes IngestEntityStates fail on
// every call while Ingest succeeds. Commits are added until the first failure
// is logged (the baseline HEAD is recorded asynchronously). A retry is proven
// by two failure records for the same SHA: a loop that advances past a failed
// HEAD logs each SHA at most once, however commits and polls interleave.
//
// spec: openspec/specs/typed-source-change-events/spec.md#Requirement: Git watch changes carry typed entity state
func TestGitHandler_Watch_RetriesHeadChangeWhenStatesFail(t *testing.T) {
	repoDir := initTempRepo(t)
	failures := make(chan string, 64)
	cfg := githandler.DefaultConfig()
	cfg.PollInterval = 50 * time.Millisecond
	cfg.Authority = entityid.Authority{Org: "acme", Platform: "bad.platform"}
	cfg.Logger = slog.New(&statesFailureRecorder{shas: failures})
	h := githandler.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := h.Watch(ctx, &srcCfg{typ: "git", path: repoDir, watch: true})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	commit := time.NewTicker(200 * time.Millisecond)
	defer commit.Stop()
	seen := make(map[string]bool)
	for n := 0; ; n++ {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("watch channel closed before the failed HEAD change was retried")
			}
			t.Fatalf("watch emitted an event (%d entities, %d states) although building its entity states failed",
				len(ev.Entities), len(ev.EntityStates))
		case sha := <-failures:
			if seen[sha] {
				return
			}
			seen[sha] = true
		case <-commit.C:
			if len(seen) == 0 {
				addCommit(t, repoDir, fmt.Sprintf("retry-%d.go", n), "watch trigger commit")
			}
		case <-ctx.Done():
			t.Fatalf("timed out: no HEAD was retried after its entity states failed (failed SHAs: %v)", seen)
		}
	}
}

// statesFailureRecorder is a slog.Handler that forwards the new_sha of each
// "building entity states" failure the git poll loop logs.
type statesFailureRecorder struct{ shas chan<- string }

func (r *statesFailureRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *statesFailureRecorder) Handle(_ context.Context, rec slog.Record) error {
	if !strings.Contains(rec.Message, "building entity states") {
		return nil
	}
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "new_sha" {
			select {
			case r.shas <- a.Value.String():
			default: // never block the poll loop once the test has its answer
			}
			return false
		}
		return true
	})
	return nil
}

func (r *statesFailureRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *statesFailureRecorder) WithGroup(string) slog.Handler { return r }

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

// TestGitHandler_Watch_KeepsURLSystemSlug: a source configured with both a
// local path and a repo URL seeds under the URL-derived system slug, so a
// poll re-ingest after a HEAD change must mint under the same slug. Before
// #350 the poll loop re-ingested through a path-only config and every commit,
// author and branch ID moved to the path-derived slug — a parallel entity set.
//
// spec: openspec/specs/typed-source-change-events/spec.md#Requirement: Git watch changes carry typed entity state
func TestGitHandler_Watch_KeepsURLSystemSlug(t *testing.T) {
	repoDir := initTempRepo(t)
	cfg := githandler.DefaultConfig()
	cfg.PollInterval = 50 * time.Millisecond
	cfg.Authority = testAuthority("acme")
	h := githandler.New(cfg)
	src := &srcCfg{typ: "git", path: repoDir, url: "https://github.com/acme/widgets.git", watch: true}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seed, err := h.IngestEntityStates(ctx, src, cfg.Authority)
	if err != nil || len(seed) == 0 {
		t.Fatalf("seed IngestEntityStates() = %d states, err %v", len(seed), err)
	}
	want := systemOf(t, seed[0].ID)

	ch, err := h.Watch(ctx, src)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	commit := time.NewTicker(200 * time.Millisecond)
	defer commit.Stop()
	for n := 0; ; n++ {
		select {
		case ev := <-ch:
			if len(ev.EntityStates) == 0 {
				t.Fatal("watch event carries no EntityStates")
			}
			for _, s := range ev.EntityStates {
				if got := systemOf(t, s.ID); got != want {
					t.Errorf("poll state %q has system %q, seed system %q", s.ID, got, want)
				}
			}
			return
		case <-commit.C:
			addCommit(t, repoDir, fmt.Sprintf("url-%d.go", n), "watch trigger commit")
		case <-ctx.Done():
			t.Fatal("timed out waiting for a watch event after new commits")
		}
	}
}

// systemOf returns the system segment of a 6-part entity ID.
func systemOf(t *testing.T, id string) string {
	t.Helper()
	parts := strings.Split(id, ".")
	if len(parts) != 6 {
		t.Fatalf("entity ID %q is not 6-part", id)
	}
	return parts[2]
}
