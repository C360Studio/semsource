package urlhandler

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/c360studio/semspec/source/weburl"
)

const (
	defaultUserAgent      = "SemSource/1.0"
	defaultMaxContentSize = 10 * 1024 * 1024 // 10 MiB
	defaultTimeout        = 30 * time.Second
)

// FetchResult holds the result of a successful HTTP fetch.
type FetchResult struct {
	// Body is the response body. Nil for 304 Not Modified.
	Body []byte
	// ContentType is the Content-Type response header.
	ContentType string
	// ETag is the ETag response header.
	ETag string
	// StatusCode is the HTTP response status code.
	StatusCode int
}

// SafeFetcher performs SSRF-safe HTTP fetches with optional ETag support.
// It validates URLs against the weburl package before dialing.
type SafeFetcher struct {
	client         *http.Client
	maxContentSize int64
	// skipValidation disables weburl.ValidateURL checks. For testing only.
	skipValidation bool
}

// NewSafeFetcher creates a SafeFetcher. If httpClient is nil a default
// SSRF-safe client is constructed. Pass a custom client (e.g. from
// httptest.NewTLSServer) in tests.
func NewSafeFetcher(httpClient *http.Client) *SafeFetcher {
	if httpClient == nil {
		httpClient = newSafeHTTPClient(defaultTimeout)
	}
	return &SafeFetcher{
		client:         httpClient,
		maxContentSize: defaultMaxContentSize,
	}
}

// newTestFetcher creates a SafeFetcher backed by the provided client with URL
// validation disabled. Only for use in tests.
func newTestFetcher(httpClient *http.Client) *SafeFetcher {
	return &SafeFetcher{
		client:         httpClient,
		maxContentSize: defaultMaxContentSize,
		skipValidation: true,
	}
}

// NewTestFetcher is exported for use in test packages. It creates a SafeFetcher
// with URL validation disabled, backed by the provided client.
func NewTestFetcher(httpClient *http.Client) *SafeFetcher {
	return newTestFetcher(httpClient)
}

// Fetch retrieves content from rawURL. etag may be empty to skip conditional
// fetch. Returns a FetchResult; for HTTP 304 the Body field is nil.
func (f *SafeFetcher) Fetch(ctx context.Context, rawURL, etag string) (*FetchResult, error) {
	// Validate before dialing (SSRF prevention + scheme check).
	if !f.skipValidation {
		if err := weburl.ValidateURL(rawURL); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("safefetcher: create request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("safefetcher: fetch: %w", err)
	}
	defer resp.Body.Close()

	result := &FetchResult{
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
		StatusCode:  resp.StatusCode,
	}

	if resp.StatusCode == http.StatusNotModified {
		return result, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("safefetcher: HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	lr := io.LimitReader(resp.Body, f.maxContentSize+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("safefetcher: read body: %w", err)
	}
	if int64(len(body)) > f.maxContentSize {
		return nil, fmt.Errorf("safefetcher: content too large (exceeds %d bytes)", f.maxContentSize)
	}

	result.Body = body
	return result, nil
}

// newSafeHTTPClient builds an http.Client with a DNS-rebinding-resistant dialer.
func newSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	safeDialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address: %w", err)
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS lookup failed: %w", err)
		}

		for _, ipAddr := range ips {
			if weburl.IsPrivateIP(ipAddr.IP) {
				return nil, fmt.Errorf("connection to private IP %s blocked", ipAddr.IP)
			}
		}

		for _, ipAddr := range ips {
			conn, connErr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if connErr == nil {
				return conn, nil
			}
		}
		return nil, fmt.Errorf("failed to connect to any resolved IP")
	}

	transport := &http.Transport{
		DialContext:           safeDialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects (max 5)")
			}
			if err := weburl.ValidateURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
}
