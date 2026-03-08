package audio

import (
	"context"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// AudioEntity is a fully-typed audio entity that produces triples using
// canonical vocabulary predicates, bypassing the normalizer entirely.
type AudioEntity struct {
	ID         string
	FilePath   string
	MimeType   string
	FileHash   string
	FileSize   int64
	Format     string
	Duration   float64
	Codec      string
	Bitrate    int
	SampleRate int
	Channels   int
	BitDepth   int
	StorageRef string
	System     string
	Org        string
	IndexedAt  time.Time
}

// Triples converts the AudioEntity to a slice of message.Triple for graph storage.
func (e *AudioEntity) Triples() []message.Triple {
	now := e.IndexedAt
	src := entityid.PlatformSemsource
	return []message.Triple{
		{Subject: e.ID, Predicate: source.MediaType, Object: "audio", Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFilePath, Object: e.FilePath, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaMimeType, Object: e.MimeType, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileHash, Object: e.FileHash, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFileSize, Object: e.FileSize, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaFormat, Object: e.Format, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaDuration, Object: e.Duration, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaCodec, Object: e.Codec, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaBitrate, Object: e.Bitrate, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaSampleRate, Object: e.SampleRate, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaChannels, Object: e.Channels, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaBitDepth, Object: e.BitDepth, Source: src, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.MediaStorageRef, Object: e.StorageRef, Source: src, Timestamp: now, Confidence: 1.0},
	}
}

// EntityState converts the AudioEntity to a handler.EntityState for graph publication.
func (e *AudioEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// audioEntityFromRaw converts a RawEntity produced by ingestFile into a typed
// AudioEntity using canonical vocabulary predicates. The system slug is taken
// from RawEntity.System (already set by ingestFile via slugify(root)).
func audioEntityFromRaw(org string, r handler.RawEntity, now time.Time) *AudioEntity {
	ae := &AudioEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "media", r.System, "audio", r.Instance),
		System:    r.System,
		Org:       org,
		IndexedAt: now,
	}
	if v, ok := r.Properties["file_path"].(string); ok {
		ae.FilePath = v
	}
	if v, ok := r.Properties["mime_type"].(string); ok {
		ae.MimeType = v
	}
	if v, ok := r.Properties["file_hash"].(string); ok {
		ae.FileHash = v
	}
	if v, ok := r.Properties["file_size"].(int64); ok {
		ae.FileSize = v
	}
	if v, ok := r.Properties["format"].(string); ok {
		ae.Format = v
	}
	if v, ok := r.Properties["duration"].(float64); ok {
		ae.Duration = v
	}
	if v, ok := r.Properties["codec"].(string); ok {
		ae.Codec = v
	}
	if v, ok := r.Properties["bitrate"].(int); ok {
		ae.Bitrate = v
	}
	if v, ok := r.Properties["sample_rate"].(int); ok {
		ae.SampleRate = v
	}
	if v, ok := r.Properties["channels"].(int); ok {
		ae.Channels = v
	}
	if v, ok := r.Properties["bit_depth"].(int); ok {
		ae.BitDepth = v
	}
	if v, ok := r.Properties["storage_ref"].(string); ok {
		ae.StorageRef = v
	}
	return ae
}

// IngestEntityStates walks every path in cfg, reads each supported audio file,
// and returns fully-typed EntityState values with vocabulary-predicate triples —
// bypassing the normalizer entirely. The org parameter is the organisation
// namespace used in the 6-part entity ID.
func (h *AudioHandler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	rawEntities, err := h.Ingest(ctx, cfg)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	states := make([]*handler.EntityState, 0, len(rawEntities))
	for _, r := range rawEntities {
		ae := audioEntityFromRaw(org, r, now)
		states = append(states, ae.EntityState())
	}
	return states, nil
}
