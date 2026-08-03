package astsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// bodyPutConcurrency bounds in-flight body writes per file. The offload was one
// SYNCHRONOUS Put per body-bearing entity: measured against a local NATS at
// ~875µs each, that is ~23s of pure round-trip for OSH Core's 25,960 bodies,
// and it degrades with contention. Bounded rather than unbounded so a large file
// cannot swamp the connection.
const bodyPutConcurrency = 16

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

// codeBody is a body-bearing entity's offload result, wiring one CONTENT blob to
// both consumers (ADR-063, mirroring the #19 doc unify): the handle triples the
// fusion code lens reads, and a StorageReference to the SAME blob that
// graph-embedding fetches via the shared StoreRegistry to embed the body.
type codeBody struct {
	triples []message.Triple
	ref     *message.StorageReference
}

// bodiesForResult offloads each body-bearing entity's verbatim source from the
// parsed file and returns per-entity-ID offload results (handle triples + a
// StorageRef to the same blob). The file is read once. result.Path is relative to
// the watcher root (the parser's repoRoot), so root is joined back to reach the
// file. Any read/offload fault degrades that entity to no body rather than failing
// ingest. Returns nil when no store is set.
func (c *Component) bodiesForResult(ctx context.Context, result *semsourceast.ParseResult, root string) map[string]codeBody {
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

	// Slice and hash first — pure CPU, and cheap: measured over OSH Core's 25,960
	// bodies, slicing is 11ms and sha256 43ms. The cost is entirely in the Puts
	// below, which is why they are deduped and issued concurrently rather than
	// one blocking round trip per entity.
	type pending struct {
		entity string
		key    string
		body   string
	}
	work := make([]pending, 0, len(result.Entities))
	for _, e := range result.Entities {
		if e == nil || containerTypes[e.Type] {
			continue
		}
		body := sliceLines(lines, e.StartLine, e.EndLine)
		if body == "" {
			continue
		}
		work = append(work, pending{entity: e.ID, key: bodyKeyPrefix + hashBody(body), body: body})
	}

	var mu sync.Mutex
	out := make(map[string]codeBody, len(work))
	attach := func(w pending) {
		mu.Lock()
		defer mu.Unlock()
		out[w.entity] = codeBody{
			triples: []message.Triple{
				{Subject: w.entity, Predicate: semsourceast.CodeBodyStore, Object: graph.BodyStoreInstance},
				{Subject: w.entity, Predicate: semsourceast.CodeBodyKey, Object: w.key},
			},
			ref: &message.StorageReference{
				StorageInstance: graph.BodyStoreInstance,
				Key:             w.key,
				ContentType:     "text/plain; charset=utf-8",
				Size:            int64(len(w.body)),
			},
		}
	}

	sem := make(chan struct{}, bodyPutConcurrency)
	var wg sync.WaitGroup
	for _, w := range work {
		// Bodies are content-addressed, so a key already written this run holds
		// byte-identical content and re-Putting it is pure waste — 12.3% of all
		// Puts on OSH Core. Skipping the WRITE must NOT skip the ATTACHMENT: the
		// entity still needs its handle triples, or it silently loses its body.
		if c.bodyPutDone(w.key) {
			attach(w)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(w pending) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := c.bodyStore.Put(ctx, w.key, []byte(w.body)); err != nil {
				c.logger.Warn("offload code body failed", "entity", w.entity, "error", err)
				return // degrade this entity to no body, as before
			}
			c.markBodyPut(w.key)
			attach(w)
		}(w)
	}
	wg.Wait()
	return out
}

// bodyPutDone reports whether this key has already been written in this process.
func (c *Component) bodyPutDone(key string) bool {
	_, ok := c.bodyPuts.Load(key)
	return ok
}

// markBodyPut records a key as written. The set is bounded by the number of
// DISTINCT bodies in the watched tree (22,772 on OSH Core, ~1.5 MB of keys), and
// a re-parse of unchanged files hits it rather than growing it.
func (c *Component) markBodyPut(key string) { c.bodyPuts.Store(key, struct{}{}) }

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
