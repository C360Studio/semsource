package config

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
)

// TestProjectIsNormalizedNotRejected pins the asymmetry between the two
// identity-shaped configuration values, because the spec previously claimed they
// were treated the same and they never were.
//
// `runtime-configuration` used to require that every value becoming an entity-ID
// segment — "namespace/org, explicit source identity overrides" — was validated
// at load and "rejected, never silently rewritten". Only namespace is. There is
// no `project` validation anywhere in config load, and `entityid.SystemSlug`
// maps out-of-alphabet runes to '-' and truncates past 80 characters with a
// content hash.
//
// The asymmetry is deliberate, not an oversight, which is why the spec was
// narrowed to it rather than the code changed to match the spec:
//
//   - org is the sovereignty boundary. It is never normalized, so a bad one must
//     fail loudly at startup — it would otherwise break every entity in the
//     deployment (audit 2026-07-19: "acme.io" passed validate and produced a
//     permanently empty graph).
//   - project is routinely a module path or a filesystem path. SystemSlug exists
//     precisely to slugify those, so rejecting a value it would cleanly handle
//     costs usability for no safety gain.
//
// This test exists so the two cannot drift apart again silently.
func TestProjectIsNormalizedNotRejected(t *testing.T) {
	messy := []string{
		"github.com/acme/thing",
		"acme thing",
		"semstreams@v1.2.3",
		"./local/path",
	}

	for _, project := range messy {
		t.Run(project, func(t *testing.T) {
			// It loads. No validation rejects it.
			cfg := &Config{
				Namespace: "acme",
				Sources: []SourceEntry{
					{Type: "docs", Paths: []string{"./docs"}, Project: project},
				},
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected project %q: %v — project is normalized by design, "+
					"not validated; if this now fails, the spec must change with the code", project, err)
			}

			// And it is rewritten on the way to an entity ID.
			slug := entityid.SystemSlug(project)
			if slug == "" {
				t.Fatalf("SystemSlug(%q) is empty", project)
			}
			for _, r := range slug {
				ok := r == '-' || r == '_' ||
					(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
				if !ok {
					t.Errorf("SystemSlug(%q) = %q contains %q, outside the ID alphabet", project, slug, r)
				}
			}
		})
	}
}

// The other half of the contract: org really is rejected rather than slugified,
// so the narrowed requirement's first sentence stays true.
func TestOrgIsRejectedNotNormalized(t *testing.T) {
	cfg := &Config{
		Namespace: "acme.io",
		Sources:   []SourceEntry{{Type: "docs", Paths: []string{"./docs"}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted a dotted namespace; org must be rejected, never rewritten")
	}
	if !strings.Contains(err.Error(), "acme.io") {
		t.Errorf("error %q does not name the offending value", err)
	}
}
