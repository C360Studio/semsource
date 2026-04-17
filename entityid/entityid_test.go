package entityid_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
)

// entityIDSegmentRegex mirrors the per-segment rule enforced by
// semstreams/processor/graph-ingest/component.go's entityIDRegex.
// Keep this identical — it is the contract SanitizeInstance must satisfy.
var entityIDSegmentRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func TestParts(t *testing.T) {
	tests := []struct {
		id         string
		wantDomain string
		wantType   string
	}{
		{"acme.semsource.golang.myrepo.function.main-go-Foo", "golang", "function"},
		{"acme.semsource.git.myrepo.commit.abc123", "git", "commit"},
		{"acme.semsource.web.docs.doc.sha256", "web", "doc"},
		{"acme.semsource.config.myrepo.dependency.lodash", "config", "dependency"},
		{"acme.semsource.code.myrepo.folder.src-pkg", "code", "folder"},
		// Instance segment with dots — SplitN(6) captures everything after 5th dot.
		{"acme.semsource.java.repo.class.com.example.Foo", "java", "class"},
		// Too few parts.
		{"acme.semsource.golang", "", ""},
		{"", "", ""},
		// Exactly 6 parts.
		{"a.b.c.d.e.f", "c", "e"},
	}
	for _, tt := range tests {
		domain, eType := entityid.Parts(tt.id)
		if domain != tt.wantDomain || eType != tt.wantType {
			t.Errorf("Parts(%q) = (%q, %q), want (%q, %q)",
				tt.id, domain, eType, tt.wantDomain, tt.wantType)
		}
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name       string
		org        string
		platform   string
		domain     string
		system     string
		entityType string
		instance   string
		want       string
	}{
		{
			name:       "code symbol",
			org:        "acme",
			platform:   entityid.PlatformSemsource,
			domain:     "golang",
			system:     "github.com-acme-gcs",
			entityType: "function",
			instance:   "NewController",
			want:       "acme.semsource.golang.github.com-acme-gcs.function.NewController",
		},
		{
			name:       "git commit",
			org:        "acme",
			platform:   entityid.PlatformSemsource,
			domain:     "git",
			system:     "github.com-acme-gcs",
			entityType: "commit",
			instance:   "a3f9b2",
			want:       "acme.semsource.git.github.com-acme-gcs.commit.a3f9b2",
		},
		{
			name:       "public namespace",
			org:        "public",
			platform:   entityid.PlatformSemsource,
			domain:     "golang",
			system:     "github.com-gin-gonic-gin",
			entityType: "function",
			instance:   "New",
			want:       "public.semsource.golang.github.com-gin-gonic-gin.function.New",
		},
		{
			name:       "public web doc",
			org:        "public",
			platform:   entityid.PlatformSemsource,
			domain:     "web",
			system:     "pkg.go.dev",
			entityType: "doc",
			instance:   "c821de",
			want:       "public.semsource.web.pkg.go.dev.doc.c821de",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entityid.Build(tt.org, tt.platform, tt.domain, tt.system, tt.entityType, tt.instance)
			if got != tt.want {
				t.Errorf("Build() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSystemSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/acme/gcs", "github-com-acme-gcs"},
		{"github.com/gin-gonic/gin", "github-com-gin-gonic-gin"},
		{"stdlib/net/http", "stdlib-net-http"},
		{"pkg.go.dev", "pkg-go-dev"},
		{"my-repo", "my-repo"},
		// Absolute paths use only the base name to avoid encoding
		// deep directory hierarchies into entity IDs.
		{"/data/fixture", "fixture"},
		{"/tmp/test-workspace/src", "src"},
		{"/var/folders/db/long-temp-path/github-com-opensensorhub-osh-core", "github-com-opensensorhub-osh-core"},
		{"./src", "src"},
		{"///leading-slashes///", "leading-slashes"},
		{"", ""},
	}

	for _, tt := range tests {
		got := entityid.SystemSlug(tt.input)
		if got != tt.want {
			t.Errorf("SystemSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSystemSlug_MaxLength(t *testing.T) {
	// A very long directory name should be capped.
	long := strings.Repeat("a", 120)
	got := entityid.SystemSlug(long)
	if len(got) > 80 {
		t.Errorf("SystemSlug(120-char input) length = %d, want <= 80", len(got))
	}
	// Still produces a non-empty slug.
	if got == "" {
		t.Error("SystemSlug(120-char input) should not be empty")
	}
}

func TestSystemSlug_NoSlashes(t *testing.T) {
	inputs := []string{"github.com/acme/gcs", "pkg.go.dev/some/path", "a/b/c/d"}
	for _, input := range inputs {
		got := entityid.SystemSlug(input)
		if strings.Contains(got, "/") {
			t.Errorf("SystemSlug(%q) contains slash: %q", input, got)
		}
	}
}

// TestSystemSlug_IsValidGraphIngestSegment guards against regressions that
// would reintroduce '.' or other characters that break the 6-part entity ID.
func TestSystemSlug_IsValidGraphIngestSegment(t *testing.T) {
	inputs := []string{
		"github.com/acme/gcs",
		"https://github.com/opensensorhub/osh-core",
		"pkg.go.dev",
		"pkg.go.dev/net/http",
		"docs.acme.io",
		"https://docs.anthropic.com/en/api",
	}
	for _, input := range inputs {
		got := entityid.SystemSlug(input)
		if !entityIDSegmentRegex.MatchString(got) {
			t.Errorf("SystemSlug(%q) = %q, not a valid entity-ID segment", input, got)
		}
	}
}

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trailing slash", "https://docs.acme.io/gcs/", "https://docs.acme.io/gcs"},
		{"fragment stripped", "https://docs.acme.io/gcs#section-1", "https://docs.acme.io/gcs"},
		{"query stripped", "https://docs.acme.io/gcs?tab=overview", "https://docs.acme.io/gcs"},
		{"uppercase scheme", "HTTPS://docs.acme.io/gcs", "https://docs.acme.io/gcs"},
		{"uppercase host", "https://DOCS.ACME.IO/gcs", "https://docs.acme.io/gcs"},
		{"mixed case", "HTTP://PKG.GO.DEV/net/http", "http://pkg.go.dev/net/http"},
		{"no trailing slash on root", "https://pkg.go.dev", "https://pkg.go.dev"},
		{"all combined", "HTTPS://DOCS.ACME.IO/GCS/?foo=bar#top", "https://docs.acme.io/GCS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entityid.CanonicalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("CanonicalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsPublicNamespace(t *testing.T) {
	tests := []struct {
		org  string
		want bool
	}{
		{"public", true},
		{"acme", false},
		{"PUBLIC", false},
		{"", false},
	}
	for _, tt := range tests {
		got := entityid.IsPublicNamespace(tt.org)
		if got != tt.want {
			t.Errorf("IsPublicNamespace(%q) = %v, want %v", tt.org, got, tt.want)
		}
	}
}

func TestValidateNATSKVKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 6-part", "acme.semsource.golang.github.com-acme-gcs.function.NewController", false},
		{"valid public", "public.semsource.web.pkg.go.dev.doc.c821de", false},
		{"empty", "", true},
		{"space", "acme.semsource.golang.repo.function.New Controller", true},
		{"wildcard", "acme.semsource.golang.*.function.New", true},
		{"gt", "acme.semsource.golang.repo.function.>", true},
		{"slash", "acme/semsource.golang.repo.function.New", true},
		{"hyphens ok", "acme.semsource.git.github.com-acme-gcs.commit.a3f9b2", false},
		{"underscores ok", "acme.semsource.golang.my_repo.function.my_func", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := entityid.ValidateNATSKVKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNATSKVKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeInstance(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple branch", "main", "main"},
		{"branch with slash", "feature/auth", "feature-auth"},
		{"branch with dots", "v1.0.0", "v1-0-0"},
		{"reported failing branch",
			"semspec/requirement-requirement.ec55314ae0f5.1",
			"semspec-requirement-requirement-ec55314ae0f5-1"},
		{"github noreply email",
			"43158+cglusky@users.noreply.github.com",
			"43158-cglusky-users-noreply-github-com"},
		{"plain email", "alice@example.com", "alice-example-com"},
		{"preserves case", "FeatureAuth", "FeatureAuth"},
		{"keeps hyphens and underscores", "my-branch_name", "my-branch_name"},
		{"collapses runs of separators", "foo///...bar", "foo-bar"},
		{"trims leading underscore", "_leading", "leading"},
		{"trims trailing underscore", "trailing_", "trailing"},
		{"space", "with space", "with-space"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entityid.SanitizeInstance(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeInstance(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !entityIDSegmentRegex.MatchString(got) {
				t.Errorf("SanitizeInstance(%q) = %q, does not match entity-ID segment regex",
					tt.input, got)
			}
		})
	}
}

func TestSanitizeInstance_FallbackHash(t *testing.T) {
	// Degenerate inputs that sanitize to nothing must still yield a valid
	// alphanumeric segment (the deterministic content hash).
	inputs := []string{"", "---", "...", "/.//", "___"}
	for _, input := range inputs {
		got := entityid.SanitizeInstance(input)
		if !entityIDSegmentRegex.MatchString(got) {
			t.Errorf("SanitizeInstance(%q) = %q, not a valid segment", input, got)
		}
	}
}

func TestSanitizeInstance_DeterministicForSameInput(t *testing.T) {
	input := "feature/auth.v2"
	a := entityid.SanitizeInstance(input)
	b := entityid.SanitizeInstance(input)
	if a != b {
		t.Errorf("SanitizeInstance not deterministic: %q vs %q", a, b)
	}
}

func TestSanitizeInstance_LengthCapPreservesUniqueness(t *testing.T) {
	// Two near-identical long inputs that share a long prefix must sanitize
	// to distinct segments (the hash suffix guarantees this).
	longA := strings.Repeat("a", 100) + "-suffix-A"
	longB := strings.Repeat("a", 100) + "-suffix-B"
	a := entityid.SanitizeInstance(longA)
	b := entityid.SanitizeInstance(longB)
	if a == b {
		t.Errorf("long inputs collided: both sanitize to %q", a)
	}
	if !entityIDSegmentRegex.MatchString(a) || !entityIDSegmentRegex.MatchString(b) {
		t.Errorf("long-input sanitization failed regex: %q / %q", a, b)
	}
	if len(a) > 60 || len(b) > 60 {
		t.Errorf("length cap exceeded: %d / %d", len(a), len(b))
	}
}

func TestBuild_AllTypesProduceValidNATSKeys(t *testing.T) {
	ids := []string{
		entityid.Build("acme", entityid.PlatformSemsource, "golang",
			entityid.SystemSlug("github.com/acme/gcs"), "function", "NewController"),
		entityid.Build("acme", entityid.PlatformSemsource, "git",
			entityid.SystemSlug("github.com/acme/gcs"), "commit", "a3f9b2"),
		entityid.Build("acme", entityid.PlatformSemsource, "web",
			"docs.acme.io", "doc", "ab12cd"),
		entityid.Build("acme", entityid.PlatformSemsource, "config",
			entityid.SystemSlug("github.com/acme/gcs"), "dockerfile", "ab12cd"),
	}

	for _, id := range ids {
		if err := entityid.ValidateNATSKVKey(id); err != nil {
			t.Errorf("ID %q failed NATS KV validation: %v", id, err)
		}
	}
}

func TestOrgFromID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"acme.semsource.golang.repo.function.New", "acme"},
		{"public.semsource.web.pkg.go.dev.doc.c821de", "public"},
		{"", ""},
		{"noperiods", ""},
	}
	for _, tt := range tests {
		got := entityid.OrgFromID(tt.id)
		if got != tt.want {
			t.Errorf("OrgFromID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestResolveOrg(t *testing.T) {
	if got := entityid.ResolveOrg("public", "acme"); got != "public" {
		t.Errorf("ResolveOrg(public, acme) = %q, want public", got)
	}
	if got := entityid.ResolveOrg("other", "acme"); got != "acme" {
		t.Errorf("ResolveOrg(other, acme) = %q, want acme", got)
	}
	if got := entityid.ResolveOrg("", "acme"); got != "acme" {
		t.Errorf("ResolveOrg(empty, acme) = %q, want acme", got)
	}
}

func TestBranchScopedSlug(t *testing.T) {
	tests := []struct {
		systemSlug string
		branchSlug string
		want       string
	}{
		{"github-com-acme-repo", "", "github-com-acme-repo"},
		{"github-com-acme-repo", "main", "github-com-acme-repo-main"},
		{"github-com-acme-repo", "scenario-auth-flow", "github-com-acme-repo-scenario-auth-flow"},
		{"my-repo", "feature-123", "my-repo-feature-123"},
	}
	for _, tt := range tests {
		got := entityid.BranchScopedSlug(tt.systemSlug, tt.branchSlug)
		if got != tt.want {
			t.Errorf("BranchScopedSlug(%q, %q) = %q, want %q",
				tt.systemSlug, tt.branchSlug, got, tt.want)
		}
		// Result must pass both NATS KV validation and the graph-ingest
		// per-segment regex — it serves as a single entity ID segment.
		if tt.want != "" {
			if err := entityid.ValidateNATSKVKey(tt.want); err != nil {
				t.Errorf("BranchScopedSlug result %q is not a valid NATS KV key: %v", tt.want, err)
			}
			if !entityIDSegmentRegex.MatchString(tt.want) {
				t.Errorf("BranchScopedSlug result %q is not a valid entity-ID segment", tt.want)
			}
		}
	}
}
