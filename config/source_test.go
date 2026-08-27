package config

import (
	"sort"
	"strings"
	"testing"
)

// TestSourceEntry_Validate_GitAcceptsPathOnly is the regression guard for issue
// #1: a git source may be configured with a url OR a local path. Path-only is
// the sidecar case — index a mounted/agent worktree in place without cloning
// (ADR-0007). Previously git required a url, forcing the clone path.
func TestSourceEntry_Validate_GitAcceptsPathOnly(t *testing.T) {
	cases := []struct {
		name    string
		entry   SourceEntry
		wantErr bool
	}{
		{"git url only", SourceEntry{Type: "git", URL: "https://example.com/x.git"}, false},
		{"git path only (issue #1)", SourceEntry{Type: "git", Path: "/mnt/workspace"}, false},
		{"git url and path", SourceEntry{Type: "git", URL: "https://example.com/x.git", Path: "/mnt/workspace"}, false},
		{"git neither url nor path", SourceEntry{Type: "git"}, true},
		// repo already accepted url-or-path; assert parity so the two stay aligned.
		{"repo path only", SourceEntry{Type: "repo", Path: "/mnt/workspace"}, false},
		{"repo neither", SourceEntry{Type: "repo"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestSourceEntry_Validate_ObjectStore covers what a bad s3 entry has to be
// rejected for, and what a good one must be accepted for. The rejections
// matter because they are the difference between a source that fails at load
// with a message naming the field, and one that starts and then cannot reach
// anything.
func TestSourceEntry_Validate_ObjectStore(t *testing.T) {
	rejected := []struct {
		name  string
		entry SourceEntry
		names string
	}{
		{"no bucket", SourceEntry{Type: "s3", Prefix: "reports/"}, "bucket"},
		{"endpoint with no scheme", SourceEntry{Type: "s3", Bucket: "artifacts", Endpoint: "localhost:9000"}, "endpoint"},
		{"unparseable endpoint", SourceEntry{Type: "s3", Bucket: "artifacts", Endpoint: "://storage"}, "endpoint"},
		{"endpoint with a base path", SourceEntry{Type: "s3", Bucket: "artifacts", Endpoint: "https://storage.example.com/s3/v1"}, "endpoint"},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.entry.Validate()
			if err == nil {
				t.Fatalf("expected an error naming %q", tc.names)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("error should name %q, got: %v", tc.names, err)
			}
		})
	}

	accepted := []struct {
		name  string
		entry SourceEntry
	}{
		{"bucket only — the default endpoint applies", SourceEntry{Type: "s3", Bucket: "artifacts"}},
		{"self-hosted, path style", SourceEntry{
			Type:      "s3",
			Bucket:    "artifacts",
			Prefix:    "reports/",
			Endpoint:  "https://garage.internal:3900",
			Region:    "garage",
			PathStyle: true,
		}},
		{"explicit project identity", SourceEntry{Type: "s3", Bucket: "artifacts", Project: "quarterly-reports"}},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.entry.Validate(); err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestSourceTypes_IsTheOneList pins the accessor the spawn-side invariant test
// reads through. The set used to appear three times — the map, the error
// message, and the type switch in internal/sourcespawn — with nothing checking
// that they agreed.
func TestSourceTypes_IsTheOneList(t *testing.T) {
	types := SourceTypes()
	if len(types) != len(validSourceTypes) {
		t.Fatalf("SourceTypes returned %d entries for %d valid types", len(types), len(validSourceTypes))
	}
	for _, sourceType := range types {
		if !validSourceTypes[sourceType] {
			t.Errorf("SourceTypes returned %q, which is not a valid type", sourceType)
		}
	}
	if !sort.StringsAreSorted(types) {
		t.Errorf("SourceTypes is not sorted: %v", types)
	}

	// The rejection message is built from the same list, so it cannot drift
	// from what is actually accepted.
	err := SourceEntry{Type: "not-a-source"}.Validate()
	if err == nil {
		t.Fatal("expected an error for an unknown type")
	}
	for _, sourceType := range types {
		if !strings.Contains(err.Error(), sourceType) {
			t.Errorf("the rejection message omits the valid type %q: %v", sourceType, err)
		}
	}
}

// TestLoadConfig_RejectsCredentialsOnAnObjectStoreEntry is the runtime half of
// the no-credentials rule, and the half that matters most: this document is
// watched and replicated through KV, so a key placed on a source entry would
// be distributed well beyond the process that needs it.
//
// Nothing credential-specific enforces this. SourceEntry has nowhere to put a
// key, so the loader's strict decoding rejects one as an ordinary unknown
// field — the same classification any typo gets, with no special case to
// maintain or forget. The test exists because that is a property of the
// struct's shape, and a well-meaning future field would silently remove it.
func TestLoadConfig_RejectsCredentialsOnAnObjectStoreEntry(t *testing.T) {
	fields := []string{
		"access_key_id",
		"secret_access_key",
		"aws_access_key_id",
		"aws_secret_access_key",
		"session_token",
		"credentials",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			doc := `{
				"namespace": "acme",
				"sources": [
					{"type": "s3", "bucket": "artifacts", "` + field + `": "AKIAIOSFODNN7EXAMPLE"}
				]
			}`

			_, err := LoadConfigFromReader(strings.NewReader(doc))
			if err == nil {
				t.Fatalf("the loader accepted a %q field on a source entry", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("the error should name the rejected field, got: %v", err)
			}
		})
	}

	// The same document without the credential loads, so the rejection above
	// is attributable to the credential and not to the rest of the fixture.
	clean := `{
		"namespace": "acme",
		"sources": [{"type": "s3", "bucket": "artifacts", "prefix": "reports/"}]
	}`
	if _, err := LoadConfigFromReader(strings.NewReader(clean)); err != nil {
		t.Fatalf("the credential-free entry should load, got: %v", err)
	}
}
