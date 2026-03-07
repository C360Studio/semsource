package weburl

import (
	"net"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid https URL",
			url:     "https://go.dev/doc/effective_go",
			wantErr: false,
		},
		{
			name:    "http URL rejected",
			url:     "http://example.com",
			wantErr: true,
		},
		{
			name:    "localhost rejected",
			url:     "https://localhost:8080",
			wantErr: true,
		},
		{
			name:    "127.0.0.1 rejected",
			url:     "https://127.0.0.1/path",
			wantErr: true,
		},
		{
			name:    ".local domain rejected",
			url:     "https://myserver.local/api",
			wantErr: true,
		},
		{
			name:    ".internal domain rejected",
			url:     "https://app.internal/api",
			wantErr: true,
		},
		{
			name:    "private IP 192.168.x.x rejected",
			url:     "https://192.168.1.1/path",
			wantErr: true,
		},
		{
			name:    "private IP 10.x.x.x rejected",
			url:     "https://10.0.0.1/path",
			wantErr: true,
		},
		{
			name:    "private IP 172.16.x.x rejected",
			url:     "https://172.16.0.1/path",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		// IPv4 private ranges
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true}, // IPv4 link-local

		// IPv4 public
		{"8.8.8.8", false},
		{"1.1.1.1", false},

		// CGNAT
		{"100.64.0.1", true},
		{"100.127.255.255", true},

		// IPv6
		{"::1", true},                // IPv6 loopback
		{"::ffff:192.168.1.1", true}, // IPv6-mapped private IPv4
		{"::ffff:127.0.0.1", true},   // IPv6-mapped loopback
		{"::ffff:8.8.8.8", false},    // IPv6-mapped public IPv4
		{"fe80::1", true},            // IPv6 link-local
		{"fc00::1", true},            // IPv6 unique local
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.ip)
			}
			got := IsPrivateIP(ip)
			if got != tt.expected {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.expected)
			}
		})
	}
}

func TestGenerateEntityID(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "simple domain",
			url:      "https://example.com",
			expected: "c360.semspec.source.web.page.example-com",
		},
		{
			name:     "domain with path",
			url:      "https://example.com/docs/guide",
			expected: "c360.semspec.source.web.page.example-com-docs-guide",
		},
		{
			name:     "subdomain",
			url:      "https://docs.example.com/api",
			expected: "c360.semspec.source.web.page.docs-example-com-api",
		},
		{
			name:     "github docs",
			url:      "https://github.com/user/repo/blob/main/README.md",
			expected: "c360.semspec.source.web.page.github-com-user-repo-blob-main-readme-md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateEntityID(tt.url)
			if got != tt.expected {
				t.Errorf("GenerateEntityID(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestGenerateEntityID_InvalidURL(t *testing.T) {
	// Invalid URLs should return hash-based IDs
	id := GenerateEntityID("not a valid url ://")
	if !ValidateEntityID(id) {
		t.Errorf("GenerateEntityID for invalid URL returned invalid ID: %s", id)
	}
	if len(id) < len("c360.semspec.source.web.page.") {
		t.Errorf("GenerateEntityID for invalid URL returned too short ID: %s", id)
	}
}

func TestValidateEntityID(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"c360.semspec.source.web.page.example-com", true},
		{"c360.semspec.source.web.page.docs-go-dev-effective-go", true},
		{"c360.semspec.source.web.page.123-abc-456", true},
		{"c360.semspec.source.web.page.", false},                // empty slug
		{"c360.semspec.source.web.page.ABC", false},             // uppercase
		{"c360.semspec.source.web.page.test_underscore", false}, // underscore
		{"c360.semspec.source.web.page.test.dot", false},        // dot in slug
		{"c360.semspec.source.web.page.test>wildcard", false},   // NATS wildcard
		{"c360.semspec.source.web.page.test*star", false},       // NATS wildcard
		{"source.document.test", false},                         // wrong prefix
		{"", false},                                             // empty
		{"c360.semspec.source.web.page.a", true},                // single char
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := ValidateEntityID(tt.id)
			if got != tt.expected {
				t.Errorf("ValidateEntityID(%q) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"https://docs.example.com", "docs.example.com"},
		{"https://example.com:8080/path", "example.com"},
		{"invalid-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := ExtractDomain(tt.url)
			if got != tt.expected {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}
