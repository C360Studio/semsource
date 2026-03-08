package entityid_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
)

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
		{"github.com/acme/gcs", "github.com-acme-gcs"},
		{"github.com/gin-gonic/gin", "github.com-gin-gonic-gin"},
		{"stdlib/net/http", "stdlib-net-http"},
		{"pkg.go.dev", "pkg.go.dev"},
		{"my-repo", "my-repo"},
		{"", ""},
	}

	for _, tt := range tests {
		got := entityid.SystemSlug(tt.input)
		if got != tt.want {
			t.Errorf("SystemSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
