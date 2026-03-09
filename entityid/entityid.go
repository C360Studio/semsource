// Package entityid provides deterministic 6-part entity ID construction
// and validation utilities for the SemSource knowledge graph.
//
// Entity IDs follow the format:
//
//	{org}.{platform}.{domain}.{system}.{type}.{instance}
//
// All IDs must be valid NATS KV keys.
package entityid

import (
	"fmt"
	"net/url"
	"strings"
)

// PlatformSemsource is the platform segment used in all SemSource entity IDs.
const PlatformSemsource = "semsource"

// Build constructs the canonical 6-part entity ID string.
// Format: {org}.{platform}.{domain}.{system}.{type}.{instance}
// All parts must be non-empty — callers are responsible for supplying valid values.
func Build(org, platform, domain, system, entityType, instance string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s", org, platform, domain, system, entityType, instance)
}

// SystemSlug converts a canonical path, URL, or module string into a NATS-safe
// system segment. URLs have their scheme stripped; all forward slashes and
// colons are replaced with hyphens.
//
// Examples:
//
//	"github.com/acme/gcs"                          → "github.com-acme-gcs"
//	"https://github.com/opensensorhub/osh-core"    → "github.com-opensensorhub-osh-core"
//	"stdlib/net/http"                               → "stdlib-net-http"
//	"pkg.go.dev"                                    → "pkg.go.dev"  (no slashes, unchanged)
func SystemSlug(canonicalPath string) string {
	// Strip URL scheme if present so "https://host/path" becomes "host/path".
	if parsed, err := url.Parse(canonicalPath); err == nil && parsed.Host != "" {
		canonicalPath = parsed.Host + parsed.Path
	}
	canonicalPath = strings.TrimSuffix(canonicalPath, ".git")
	canonicalPath = strings.TrimPrefix(canonicalPath, "./")
	slug := strings.ReplaceAll(canonicalPath, "/", "-")
	slug = strings.ReplaceAll(slug, ":", "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// CanonicalizeURL normalizes a URL for use in deterministic entity ID construction.
// Rules applied:
//   - Lowercase scheme and host
//   - Strip fragment (#...)
//   - Strip query parameters (?...)
//   - Remove trailing slashes from the path
func CanonicalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		return strings.TrimRight(rawURL, "/")
	}

	scheme := strings.ToLower(rawURL[:schemeEnd])
	rest := rawURL[schemeEnd+3:]

	if i := strings.Index(rest, "#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}

	var host, path string
	if i := strings.Index(rest, "/"); i >= 0 {
		host = rest[:i]
		path = rest[i:]
	} else {
		host = rest
	}

	host = strings.ToLower(host)
	path = strings.TrimRight(path, "/")

	return scheme + "://" + host + path
}

// IsPublicNamespace reports whether org is the reserved public namespace.
// The public namespace is used for open-source entities that must have
// deterministic identity across all SemSource instances.
func IsPublicNamespace(org string) bool {
	return org == "public"
}

// natsKVForbidden lists characters disallowed in NATS KV keys.
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
		if ch < 0x21 || ch > 0x7E {
			return fmt.Errorf("NATS KV key %q contains non-printable character %U", id, ch)
		}
	}
	return nil
}

// OrgFromID extracts the first segment (org) from a dot-delimited entity ID.
// Returns empty string if the ID is malformed.
func OrgFromID(id string) string {
	if id == "" {
		return ""
	}
	for i, ch := range id {
		if ch == '.' {
			return id[:i]
		}
	}
	return ""
}

// ResolveOrg returns "public" if the given org value is the public namespace,
// otherwise returns the default org.
func ResolveOrg(orgOverride, defaultOrg string) string {
	if IsPublicNamespace(orgOverride) {
		return "public"
	}
	return defaultOrg
}
