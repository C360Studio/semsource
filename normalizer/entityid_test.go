package normalizer_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/normalizer"
)

// ---------------------------------------------------------------------------
// BuildEntityID
// ---------------------------------------------------------------------------

func TestBuildEntityID(t *testing.T) {
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
			platform:   normalizer.PlatformSemsource,
			domain:     "golang",
			system:     "github.com-acme-gcs",
			entityType: "function",
			instance:   "NewController",
			want:       "acme.semsource.golang.github.com-acme-gcs.function.NewController",
		},
		{
			name:       "git commit",
			org:        "acme",
			platform:   normalizer.PlatformSemsource,
			domain:     "git",
			system:     "github.com-acme-gcs",
			entityType: "commit",
			instance:   "a3f9b2",
			want:       "acme.semsource.git.github.com-acme-gcs.commit.a3f9b2",
		},
		{
			name:       "public namespace open-source symbol",
			org:        "public",
			platform:   normalizer.PlatformSemsource,
			domain:     "golang",
			system:     "github.com-gin-gonic-gin",
			entityType: "function",
			instance:   "New",
			want:       "public.semsource.golang.github.com-gin-gonic-gin.function.New",
		},
		{
			name:       "public web doc",
			org:        "public",
			platform:   normalizer.PlatformSemsource,
			domain:     "web",
			system:     "pkg.go.dev",
			entityType: "doc",
			instance:   "c821de",
			want:       "public.semsource.web.pkg.go.dev.doc.c821de",
		},
		{
			name:       "stdlib function",
			org:        "public",
			platform:   normalizer.PlatformSemsource,
			domain:     "golang",
			system:     "stdlib-net-http",
			entityType: "function",
			instance:   "ListenAndServe",
			want:       "public.semsource.golang.stdlib-net-http.function.ListenAndServe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizer.BuildEntityID(tt.org, tt.platform, tt.domain, tt.system, tt.entityType, tt.instance)
			if got != tt.want {
				t.Errorf("BuildEntityID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildSystemSlug
// ---------------------------------------------------------------------------

func TestBuildSystemSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "github repo path with dots and slashes",
			input: "github.com/acme/gcs",
			want:  "github.com-acme-gcs",
		},
		{
			name:  "gin-gonic with extra dots",
			input: "github.com/gin-gonic/gin",
			want:  "github.com-gin-gonic-gin",
		},
		{
			name:  "stdlib net/http",
			input: "stdlib/net/http",
			want:  "stdlib-net-http",
		},
		{
			name:  "plain hostname no slashes",
			input: "pkg.go.dev",
			want:  "pkg.go.dev",
		},
		{
			name:  "already dash-separated",
			input: "my-repo",
			want:  "my-repo",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "deep path",
			input: "github.com/org/repo/sub",
			want:  "github.com-org-repo-sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizer.BuildSystemSlug(tt.input)
			if got != tt.want {
				t.Errorf("BuildSystemSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// The system slug must not contain slashes (NATS KV key constraint).
func TestBuildSystemSlug_NoSlashes(t *testing.T) {
	inputs := []string{
		"github.com/acme/gcs",
		"pkg.go.dev/some/path",
		"a/b/c/d",
	}
	for _, input := range inputs {
		got := normalizer.BuildSystemSlug(input)
		if strings.Contains(got, "/") {
			t.Errorf("BuildSystemSlug(%q) contains slash: %q", input, got)
		}
	}
}

// ---------------------------------------------------------------------------
// CanonicalizeURL
// ---------------------------------------------------------------------------

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trailing slash removed",
			input: "https://docs.acme.io/gcs/",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "fragment stripped",
			input: "https://docs.acme.io/gcs#section-1",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "trailing slash and fragment",
			input: "https://docs.acme.io/gcs/#section-1",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "query params stripped",
			input: "https://docs.acme.io/gcs?tab=overview",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "uppercase scheme normalized",
			input: "HTTPS://docs.acme.io/gcs",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "uppercase host normalized",
			input: "https://DOCS.ACME.IO/gcs",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "mixed case scheme and host",
			input: "HTTP://PKG.GO.DEV/net/http",
			want:  "http://pkg.go.dev/net/http",
		},
		{
			name:  "no trailing slash on root",
			input: "https://pkg.go.dev",
			want:  "https://pkg.go.dev",
		},
		{
			name:  "path with multiple trailing slashes",
			input: "https://docs.acme.io/gcs///",
			want:  "https://docs.acme.io/gcs",
		},
		{
			name:  "all: uppercase + query + fragment + trailing slash",
			input: "HTTPS://DOCS.ACME.IO/GCS/?foo=bar#top",
			want:  "https://docs.acme.io/GCS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizer.CanonicalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("CanonicalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsPublicNamespace
// ---------------------------------------------------------------------------

func TestIsPublicNamespace(t *testing.T) {
	tests := []struct {
		org  string
		want bool
	}{
		{"public", true},
		{"acme", false},
		{"PUBLIC", false}, // case-sensitive
		{"", false},
		{"public-org", false},
	}
	for _, tt := range tests {
		got := normalizer.IsPublicNamespace(tt.org)
		if got != tt.want {
			t.Errorf("IsPublicNamespace(%q) = %v, want %v", tt.org, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidateNATSKVKey
// ---------------------------------------------------------------------------

func TestValidateNATSKVKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid 6-part ID",
			key:     "acme.semsource.golang.github.com-acme-gcs.function.NewController",
			wantErr: false,
		},
		{
			name:    "valid public namespace",
			key:     "public.semsource.web.pkg.go.dev.doc.c821de",
			wantErr: false,
		},
		{
			name:    "empty string",
			key:     "",
			wantErr: true,
		},
		{
			name:    "contains space",
			key:     "acme.semsource.golang.repo.function.New Controller",
			wantErr: true,
		},
		{
			name:    "contains wildcard star",
			key:     "acme.semsource.golang.*.function.New",
			wantErr: true,
		},
		{
			name:    "contains greater-than",
			key:     "acme.semsource.golang.repo.function.>",
			wantErr: true,
		},
		{
			name:    "contains slash",
			key:     "acme/semsource.golang.repo.function.New",
			wantErr: true,
		},
		{
			name:    "valid with hyphens",
			key:     "acme.semsource.git.github.com-acme-gcs.commit.a3f9b2",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			key:     "acme.semsource.golang.my_repo.function.my_func",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizer.ValidateNATSKVKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNATSKVKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PlatformSemsource constant
// ---------------------------------------------------------------------------

func TestPlatformSemsourceConstant(t *testing.T) {
	if normalizer.PlatformSemsource != "semsource" {
		t.Errorf("PlatformSemsource = %q, want %q", normalizer.PlatformSemsource, "semsource")
	}
}

// ---------------------------------------------------------------------------
// Integration: BuildEntityID produces valid NATS KV keys for all entity types
// ---------------------------------------------------------------------------

func TestBuildEntityID_AllTypesProduceValidNATSKeys(t *testing.T) {
	// Code symbol: acme.semsource.golang.github.com-acme-gcs.function.NewController
	id1 := normalizer.BuildEntityID("acme", normalizer.PlatformSemsource, "golang",
		normalizer.BuildSystemSlug("github.com/acme/gcs"), "function", "NewController")

	// Git commit: acme.semsource.git.github.com-acme-gcs.commit.a3f9b2
	id2 := normalizer.BuildEntityID("acme", normalizer.PlatformSemsource, "git",
		normalizer.BuildSystemSlug("github.com/acme/gcs"), "commit", "a3f9b2")

	// URL/doc (instance is sha256 prefix):  acme.semsource.web.docs.acme.io.doc.ab12cd
	id3 := normalizer.BuildEntityID("acme", normalizer.PlatformSemsource, "web",
		"docs.acme.io", "doc", "ab12cd")

	// Config file: acme.semsource.config.github.com-acme-gcs.dockerfile.ab12cd
	id4 := normalizer.BuildEntityID("acme", normalizer.PlatformSemsource, "config",
		normalizer.BuildSystemSlug("github.com/acme/gcs"), "dockerfile", "ab12cd")

	for _, id := range []string{id1, id2, id3, id4} {
		if err := normalizer.ValidateNATSKVKey(id); err != nil {
			t.Errorf("ID %q failed NATS KV validation: %v", id, err)
		}
	}
}
