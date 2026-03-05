// Package normalizer converts raw handler entities into normalized graph entities
// with fully-qualified 6-part deterministic IDs per spec Section 4.
package normalizer

import (
	"fmt"
	"strings"
)

// PlatformSemsource is the platform segment used in all SemSource entity IDs.
const PlatformSemsource = "semsource"

// BuildEntityID constructs the canonical 6-part entity ID string.
// Format: {org}.{platform}.{domain}.{system}.{type}.{instance}
// All parts must be non-empty — callers are responsible for supplying valid values.
func BuildEntityID(org, platform, domain, system, entityType, instance string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s", org, platform, domain, system, entityType, instance)
}

// BuildSystemSlug converts a canonical path or module string into a NATS-safe
// system segment by replacing forward slashes with hyphens.
//
// The spec table (Section 4.1) states "dots/slashes replaced with dashes", but
// the canonical example — "github.com/acme/gcs" → "github.com-acme-gcs" —
// shows that only slashes become hyphens while dots in the hostname are preserved.
// This implementation follows the example rather than the ambiguous table prose.
//
// Examples:
//
//	"github.com/acme/gcs"     → "github.com-acme-gcs"
//	"stdlib/net/http"         → "stdlib-net-http"
//	"pkg.go.dev"              → "pkg.go.dev"  (no slashes, unchanged)
func BuildSystemSlug(canonicalPath string) string {
	return strings.ReplaceAll(canonicalPath, "/", "-")
}

// CanonicalizeURL normalizes a URL for use in deterministic entity ID construction.
// Rules applied (per spec Section 4.3):
//   - Lowercase scheme and host
//   - Strip fragment (#...)
//   - Strip query parameters (?...)
//   - Remove trailing slashes from the path
func CanonicalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Separate scheme from the rest.
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		// Not a full URL; do minimal cleanup.
		return strings.TrimRight(rawURL, "/")
	}

	scheme := strings.ToLower(rawURL[:schemeEnd])
	rest := rawURL[schemeEnd+3:] // everything after "://"

	// Separate authority+path from fragment.
	if i := strings.Index(rest, "#"); i >= 0 {
		rest = rest[:i]
	}

	// Separate authority+path from query.
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}

	// Split host from path on first slash.
	var host, path string
	if i := strings.Index(rest, "/"); i >= 0 {
		host = rest[:i]
		path = rest[i:]
	} else {
		host = rest
		path = ""
	}

	host = strings.ToLower(host)

	// Remove trailing slashes from path.
	path = strings.TrimRight(path, "/")

	return scheme + "://" + host + path
}

// IsPublicNamespace reports whether org is the reserved public namespace.
// The public namespace is used for open-source entities that must have
// deterministic identity across all SemSource instances worldwide.
func IsPublicNamespace(org string) bool {
	return org == "public"
}

// natsKVForbidden lists characters disallowed in NATS KV keys.
// NATS KV keys may not contain spaces, wildcards (* or >), or forward slashes.
const natsKVForbidden = " */>\\"

// ValidateNATSKVKey returns an error if id contains characters that are
// disallowed in a NATS KV key. Valid keys are non-empty strings containing
// only printable ASCII except space, *, >, /, and \.
func ValidateNATSKVKey(id string) error {
	if id == "" {
		return fmt.Errorf("NATS KV key must not be empty")
	}
	for _, ch := range id {
		if strings.ContainsRune(natsKVForbidden, ch) {
			return fmt.Errorf("NATS KV key %q contains forbidden character %q", id, ch)
		}
		// Guard against non-printable characters.
		if ch < 0x21 || ch > 0x7E {
			return fmt.Errorf("NATS KV key %q contains non-printable character %U", id, ch)
		}
	}
	return nil
}
