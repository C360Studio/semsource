// Package urlhandler provides a SourceHandler for HTTP/S URL sources.
// It uses SafeFetcher for SSRF-safe content retrieval with ETag-based
// conditional fetching to avoid re-emitting unchanged content.
package urlhandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/source/weburl"
)

const (
	// defaultPollInterval is used when no poll_interval is configured.
	defaultPollInterval = 5 * time.Minute
)

// URLSourceConfig extends handler.SourceConfig with URL-specific configuration.
// Handlers that return a poll interval string can implement this interface;
// it is optional.
type URLSourceConfig interface {
	handler.SourceConfig
	GetPollInterval() string
}

// URLHandler implements handler.SourceHandler for HTTP/S URL sources.
type URLHandler struct {
	fetcher *SafeFetcher
	logger  *slog.Logger
	// org is the organisation namespace used when building typed EntityState
	// values via IngestEntityStates and Watch. When empty, EntityStates are not
	// populated on watch events.
	org string
}

// New creates a URLHandler with a default SSRF-safe HTTP client.
// logger may be nil; slog.Default() is used.
func New(logger *slog.Logger) *URLHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLHandler{
		fetcher: NewSafeFetcher(nil),
		logger:  logger,
	}
}

// NewWithOrg creates a URLHandler that will populate EntityStates on watch
// events using the given org namespace.
func NewWithOrg(logger *slog.Logger, org string) *URLHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLHandler{
		fetcher: NewSafeFetcher(nil),
		logger:  logger,
		org:     org,
	}
}

// NewWithClient creates a URLHandler using the provided HTTP client with URL
// validation disabled. Intended for use in tests (e.g. httptest.NewTLSServer).
func NewWithClient(logger *slog.Logger, client *http.Client) *URLHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLHandler{
		fetcher: newTestFetcher(client),
		logger:  logger,
	}
}

// SourceType implements handler.SourceHandler.
func (h *URLHandler) SourceType() string { return handler.SourceTypeURL }

// Supports implements handler.SourceHandler.
// Returns true for cfg.GetType() == "url" with a valid HTTPS URL.
func (h *URLHandler) Supports(cfg handler.SourceConfig) bool {
	if cfg.GetType() != handler.SourceTypeURL {
		return false
	}
	rawURL := cfg.GetURL()
	if rawURL == "" {
		return false
	}
	return weburl.ValidateURL(rawURL) == nil
}

// Ingest implements handler.SourceHandler.
// Fetches the URL and returns a single page entity with content metadata.
func (h *URLHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	rawURL := cfg.GetURL()
	result, err := h.fetcher.Fetch(ctx, rawURL, "")
	if err != nil {
		return nil, fmt.Errorf("urlhandler: ingest %s: %w", rawURL, err)
	}

	entity := h.buildPageEntity(rawURL, result)
	return []handler.RawEntity{entity}, nil
}

// Watch implements handler.SourceHandler.
// Returns nil, nil when watching is disabled. Otherwise starts a polling
// goroutine and returns a channel of ChangeEvents.
func (h *URLHandler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	interval := parsePollInterval(cfg)
	out := make(chan handler.ChangeEvent, 64)
	go h.pollLoop(ctx, cfg.GetURL(), interval, out)
	return out, nil
}

// pollLoop repeatedly fetches the URL at interval. It emits a ChangeEvent
// when the content hash changes. The first successful fetch always emits
// an event (OperationCreate).
func (h *URLHandler) pollLoop(ctx context.Context, rawURL string, interval time.Duration, out chan<- handler.ChangeEvent) {
	defer close(out)

	var lastHash string
	var lastETag string

	// Initial fetch
	result, err := h.fetcher.Fetch(ctx, rawURL, "")
	if err != nil {
		h.logger.Warn("urlhandler: initial fetch failed", "url", rawURL, "error", err)
	} else {
		lastETag = result.ETag
		if len(result.Body) > 0 {
			now := time.Now()
			lastHash = contentHash(result.Body)
			entity := h.buildPageEntity(rawURL, result)
			ev := handler.ChangeEvent{
				Path:      rawURL,
				Operation: handler.OperationCreate,
				Timestamp: now,
				Entities:  []handler.RawEntity{entity},
			}
			// Populate EntityStates for the normalizer-free processor path.
			if h.org != "" {
				ev.EntityStates = []*handler.EntityState{h.buildPageEntityState(rawURL, result, h.org, now.UTC())}
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := h.fetcher.Fetch(ctx, rawURL, lastETag)
			if err != nil {
				h.logger.Warn("urlhandler: poll fetch failed", "url", rawURL, "error", err)
				continue
			}

			// 304 Not Modified — skip
			if result.StatusCode == 304 {
				continue
			}

			if len(result.Body) == 0 {
				continue
			}

			newHash := contentHash(result.Body)
			if newHash == lastHash {
				// Content hash identical — no real change
				lastETag = result.ETag
				continue
			}

			lastHash = newHash
			lastETag = result.ETag

			now := time.Now()
			entity := h.buildPageEntity(rawURL, result)
			ev := handler.ChangeEvent{
				Path:      rawURL,
				Operation: handler.OperationModify,
				Timestamp: now,
				Entities:  []handler.RawEntity{entity},
			}
			// Populate EntityStates for the normalizer-free processor path.
			if h.org != "" {
				ev.EntityStates = []*handler.EntityState{h.buildPageEntityState(rawURL, result, h.org, now.UTC())}
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}

// buildPageEntity constructs a RawEntity for a fetched URL page.
func (h *URLHandler) buildPageEntity(rawURL string, result *FetchResult) handler.RawEntity {
	system := domainSlug(rawURL)
	instance := urlInstanceID(rawURL)
	props := map[string]any{
		"url":          rawURL,
		"content_type": result.ContentType,
	}
	if result.ETag != "" {
		props["etag"] = result.ETag
	}
	if len(result.Body) > 0 {
		props["content_hash"] = contentHash(result.Body)
		props["content_size"] = len(result.Body)
	}

	return handler.RawEntity{
		SourceType: handler.SourceTypeURL,
		Domain:     handler.DomainWeb,
		System:     system,
		EntityType: "page",
		Instance:   instance,
		Properties: props,
	}
}

// urlInstanceID builds a deterministic instance ID from the canonical URL.
// It uses the first 8 hex chars of SHA-256(canonical_url).
func urlInstanceID(rawURL string) string {
	canonical := canonicalizeURL(rawURL)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])[:8]
}

// canonicalizeURL returns a normalised URL: lowercase scheme+host, no
// trailing slash, no fragment, no query (unless semantically necessary).
func canonicalizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.ToLower(rawURL)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/"
	}
	parsed.Path = path
	return parsed.String()
}

// domainSlug extracts the domain from a URL and converts it to a NATS-safe slug.
func domainSlug(rawURL string) string {
	domain := weburl.ExtractDomain(rawURL)
	if domain == "" {
		return "unknown"
	}
	return strings.ReplaceAll(strings.ToLower(domain), ".", "-")
}

// contentHash returns the hex-encoded SHA-256 of content.
func contentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// parsePollInterval extracts the poll interval from cfg. Falls back to
// defaultPollInterval if not set or unparseable.
func parsePollInterval(cfg handler.SourceConfig) time.Duration {
	if usc, ok := cfg.(URLSourceConfig); ok {
		if s := usc.GetPollInterval(); s != "" {
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultPollInterval
}
