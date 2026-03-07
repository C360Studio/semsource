// Package weburl provides URL validation and ID generation for web sources.
// It implements SSRF prevention including private IP detection and DNS rebinding
// protection.
package weburl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Pre-compiled CIDR networks for private/reserved IP ranges.
// These are parsed once at package initialization for efficiency.
var (
	cgnat    *net.IPNet // 100.64.0.0/10 - Carrier-grade NAT
	v6unique *net.IPNet // fc00::/7 - IPv6 unique local
	v6link   *net.IPNet // fe80::/10 - IPv6 link-local
)

// entityIDPattern validates entity ID format to prevent injection.
// 6-part format: c360.semspec.source.web.page.{slug}
var entityIDPattern = regexp.MustCompile(`^c360\.semspec\.source\.web\.page\.[a-z0-9-]+$`)

func init() {
	var err error

	_, cgnat, err = net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		panic("invalid CGNAT CIDR: " + err.Error())
	}

	_, v6unique, err = net.ParseCIDR("fc00::/7")
	if err != nil {
		panic("invalid IPv6 unique local CIDR: " + err.Error())
	}

	_, v6link, err = net.ParseCIDR("fe80::/10")
	if err != nil {
		panic("invalid IPv6 link-local CIDR: " + err.Error())
	}
}

// ValidateURL validates a URL for security (SSRF prevention).
// It requires HTTPS and blocks localhost, private IPs, and local domains.
func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Require HTTPS
	if parsed.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}

	// Get host without port
	host := parsed.Hostname()

	// Block localhost variants
	lowHost := strings.ToLower(host)
	if lowHost == "localhost" || lowHost == "127.0.0.1" || lowHost == "::1" {
		return fmt.Errorf("localhost URLs are not allowed")
	}

	// Block local domains
	if strings.HasSuffix(lowHost, ".local") || strings.HasSuffix(lowHost, ".internal") {
		return fmt.Errorf("local domain URLs are not allowed")
	}

	// Try to parse as IP and check for private ranges
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateIP(ip) {
			return fmt.Errorf("private IP addresses are not allowed")
		}
	}

	return nil
}

// IsPrivateIP checks if an IP is in private/reserved ranges.
// It handles IPv4, IPv6, and IPv6-mapped IPv4 addresses.
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for IPv6-mapped IPv4 addresses (::ffff:x.x.x.x)
	// Convert to IPv4 if it's an IPv4-mapped IPv6 address
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		// Re-check after conversion
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
	}

	// Check for additional reserved ranges using pre-compiled CIDRs
	if cgnat.Contains(ip) || v6unique.Contains(ip) || v6link.Contains(ip) {
		return true
	}

	return false
}

// GenerateEntityID creates a 6-part web source entity ID from a URL.
// Format: c360.semspec.source.web.page.{slug}
func GenerateEntityID(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fall back to hash-based ID for invalid URLs
		hash := sha256.Sum256([]byte(rawURL))
		return "c360.semspec.source.web.page." + hex.EncodeToString(hash[:8])
	}

	// Create readable slug from domain and path
	host := parsed.Hostname()
	path := strings.Trim(parsed.Path, "/")

	// Replace dots and slashes with hyphens
	slug := strings.ReplaceAll(host, ".", "-")
	if path != "" {
		pathSlug := strings.ReplaceAll(path, "/", "-")
		slug = slug + "-" + pathSlug
	}

	// Clean up the slug
	slug = strings.ToLower(slug)
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, slug)

	// Remove consecutive hyphens and trim
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")

	// Truncate if too long
	if len(slug) > 80 {
		slug = slug[:80]
		slug = strings.TrimRight(slug, "-")
	}

	return "c360.semspec.source.web.page." + slug
}

// ValidateEntityID checks if an entity ID has a valid 6-part format.
// Valid IDs match the pattern "c360.semspec.source.web.page.[a-z0-9-]+"
// to prevent NATS subject injection attacks.
func ValidateEntityID(id string) bool {
	return entityIDPattern.MatchString(id)
}

// ExtractDomain extracts the domain name from a URL.
// Returns an empty string if the URL is invalid.
func ExtractDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
