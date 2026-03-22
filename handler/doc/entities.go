package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/storage"
)

// Entity is a fully-typed document entity that builds triples directly
// using canonical vocabulary predicates, bypassing the normalizer.
type Entity struct {
	ID          string
	Title       string
	FilePath    string
	MimeType    string
	ContentHash string
	Content     string
	System      string
	Org         string
	IndexedAt   time.Time

	// storageRef is set when content is stored in ObjectStore rather than
	// inline in a triple. When non-nil, Triples() omits the DocContent triple.
	storageRef *message.StorageReference
}

// newEntity constructs a Entity with a deterministic 6-part ID.
// The instance is the first 6 hex chars of the content hash — matching the
// RawEntity path so normalizer and direct paths produce identical IDs.
func newEntity(org, title, filePath, mimeType, contentHash, content, system string, indexedAt time.Time) *Entity {
	instance := contentHash
	if len(instance) > 6 {
		instance = instance[:6]
	}
	return &Entity{
		ID:          entityid.Build(org, entityid.PlatformSemsource, "web", system, "doc", instance),
		Title:       title,
		FilePath:    filePath,
		MimeType:    mimeType,
		ContentHash: contentHash,
		Content:     content,
		System:      system,
		Org:         org,
		IndexedAt:   indexedAt,
	}
}

// Triples converts the Entity to a slice of message.Triple using canonical
// vocabulary predicates from source/vocabulary. When storageRef is set,
// the DocContent triple is omitted — body text lives in ObjectStore.
func (e *Entity) Triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DocType, Object: "document", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocMimeType, Object: e.MimeType, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFileHash, Object: e.ContentHash, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocSummary, Object: e.Title, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	// Only include inline content when not stored externally.
	if e.storageRef == nil {
		triples = append(triples, message.Triple{
			Subject: e.ID, Predicate: source.DocContent, Object: e.Content,
			Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0,
		})
	}
	return triples
}

// EntityState converts the Entity to a handler.EntityState for direct graph publication.
func (e *Entity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:         e.ID,
		Triples:    e.Triples(),
		UpdatedAt:  e.IndexedAt,
		StorageRef: e.storageRef,
	}
}

// IngestEntityStates walks all configured paths and returns fully-typed entity
// states that embed vocabulary-predicate triples directly, bypassing the
// normalizer entirely. org is the organisation namespace (e.g. "acme") used
// in the 6-part entity ID.
func (h *Handler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	roots, err := resolvePaths(cfg)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var states []*handler.EntityState

	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		system := entityid.SystemSlug(root)

		walkErr := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !docExtensions[ext] {
				return nil
			}

			state, err := h.ingestFileEntityState(ctx, path, root, system, org, now)
			if err != nil {
				// Non-fatal: skip unreadable files.
				return nil
			}
			states = append(states, state)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("doc handler: walk %q: %w", root, walkErr)
		}
	}

	return states, nil
}

// ingestFileEntityState reads a single document file and builds a Entity,
// returning its EntityState. When the handler has a store configured and
// content exceeds the threshold, the body is stored as raw bytes in
// ObjectStore and the DocContent triple is omitted.
func (h *Handler) ingestFileEntityState(ctx context.Context, path, root, system, org string, now time.Time) (*handler.EntityState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	hash := contentHash(content)
	relPath, _ := filepath.Rel(root, path)
	title := extractTitle(content, filepath.Base(path))
	mime := mimeForExt(filepath.Ext(path))

	e := newEntity(org, title, relPath, mime, hash, string(content), system, now)

	// Store large content in ObjectStore when configured.
	// Non-fatal: on failure the entity keeps content inline in the DocContent triple.
	_ = storeContentIfLarge(ctx, e, h.store, h.storeBucket, h.contentThreshold)

	return e.EntityState(), nil
}

// storeContentIfLarge stores the entity's content in ObjectStore when it
// exceeds the threshold. On success it sets the entity's storageRef so
// Triples() will omit the inline DocContent triple.
func storeContentIfLarge(ctx context.Context, e *Entity, store storage.Store, bucket string, threshold int) error {
	if store == nil || len(e.Content) <= threshold {
		return nil
	}
	key := fmt.Sprintf("docs/%s/%s/body", e.System, e.ID)
	if err := store.Put(ctx, key, []byte(e.Content)); err != nil {
		return fmt.Errorf("store doc content: %w", err)
	}
	e.storageRef = &message.StorageReference{
		StorageInstance: bucket,
		Key:             key,
		ContentType:     e.MimeType,
		Size:            int64(len(e.Content)),
	}
	e.Content = "" // Release large string for GC; content is in ObjectStore.
	return nil
}

// enrichEventEntityStates re-reads the changed file and populates ev.EntityStates
// alongside ev.Entities, using vocabulary-predicate triples. org is required so
// entity IDs are deterministic. For delete events the file is gone and
// EntityStates remains empty.
func (h *Handler) enrichEventEntityStates(ctx context.Context, ev handler.ChangeEvent, root, org string) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete || org == "" {
		return ev
	}

	now := time.Now().UTC()
	system := entityid.SystemSlug(root)

	state, err := h.ingestFileEntityState(ctx, ev.Path, root, system, org, now)
	if err == nil {
		ev.EntityStates = []*handler.EntityState{state}
	}
	return ev
}
