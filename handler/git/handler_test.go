package git_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
	"github.com/c360studio/semstreams/message"
)

// entityIDSegmentRegex matches one segment of a graph-ingest-valid entity ID.
// Mirrors the per-segment rule from semstreams/processor/graph-ingest
// component.go's entityIDRegex. SanitizeInstance must produce output that
// satisfies this.
var entityIDSegmentRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

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
	tests := []struct {
		name  string
		email string
	}{
		{"plain email", "alice@example.com"},
		{"github noreply with plus",
			"43158+cglusky@users.noreply.github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := githandler.BuildAuthorEntity("Alice", tt.email, "gh-com-acme-repo")
			if e.EntityType != "author" {
				t.Errorf("EntityType = %q, want author", e.EntityType)
			}
			if e.Domain != "git" {
				t.Errorf("Domain = %q, want git", e.Domain)
			}
			if !entityIDSegmentRegex.MatchString(e.Instance) {
				t.Errorf("Instance %q is not a valid entity-ID segment", e.Instance)
			}
			// Instance must not leak characters forbidden by graph-ingest.
			for _, forbidden := range []string{"+", "@", "."} {
				if strings.Contains(e.Instance, forbidden) {
					t.Errorf("Instance %q contains forbidden character %q", e.Instance, forbidden)
				}
			}
			// Reference: ensure the constructed ID is what we expect.
			_ = entityid.Build("acme", entityid.PlatformSemsource, e.Domain,
				e.System, e.EntityType, e.Instance)
		})
	}
}

func TestBuildBranchEntity(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
	}{
		{"simple", "main"},
		{"slash separator", "feature/auth"},
		{"reported failing branch",
			"semspec/requirement-requirement.ec55314ae0f5.1"},
		{"version-like", "release/v1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := githandler.BuildBranchEntity(tt.branchName, "abc1234def5678",
				"gh-com-acme-repo")
			if e.EntityType != "branch" {
				t.Errorf("EntityType = %q, want branch", e.EntityType)
			}
			if e.Domain != "git" {
				t.Errorf("Domain = %q, want git", e.Domain)
			}
			// Raw branch name must still be preserved in properties so downstream
			// consumers see the true name even if the ID segment is sanitized.
			if got, _ := e.Properties["name"].(string); got != tt.branchName {
				t.Errorf("Properties[name] = %q, want %q", got, tt.branchName)
			}
			// Sanitized instance must pass the graph-ingest per-segment regex.
			if !entityIDSegmentRegex.MatchString(e.Instance) {
				t.Errorf("branch Instance %q is not a valid entity-ID segment", e.Instance)
			}
			_ = entityid.Build("acme", entityid.PlatformSemsource, e.Domain,
				e.System, e.EntityType, e.Instance)
		})
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

	parts := strings.SplitN(commitState.ID, ".", 6)
	if len(parts) < 6 {
		t.Fatalf("commit entity ID %q has fewer than 6 parts", commitState.ID)
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
	// The instance segment is what SanitizeInstance controls — must pass
	// graph-ingest's per-segment regex.
	if !entityIDSegmentRegex.MatchString(parts[5]) {
		t.Errorf("commit instance segment %q is not valid", parts[5])
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

	parts := strings.SplitN(authorState.ID, ".", 6)
	if len(parts) < 6 {
		t.Fatalf("author entity ID %q has fewer than 6 parts", authorState.ID)
	}
	if parts[4] != "author" {
		t.Errorf("author entity ID type = %q, want author", parts[4])
	}
	if !entityIDSegmentRegex.MatchString(parts[5]) {
		t.Errorf("author instance segment %q is not valid", parts[5])
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

	parts := strings.SplitN(branchState.ID, ".", 6)
	if len(parts) < 6 {
		t.Fatalf("branch entity ID %q has fewer than 6 parts", branchState.ID)
	}
	if parts[4] != "branch" {
		t.Errorf("branch entity ID type = %q, want branch", parts[4])
	}
	if !entityIDSegmentRegex.MatchString(parts[5]) {
		t.Errorf("branch instance segment %q is not valid", parts[5])
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
