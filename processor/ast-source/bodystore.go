package astsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/storage/objectstore"

	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// The verbatim code body producer (ADR-062 hydration contract / ADR-0006 §5).
// At ingest, ast-source offloads each body-bearing entity's pre-sliced source to
// the "objectstore" store and stamps two body-handle triples (CodeBodyStore +
// CodeBodyKey). The fusion code lens reads them into a StorageReference and the
// engine's BodyResolver Gets the bytes — so a standalone/remote gateway serves
// verbatim source without access to the ingesting host's worktree. Bodies are
// content-addressed (sha256), so an unchanged symbol re-ingests to the same key.
//
// The store instance + bucket are graph.BodyStore{Instance,Bucket} (shared with
// the resolver so the addressing cannot drift).

// bodyKeyPrefix namespaces code bodies within the shared CONTENT bucket.
const bodyKeyPrefix = "code:"

// containerTypes are entity types with no verbatim body of their own (repo,
// folder, file, package) — their "body" would be a whole tree/file, not a symbol.
var containerTypes = map[semsourceast.CodeEntityType]bool{
	semsourceast.TypeRepo:    true,
	semsourceast.TypeFolder:  true,
	semsourceast.TypeFile:    true,
	semsourceast.TypePackage: true,
}

// initBodyStore attaches the verbatim-body ObjectStore (best-effort). A failure
// is logged, not fatal: ast-source still ingests; bodies are a best-effort facet
// and the fusion code lens degrades gracefully when the handle triples are absent.
func (c *Component) initBodyStore(ctx context.Context) {
	if c.natsClient == nil {
		return
	}
	store, err := objectstore.NewStoreWithConfig(ctx, c.natsClient, objectstore.Config{
		BucketName:   graph.BodyStoreBucket,
		InstanceName: graph.BodyStoreInstance,
	})
	if err != nil {
		c.logger.Warn("verbatim body store unavailable; code bodies will not be offloaded", "error", err)
		return
	}
	c.bodyStore = store
}

// bodyTriplesForResult offloads each body-bearing entity's verbatim source from
// the parsed file and returns per-entity-ID body-handle triples. The file is read
// once. result.Path is relative to the watcher root (the parser's repoRoot), so
// root is joined back to reach the file. Any read/offload fault degrades that
// entity to no body rather than failing ingest. Returns nil when no store is set.
func (c *Component) bodyTriplesForResult(ctx context.Context, result *semsourceast.ParseResult, root string) map[string][]message.Triple {
	if c.bodyStore == nil || result == nil || result.Path == "" {
		return nil
	}
	absPath := filepath.Join(root, result.Path)
	content, err := os.ReadFile(absPath)
	if err != nil {
		c.logger.Debug("read source for body offload failed", "path", absPath, "error", err)
		return nil
	}
	lines := strings.Split(string(content), "\n")

	out := make(map[string][]message.Triple)
	for _, e := range result.Entities {
		if e == nil || containerTypes[e.Type] {
			continue
		}
		body := sliceLines(lines, e.StartLine, e.EndLine)
		if body == "" {
			continue
		}
		key := bodyKeyPrefix + hashBody(body)
		if err := c.bodyStore.Put(ctx, key, []byte(body)); err != nil {
			c.logger.Warn("offload code body failed", "entity", e.ID, "error", err)
			continue
		}
		out[e.ID] = []message.Triple{
			{Subject: e.ID, Predicate: semsourceast.CodeBodyStore, Object: graph.BodyStoreInstance},
			{Subject: e.ID, Predicate: semsourceast.CodeBodyKey, Object: key},
		}
	}
	return out
}

// sliceLines returns the verbatim source for the inclusive 1-based [start,end]
// range, or "" when the range is invalid or out of bounds.
func sliceLines(lines []string, start, end int) string {
	if start <= 0 || end < start || start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}

// hashBody content-addresses a body so identical bodies share one blob.
func hashBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}
