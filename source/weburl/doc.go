// Package weburl provides URL validation and entity ID generation for web sources.
//
// # Overview
//
// This package implements security validation for web URLs to prevent SSRF
// (Server-Side Request Forgery) attacks, and provides consistent entity ID
// generation from URLs.
//
// # URL Validation
//
// The ValidateURL function checks URLs against multiple security criteria:
//
//   - Requires HTTPS scheme
//   - Blocks localhost variants (localhost, 127.0.0.1, ::1)
//   - Blocks local domains (.local, .internal)
//   - Blocks private IP ranges (RFC 1918, CGNAT, link-local)
//
// # IP Address Handling
//
// The IsPrivateIP function detects private/reserved IP addresses including:
//
//   - IPv4 private ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
//   - IPv4 loopback (127.0.0.0/8)
//   - IPv4 link-local (169.254.0.0/16)
//   - CGNAT range (100.64.0.0/10)
//   - IPv6 loopback (::1)
//   - IPv6 unique local (fc00::/7)
//   - IPv6 link-local (fe80::/10)
//   - IPv6-mapped IPv4 addresses (::ffff:x.x.x.x)
//
// CIDRs are pre-compiled at package initialization for efficiency.
//
// # Entity ID Generation
//
// GenerateEntityID creates readable, URL-safe entity IDs from URLs:
//
//	https://example.com/docs/guide → source.web.example-com-docs-guide
//
// IDs are:
//   - Lowercase with hyphens as separators
//   - Truncated to 80 characters maximum
//   - Deterministic (same URL always produces same ID)
//
// For invalid URLs, a hash-based fallback ID is generated.
//
// # Entity ID Validation
//
// ValidateEntityID checks that an entity ID matches the expected format,
// preventing NATS subject injection attacks.
//
// # Usage
//
//	import "github.com/c360studio/semsource/source/weburl"
//
//	// Validate a URL
//	if err := weburl.ValidateURL("https://example.com"); err != nil {
//	    return err
//	}
//
//	// Generate entity ID
//	id := weburl.GenerateEntityID("https://example.com/docs")
//	// Returns: "source.web.example-com-docs"
//
//	// Check IP address
//	ip := net.ParseIP("192.168.1.1")
//	if weburl.IsPrivateIP(ip) {
//	    // Handle private IP
//	}
package weburl
