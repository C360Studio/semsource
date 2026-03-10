package image

import (
	"context"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// Entity is a fully-typed image entity that produces triples using
// canonical vocabulary predicates, bypassing the normalizer entirely.
type Entity struct {
	ID         string
	FilePath   string
	MimeType   string
	FileHash   string
	FileSize   int64
	Format     string
	Width      int
	Height     int
	StorageRef string
	ThumbRef   string
	System     string
	Org        string
	IndexedAt  time.Time
}

// Triples converts the Entity to a slice of message.Triple for graph storage.
func (e *Entity) Triples() []message.Triple {
	now := e.IndexedAt
	src := entityid.PlatformSemsource
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.MediaType, Object: "image", Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFilePath, Object: e.FilePath, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaMimeType, Object: e.MimeType, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileHash, Object: e.FileHash, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileSize, Object: e.FileSize, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFormat, Object: e.Format, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaWidth, Object: e.Width, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaHeight, Object: e.Height, Source: src, Timestamp: now, Confidence: 1.0},
	}
	if e.StorageRef != "" {
		triples = append(triples, message.Triple{
			Subject: e.ID, Predicate: source.MediaStorageRef, Object: e.StorageRef,
			Source: src, Timestamp: now, Confidence: 1.0,
		})
	}
	if e.ThumbRef != "" {
		triples = append(triples, message.Triple{
			Subject: e.ID, Predicate: source.MediaThumbnailRef, Object: e.ThumbRef,
			Source: src, Timestamp: now, Confidence: 1.0,
		})
	}
	return triples
}

// EntityState converts the Entity to a handler.EntityState for graph publication.
func (e *Entity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// imageEntityFromRaw converts a RawEntity produced by ingestFile into a typed
// Entity using canonical vocabulary predicates. The system slug is derived
// from the root path that produced the entity (stored in RawEntity.System).
func imageEntityFromRaw(org string, r handler.RawEntity, now time.Time) *Entity {
	ie := &Entity{
		// RawEntity.System already holds slugify(root) set by ingestFile.
		ID:        entityid.Build(org, entityid.PlatformSemsource, "media", r.System, "image", r.Instance),
		System:    r.System,
		Org:       org,
		IndexedAt: now,
	}
	if v, ok := r.Properties["file_path"].(string); ok {
		ie.FilePath = v
	}
	if v, ok := r.Properties["mime_type"].(string); ok {
		ie.MimeType = v
	}
	if v, ok := r.Properties["file_hash"].(string); ok {
		ie.FileHash = v
	}
	if v, ok := r.Properties["file_size"].(int64); ok {
		ie.FileSize = v
	}
	if v, ok := r.Properties["format"].(string); ok {
		ie.Format = v
	}
	if v, ok := r.Properties["width"].(int); ok {
		ie.Width = v
	}
	if v, ok := r.Properties["height"].(int); ok {
		ie.Height = v
	}
	if v, ok := r.Properties["storage_ref"].(string); ok {
		ie.StorageRef = v
	}
	if v, ok := r.Properties["thumbnail_ref"].(string); ok {
		ie.ThumbRef = v
	}
	return ie
}

// IngestEntityStates walks every path in cfg, reads each supported image file,
// and returns fully-typed EntityState values with vocabulary-predicate triples —
// bypassing the normalizer entirely. The org parameter is the organisation
// namespace used in the 6-part entity ID.
func (h *Handler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	rawEntities, err := h.Ingest(ctx, cfg)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	states := make([]*handler.EntityState, 0, len(rawEntities))
	for _, r := range rawEntities {
		ie := imageEntityFromRaw(org, r, now)
		states = append(states, ie.EntityState())
	}
	return states, nil
}
