package video

import (
	"context"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// VideoEntity is a fully-typed video entity that produces triples using
// canonical vocabulary predicates, bypassing the normalizer entirely.
type VideoEntity struct {
	ID            string
	FilePath      string
	MimeType      string
	FileHash      string
	FileSize      int64
	Format        string
	Duration      float64
	FrameRate     float64
	Width         int
	Height        int
	Codec         string
	Bitrate       int
	KeyframeCount int
	StorageRef    string
	System        string
	Org           string
	IndexedAt     time.Time
}

// Triples converts the VideoEntity to a slice of message.Triple for graph storage.
func (e *VideoEntity) Triples() []message.Triple {
	now := e.IndexedAt
	src := entityid.PlatformSemsource
	return []message.Triple{
		{Subject: e.ID, Predicate: source.MediaType, Object: "video", Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFilePath, Object: e.FilePath, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaMimeType, Object: e.MimeType, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileHash, Object: e.FileHash, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileSize, Object: e.FileSize, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFormat, Object: e.Format, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaDuration, Object: e.Duration, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFrameRate, Object: e.FrameRate, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaWidth, Object: e.Width, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaHeight, Object: e.Height, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaCodec, Object: e.Codec, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaBitrate, Object: e.Bitrate, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaKeyframeCount, Object: e.KeyframeCount, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaStorageRef, Object: e.StorageRef, Source: src, Timestamp: now, Confidence: 1.0},
	}
}

// EntityState converts the VideoEntity to a handler.EntityState for graph publication.
func (e *VideoEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// KeyframeEntity is a fully-typed keyframe entity that produces triples using
// canonical vocabulary predicates, bypassing the normalizer entirely.
type KeyframeEntity struct {
	ID         string
	Timestamp  string // formatted seconds string, e.g. "30s"
	FrameIndex int
	Width      int
	Height     int
	StorageRef string
	// VideoID is the parent video entity ID — stored as a relationship triple.
	VideoID   string
	System    string
	Org       string
	IndexedAt time.Time
}

// Triples converts the KeyframeEntity to a slice of message.Triple for graph storage.
func (e *KeyframeEntity) Triples() []message.Triple {
	now := e.IndexedAt
	src := entityid.PlatformSemsource
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.MediaType, Object: "keyframe", Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaTimestamp, Object: e.Timestamp, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFrameIndex, Object: e.FrameIndex, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaWidth, Object: e.Width, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaHeight, Object: e.Height, Source: src, Timestamp: now, Confidence: 1.0},
	}
	if e.StorageRef != "" {
		triples = append(triples, message.Triple{
			Subject: e.ID, Predicate: source.MediaStorageRef, Object: e.StorageRef,
			Source: src, Timestamp: now, Confidence: 1.0,
		})
	}
	// Relationship triple: this keyframe belongs to its parent video entity.
	if e.VideoID != "" {
		triples = append(triples, message.Triple{
			Subject:    e.ID,
			Predicate:  "source.media.keyframe_of",
			Object:     e.VideoID,
			Source:     src,
			Timestamp:  now,
			Confidence: 1.0,
		})
	}
	return triples
}

// EntityState converts the KeyframeEntity to a handler.EntityState for graph publication.
func (e *KeyframeEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// videoEntityFromRaw converts a RawEntity with EntityType "video" produced by
// ingestFile into a typed VideoEntity using canonical vocabulary predicates.
// The system slug is taken from RawEntity.System (already set by ingestFile).
func videoEntityFromRaw(org string, r handler.RawEntity, now time.Time) *VideoEntity {
	ve := &VideoEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "media", r.System, "video", r.Instance),
		System:    r.System,
		Org:       org,
		IndexedAt: now,
	}
	if v, ok := r.Properties["file_path"].(string); ok {
		ve.FilePath = v
	}
	if v, ok := r.Properties["mime_type"].(string); ok {
		ve.MimeType = v
	}
	if v, ok := r.Properties["file_hash"].(string); ok {
		ve.FileHash = v
	}
	if v, ok := r.Properties["file_size"].(int64); ok {
		ve.FileSize = v
	}
	if v, ok := r.Properties["format"].(string); ok {
		ve.Format = v
	}
	if v, ok := r.Properties["duration"].(float64); ok {
		ve.Duration = v
	}
	if v, ok := r.Properties["frame_rate"].(float64); ok {
		ve.FrameRate = v
	}
	if v, ok := r.Properties["width"].(int); ok {
		ve.Width = v
	}
	if v, ok := r.Properties["height"].(int); ok {
		ve.Height = v
	}
	if v, ok := r.Properties["codec"].(string); ok {
		ve.Codec = v
	}
	if v, ok := r.Properties["bitrate"].(int); ok {
		ve.Bitrate = v
	}
	if v, ok := r.Properties["keyframe_count"].(int); ok {
		ve.KeyframeCount = v
	}
	if v, ok := r.Properties["storage_ref"].(string); ok {
		ve.StorageRef = v
	}
	return ve
}

// keyframeEntityFromRaw converts a RawEntity with EntityType "keyframe" produced
// by ingestFile into a typed KeyframeEntity. videoID is the fully-qualified
// entity ID of the parent video, used to emit the keyframe_of relationship triple.
func keyframeEntityFromRaw(org, videoID string, r handler.RawEntity, now time.Time) *KeyframeEntity {
	ke := &KeyframeEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "media", r.System, "keyframe", r.Instance),
		VideoID:   videoID,
		System:    r.System,
		Org:       org,
		IndexedAt: now,
	}
	if v, ok := r.Properties["timestamp"].(string); ok {
		ke.Timestamp = v
	}
	if v, ok := r.Properties["frame_index"].(int); ok {
		ke.FrameIndex = v
	}
	if v, ok := r.Properties["width"].(int); ok {
		ke.Width = v
	}
	if v, ok := r.Properties["height"].(int); ok {
		ke.Height = v
	}
	if v, ok := r.Properties["storage_ref"].(string); ok {
		ke.StorageRef = v
	}
	return ke
}

// IngestEntityStates walks every path in cfg, extracts video and keyframe
// entities, and returns fully-typed EntityState values with vocabulary-predicate
// triples — bypassing the normalizer entirely. The org parameter is the
// organisation namespace used in the 6-part entity ID.
func (h *VideoHandler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	rawEntities, err := h.Ingest(ctx, cfg)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// First pass: build all video entities and index them by instance so
	// keyframe entities can look up their parent video ID.
	videoIDByInstance := make(map[string]string)
	var states []*handler.EntityState

	for _, r := range rawEntities {
		if r.EntityType == "video" {
			ve := videoEntityFromRaw(org, r, now)
			videoIDByInstance[r.Instance] = ve.ID
			states = append(states, ve.EntityState())
		}
	}

	// Second pass: build keyframe entities and resolve parent video ID.
	for _, r := range rawEntities {
		if r.EntityType != "keyframe" {
			continue
		}
		// The instance for a keyframe is "<videoInstance>-<timestamp>".
		// The video instance is the first 6-char hash prefix before the dash.
		videoInstance := ""
		if len(r.Instance) >= 6 {
			videoInstance = r.Instance[:6]
		}
		videoID := videoIDByInstance[videoInstance]
		ke := keyframeEntityFromRaw(org, videoID, r, now)
		states = append(states, ke.EntityState())
	}

	return states, nil
}
