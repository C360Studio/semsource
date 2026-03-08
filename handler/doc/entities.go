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
)

// DocEntity is a fully-typed document entity that builds triples directly
// using canonical vocabulary predicates, bypassing the normalizer.
type DocEntity struct {
	ID          string
	Title       string
	FilePath    string
	MimeType    string
	ContentHash string
	Content     string
	System      string
	Org         string
	IndexedAt   time.Time
}

// newDocEntity constructs a DocEntity with a deterministic 6-part ID.
// The instance is the first 6 hex chars of the content hash — matching the
// RawEntity path so normalizer and direct paths produce identical IDs.
func newDocEntity(org, title, filePath, mimeType, contentHash, content, system string, indexedAt time.Time) *DocEntity {
	instance := contentHash
	if len(instance) > 6 {
		instance = instance[:6]
	}
	return &DocEntity{
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

// Triples converts the DocEntity to a slice of message.Triple using canonical
// vocabulary predicates from source/vocabulary.
func (e *DocEntity) Triples() []message.Triple {
	now := e.IndexedAt
	return []message.Triple{
		{Subject: e.ID, Predicate: source.DocType, Object: "document", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocMimeType, Object: e.MimeType, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFileHash, Object: e.ContentHash, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocContent, Object: e.Content, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocSummary, Object: e.Title, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
}

// EntityState converts the DocEntity to a handler.EntityState for direct graph publication.
func (e *DocEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:        e.ID,
		Triples:   e.Triples(),
		UpdatedAt: e.IndexedAt,
	}
}

// IngestEntityStates walks all configured paths and returns fully-typed entity
// states that embed vocabulary-predicate triples directly, bypassing the
// normalizer entirely. org is the organisation namespace (e.g. "acme") used
// in the 6-part entity ID.
func (h *DocHandler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
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

		system := entityid.SystemSlug(slugify(root))

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

			state, err := ingestFileEntityState(path, root, system, org, now)
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

// ingestFileEntityState reads a single document file and builds a DocEntity,
// returning its EntityState. Mirrors ingestFile but produces typed triples.
func ingestFileEntityState(path, root, system, org string, now time.Time) (*handler.EntityState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	hash := contentHash(content)
	relPath, _ := filepath.Rel(root, path)
	title := extractTitle(content, filepath.Base(path))
	mime := mimeForExt(filepath.Ext(path))

	e := newDocEntity(org, title, relPath, mime, hash, string(content), system, now)
	return e.EntityState(), nil
}

// enrichEventEntityStates re-reads the changed file and populates ev.EntityStates
// alongside ev.Entities, using vocabulary-predicate triples. org is required so
// entity IDs are deterministic. For delete events the file is gone and
// EntityStates remains empty.
func enrichEventEntityStates(ev handler.ChangeEvent, root, org string) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete || org == "" {
		return ev
	}

	now := time.Now().UTC()
	system := entityid.SystemSlug(slugify(root))

	state, err := ingestFileEntityState(ev.Path, root, system, org, now)
	if err == nil {
		ev.EntityStates = []*handler.EntityState{state}
	}
	return ev
}
