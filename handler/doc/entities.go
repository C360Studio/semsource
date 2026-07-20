package doc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// Entity is the parent document: the stable navigational node carrying
// identity, title, path, hash, provenance, and the passage count.
//
// It holds NO body. A whole-file body here would keep the averaged, truncated
// whole-file vector that passages exist to replace, and would return the same
// prose twice — once as the document, again as its passages. Bodies live on
// PassageEntity.
//
// It is still embedded, from its title: graph-embedding's text_suffixes include
// ".title", and its indexingEligible is lenient by design (ADR-054 Phase 1 never
// excludes), so there is no producer-side way to opt an entity out of embedding
// today. A title-only vector for a navigational node is defensible — a title
// match should surface the document — but whether it becomes lexical noise the
// way empty-bodied config nodes did is an empirical question, measured by the
// graded re-run rather than asserted here.
type Entity struct {
	ID          string
	Title       string
	FilePath    string
	MimeType    string
	ContentHash string
	IndexedAt   time.Time

	// ChunkCount is how many passage entities this document currently has. It is
	// the signal the retraction pass uses: a passage whose DocChunkIndex is at or
	// above the parent's count no longer exists in the file and must not serve as
	// current. The staleness pass cannot derive this from the filesystem, because
	// every passage of a shrunken document still carries the path of a file that
	// is very much still there.
	ChunkCount int
}

// newEntity constructs a Entity with a deterministic 6-part ID. The instance
// is the sanitized relative file path (entity-staleness spec D3 — mirrors the
// code-file convention: source/ast.BuildInstanceID uses the path, not a
// content hash). This makes doc identity STABLE across edits: an edit
// re-ingests the SAME entity ID, so the substrate's per-predicate replace
// updates content triples in place instead of minting an orphaned sibling
// entity every save. The content hash still travels as the DocFileHash triple
// for change detection — it just no longer feeds identity.
func newEntity(org, title, filePath, mimeType, contentHash, system string, indexedAt time.Time) *Entity {
	instance := entityid.SanitizeInstance(filePath)
	return &Entity{
		ID:          entityid.Build(org, entityid.PlatformSemsource, "web", system, "doc", instance),
		Title:       title,
		FilePath:    filePath,
		MimeType:    mimeType,
		ContentHash: contentHash,
		IndexedAt:   indexedAt,
	}
}

// Triples converts the Entity to a slice of message.Triple using canonical
// vocabulary predicates. No body triples: the parent carries none.
func (e *Entity) Triples() []message.Triple {
	now := e.IndexedAt
	triples := []message.Triple{
		{Subject: e.ID, Predicate: source.DocType, Object: "document", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFilePath, Object: e.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocMimeType, Object: e.MimeType, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocFileHash, Object: e.ContentHash, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DcTitle, Object: e.Title, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: e.ID, Predicate: source.DocChunkCount, Object: e.ChunkCount, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
	}
	return triples
}

// PassageEntity is one retrievable passage of a document: its own entity, with
// its own body handle, linked to the parent document by CodeBelongs.
//
// Passages exist because the substrate embeds one vector per entity from text
// truncated at 8000 characters, so a whole-file entity both dilutes the vector
// and silently loses everything past the cut. They carry the parent's FilePath
// deliberately — the staleness pass groups by path to decide liveness, and a
// passage with no path predicate is skipped outright.
type PassageEntity struct {
	ID        string
	ParentID  string
	Title     string
	Section   string
	Ordinal   int
	FilePath  string
	MimeType  string
	Body      string
	IndexedAt time.Time

	storageRef   *message.StorageReference
	bodyInstance string
	bodyKey      string
}

// chunkInstance builds a passage's instance segment from its parent's path and
// its ordinal. Identity is derivable from (path, ordinal) alone — never from
// content, timestamp, or insertion order — so re-ingesting an unchanged document
// reproduces byte-identical passage IDs, and a heading rename does not orphan a
// passage the way a content-derived or heading-derived ID would.
func chunkInstance(filePath string, ordinal int) string {
	return fmt.Sprintf("%s-%04d", entityid.SanitizeInstance(filePath), ordinal)
}

// passageTitle qualifies a passage's title with its parent's, so a results list
// does not show six indistinguishable "Usage" entries from six documents.
func passageTitle(parentTitle, section string, ordinal int) string {
	if section != "" {
		return parentTitle + " § " + section
	}
	return fmt.Sprintf("%s § passage %d", parentTitle, ordinal+1)
}

// newPassageEntity builds a passage entity for one split of a document.
func newPassageEntity(org, system, parentID, parentTitle, filePath, mimeType string, p passage, indexedAt time.Time) *PassageEntity {
	return &PassageEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, "web", system, "chunk", chunkInstance(filePath, p.Ordinal)),
		ParentID:  parentID,
		Title:     passageTitle(parentTitle, p.Heading, p.Ordinal),
		Section:   p.Heading,
		Ordinal:   p.Ordinal,
		FilePath:  filePath,
		MimeType:  mimeType,
		Body:      p.Body,
		IndexedAt: indexedAt,
	}
}

// Triples builds the passage's typed facts. DocChunkIndex is 0-indexed so the
// retraction rule is a plain comparison against the parent's DocChunkCount.
func (p *PassageEntity) Triples() []message.Triple {
	now := p.IndexedAt
	triples := []message.Triple{
		{Subject: p.ID, Predicate: source.DocType, Object: "passage", Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: p.ID, Predicate: source.DocFilePath, Object: p.FilePath, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: p.ID, Predicate: source.DocMimeType, Object: p.MimeType, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: p.ID, Predicate: source.DcTitle, Object: p.Title, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{Subject: p.ID, Predicate: source.DocChunkIndex, Object: p.Ordinal, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		{
			Subject: p.ID, Predicate: source.CodeBelongs, Object: p.ParentID,
			Datatype: message.EntityReferenceDatatype,
			Source:   entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0,
		},
	}
	if p.Section != "" {
		triples = append(triples, message.Triple{
			Subject: p.ID, Predicate: source.DocSection, Object: p.Section,
			Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0,
		})
	}
	if p.bodyKey != "" && p.bodyInstance != "" {
		triples = append(triples,
			message.Triple{Subject: p.ID, Predicate: source.DocBodyStore, Object: p.bodyInstance, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: p.ID, Predicate: source.DocBodyKey, Object: p.bodyKey, Source: entityid.PlatformSemsource, Timestamp: now, Confidence: 1.0},
		)
	}
	return triples
}

// EntityState converts the passage to a handler.EntityState for publication.
func (p *PassageEntity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:              p.ID,
		Triples:         p.Triples(),
		UpdatedAt:       p.IndexedAt,
		StorageRef:      p.storageRef,
		IndexingProfile: semvocab.IndexingProfileContent,
	}
}

// EntityState converts the Entity to a handler.EntityState for direct graph publication.
func (e *Entity) EntityState() *handler.EntityState {
	return &handler.EntityState{
		ID:              e.ID,
		Triples:         e.Triples(),
		UpdatedAt:       e.IndexedAt,
		IndexingProfile: semvocab.IndexingProfileContent,
	}
}

// IngestEntityStates walks all configured paths and returns fully-typed entity
// states that embed vocabulary-predicate triples directly. org is the
// organisation namespace (e.g. "acme") used in the 6-part entity ID.
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
				if isDefaultExcludedDocDir(root, path) {
					return filepath.SkipDir
				}
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !docExtensions[ext] {
				return nil
			}

			fileStates, err := h.ingestFileEntityStates(ctx, path, root, system, org, now)
			if err != nil {
				// An unreadable file is one document's problem: skip it. A body
				// store that cannot be written to is the deployment's problem,
				// and every document after this one would fail the same way —
				// abort rather than report a healthy ingest of a corpus with no
				// retrievable bodies.
				if errors.Is(err, ErrBodyStoreRequired) {
					return err
				}
				return nil
			}
			states = append(states, fileStates...)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("doc handler: walk %q: %w", root, walkErr)
		}
	}

	return states, nil
}

// ingestFileEntityStates reads a single document and returns the parent entity
// followed by one entity per passage, in ordinal order. The parent is the stable
// navigational node (identity, title, path, hash, provenance, chunk count); the
// passages carry the retrievable bodies.
func (h *Handler) ingestFileEntityStates(ctx context.Context, path, root, system, org string, now time.Time) ([]*handler.EntityState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	hash := contentHash(content)
	relPath, _ := filepath.Rel(root, path)
	title := extractTitle(content, filepath.Base(path))
	mime := mimeForExt(filepath.Ext(path))
	passages := splitPassages(content)

	parent := newEntity(org, title, relPath, mime, hash, system, now)
	parent.ChunkCount = len(passages)

	states := make([]*handler.EntityState, 0, len(passages)+1)
	states = append(states, parent.EntityState())
	for _, p := range passages {
		pe := newPassageEntity(org, system, parent.ID, title, relPath, mime, p, now)
		if err := offloadPassageBody(ctx, pe, h.bodyStore, h.bodyInstance); err != nil {
			return nil, err
		}
		states = append(states, pe.EntityState())
	}
	return states, nil
}

// offloadPassageBody stores a passage's verbatim body and stamps the handle.
// Because the key is the hash of the PASSAGE and not of the file, editing one
// section rewrites only that passage's blob while every unchanged passage keeps
// its existing key and is never re-Put — per-edit blob churn is O(changed
// passage), not O(file).
//
// A failure here is returned, not swallowed. There is no inline fallback any
// more: a passage without a handle is a passage whose body cannot be hydrated
// and whose text never reaches the semantic index, and reporting that as a
// healthy ingest is the silent-degradation shape this project keeps removing.
func offloadPassageBody(ctx context.Context, p *PassageEntity, store storage.Store, instance string) error {
	key, ref, err := putBody(ctx, store, instance, p.Body, p.MimeType)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil // nothing to store
	}
	p.bodyInstance = instance
	p.bodyKey = key
	p.storageRef = ref
	return nil
}

// ErrBodyStoreRequired reports that the verbatim body store is unavailable. It
// is distinguished from an unreadable file so the caller can tell a per-document
// problem (skip it) from a broken deployment (fail loudly).
var ErrBodyStoreRequired = errors.New("doc handler: verbatim body store is required")

// putBody content-addresses body and stores it, returning the key and a matching
// StorageReference. Identical bodies — a shared licence header, a boilerplate
// section repeated across documents — collapse onto one blob. A nil ref with no
// error means there was nothing to store.
func putBody(ctx context.Context, store storage.Store, instance, body, mimeType string) (string, *message.StorageReference, error) {
	if store == nil || instance == "" {
		return "", nil, ErrBodyStoreRequired
	}
	if body == "" {
		return "", nil, nil
	}
	sum := sha256.Sum256([]byte(body))
	key := "doc:" + hex.EncodeToString(sum[:])
	if err := store.Put(ctx, key, []byte(body)); err != nil {
		// Classified as deployment-level, not per-document. A store that cannot
		// be written to will fail identically for every following document, and
		// falling through to the walk's skip branch would drop each one whole —
		// parent included — while reporting a healthy ingest.
		return "", nil, fmt.Errorf("%w: store passage body: %w", ErrBodyStoreRequired, err)
	}
	return key, &message.StorageReference{
		StorageInstance: instance,
		Key:             key,
		ContentType:     mimeType,
		Size:            int64(len(body)),
	}, nil
}

// enrichEventEntityStates re-reads the changed file and populates ev.EntityStates
// using vocabulary-predicate triples. org is required so
// entity IDs are deterministic. For delete events the file is gone and
// EntityStates remains empty.
func (h *Handler) enrichEventEntityStates(ctx context.Context, ev handler.ChangeEvent, root, org string) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete || org == "" {
		return ev
	}

	now := time.Now().UTC()
	system := entityid.SystemSlug(root)

	states, err := h.ingestFileEntityStates(ctx, ev.Path, root, system, org, now)
	if err == nil {
		ev.EntityStates = states
	}
	return ev
}
