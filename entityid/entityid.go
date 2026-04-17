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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
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

// maxSystemSlugLen caps the system slug length. Entity IDs have a 255-char
// limit and six dot-separated segments — keeping the system segment compact
// leaves room for the other five.
const maxSystemSlugLen = 80

// SystemSlug converts a canonical path, URL, or module string into a
// graph-ingest-safe system segment. URLs have their scheme stripped; forward
// slashes, colons, and dots are replaced with hyphens so the result is a
// single [a-zA-Z0-9_-] segment. Absolute filesystem paths are reduced to
// their base name so that deep temp-directory hierarchies don't bloat entity
// IDs. A safety cap truncates slugs that still exceed maxSystemSlugLen.
//
// Examples:
//
//	"github.com/acme/gcs"                          → "github-com-acme-gcs"
//	"https://github.com/opensensorhub/osh-core"    → "github-com-opensensorhub-osh-core"
//	"stdlib/net/http"                               → "stdlib-net-http"
//	"pkg.go.dev"                                    → "pkg-go-dev"
//	"/tmp/workspace/github-com-acme-gcs"           → "github-com-acme-gcs"
func SystemSlug(canonicalPath string) string {
	// Strip URL scheme if present so "https://host/path" becomes "host/path".
	if parsed, err := url.Parse(canonicalPath); err == nil && parsed.Host != "" {
		canonicalPath = parsed.Host + parsed.Path
	}
	canonicalPath = strings.TrimSuffix(canonicalPath, ".git")
	canonicalPath = strings.TrimPrefix(canonicalPath, "./")

	// For absolute filesystem paths, use only the base name. Workspace-
	// cloned repos already have a meaningful slug as their directory name
	// (e.g. "github-com-opensensorhub-osh-core"), so the parent hierarchy
	// (temp dirs, user home, etc.) is noise that inflates entity IDs.
	if filepath.IsAbs(canonicalPath) {
		canonicalPath = filepath.Base(canonicalPath)
	}

	// Replace segment-breaking characters with hyphens. Dots must be
	// translated too — graph-ingest treats '.' as the segment separator
	// for the 6-part entity ID, so a host like "pkg.go.dev" would
	// inflate the part count.
	slug := strings.ReplaceAll(canonicalPath, "/", "-")
	slug = strings.ReplaceAll(slug, ":", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	// Safety cap: truncate and append a short content hash when the slug
	// still exceeds the limit (e.g. extremely long directory names).
	if len(slug) > maxSystemSlugLen {
		h := sha256.Sum256([]byte(slug))
		suffix := hex.EncodeToString(h[:3]) // 6 hex chars
		slug = slug[:maxSystemSlugLen-7] + "-" + suffix
	}

	return slug
}

// BranchScopedSlug appends a branch qualifier to a system slug with a hyphen
// separator so the combined string remains a single graph-ingest segment
// (matching [a-zA-Z0-9_-]). Returns the unmodified slug when branchSlug is
// empty (single-branch mode, backward compatible).
//
// Example:
//
//	BranchScopedSlug("github-com-acme-repo", "scenario-auth-flow")
//	  → "github-com-acme-repo-scenario-auth-flow"
//	BranchScopedSlug("github-com-acme-repo", "")
//	  → "github-com-acme-repo"
func BranchScopedSlug(systemSlug, branchSlug string) string {
	if branchSlug == "" {
		return systemSlug
	}
	return systemSlug + "-" + branchSlug
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

// maxInstanceLen caps the sanitized instance segment. Combined with the other
// five segments and their dot separators, keeps total entity IDs well under
// graph-ingest's 255-char limit.
const maxInstanceLen = 60

// SanitizeInstance converts an arbitrary string into a segment that satisfies
// the graph-ingest entity-ID regex ^[a-zA-Z0-9][a-zA-Z0-9_-]*$.
//
// Runes outside [a-zA-Z0-9_-] are replaced with '-', consecutive dashes are
// collapsed, and any non-alphanumeric characters are trimmed from both ends
// so the result starts (and ends) with an alphanumeric. Inputs that sanitize
// to empty fall back to a short SHA-256 hash of the original so IDs remain
// deterministic. Overlong results are truncated with an 8-char content-hash
// suffix to preserve uniqueness across near-identical long inputs.
//
// Case is preserved so human-readable names like "featureAuth" stay legible.
func SanitizeInstance(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isAllowedInstanceRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.TrimFunc(out, func(r rune) bool { return !isASCIIAlnum(r) })

	if out == "" {
		return shortHash(s)
	}
	if len(out) > maxInstanceLen {
		suffix := shortHash(s)
		prefix := strings.TrimRightFunc(out[:maxInstanceLen-len(suffix)-1],
			func(r rune) bool { return !isASCIIAlnum(r) })
		if prefix == "" {
			return suffix
		}
		return prefix + "-" + suffix
	}
	return out
}

func isAllowedInstanceRune(r rune) bool {
	return isASCIIAlnum(r) || r == '-' || r == '_'
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
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

// Parts extracts the domain and entity type segments from a 6-part entity ID.
// Returns empty strings if the ID has fewer than 6 dot-separated parts.
func Parts(id string) (domain, entityType string) {
	parts := strings.SplitN(id, ".", 6)
	if len(parts) < 6 {
		return "", ""
	}
	return parts[2], parts[4]
}
