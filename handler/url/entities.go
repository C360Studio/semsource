package urlhandler

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
)

// PageEntity is a fully-typed URL page entity that builds triples directly
// using canonical vocabulary predicates, bypassing the normalizer.
type PageEntity struct {
	ID          string
	RawURL      string
	ContentType string
	ETag        string
	ContentHash string
	System      string
	Org         string
	IndexedAt   time.Time
}

// newPageEntity constructs a PageEntity with a deterministic 6-part ID.
// The instance is derived from the canonical URL — identical to urlInstanceID
// so that normalizer and direct paths produce the same entity ID.
func newPageEntity(org, rawURL, contentType, etag, contentHash string, indexedAt time.Time) *PageEntity {
	system := domainSlug(rawURL)
	instance := urlInstanceID(rawURL)
	return &PageEntity{
		ID:          entityid.Build(org, entityid.PlatformSemsource, "web", system, "page", instance),
		RawURL:      rawURL,
		ContentType: contentType,
		ETag:        etag,
		ContentHash: contentHash,
		System:      system,
		Org:         org,
		IndexedAt:   indexedAt,
	}
}

// Triples converts the PageEntity to a slice of message.Triple using canonical
// vocabulary predicates from source/vocabulary.
func (e *PageEntity) Triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.WebType, Object: "web", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.WebURL, Object: e.RawURL, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.WebContentType, Object: e.ContentType, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.WebDomain, Object: e.System, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	if e.ETag != "" {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.WebETag,
			Object:     e.ETag,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	if e.ContentHash != "" {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  source.WebContentHash,
			Object:     e.ContentHash,
			Source:     entityid.PlatformSemsource,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

// EntityState converts the PageEntity to a handler.EntityState for direct graph publication.
func (e *PageEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// IngestEntityStates fetches the URL and returns a fully-typed entity state that
// embeds vocabulary-predicate triples directly, bypassing the normalizer entirely.
// org is the organisation namespace (e.g. "acme") used in the 6-part entity ID.
func (h *URLHandler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	rawURL := cfg.GetURL()
	result, err := h.fetcher.Fetch(ctx, rawURL, "")
	if err != nil {
		return nil, fmt.Errorf("urlhandler: ingest entity states %s: %w", rawURL, err)
	}

	now := time.Now().UTC()
	hash := ""
	if len(result.Body) > 0 {
		hash = contentHash(result.Body)
	}

	pe := newPageEntity(org, rawURL, result.ContentType, result.ETag, hash, now)
	return []*handler.EntityState{pe.EntityState()}, nil
}

// buildPageEntityState constructs a PageEntity from a FetchResult and returns
// its EntityState. Used in Watch to populate event.EntityStates.
func (h *URLHandler) buildPageEntityState(rawURL string, result *FetchResult, org string, now time.Time) *handler.EntityState {
	hash := ""
	if len(result.Body) > 0 {
		hash = contentHash(result.Body)
	}
	pe := newPageEntity(org, rawURL, result.ContentType, result.ETag, hash, now)
	return pe.EntityState()
}
