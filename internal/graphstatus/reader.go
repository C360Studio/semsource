// Package graphstatus reads the SemStreams ADR-083 readiness envelopes that
// producers publish into the GRAPH_STATUS KV bucket.
//
// It exists because ADR-083 REMOVED the `graph.index.query.status`
// request/reply subject. Readiness is now watched KV state, one key per
// producer, folded client-side by the consumer (ADR-088: there is no published
// aggregate and there never will be, because a published aggregate would be the
// one envelope that is derived rather than observed). A caller that still
// requests the old subject gets no responder and — because an absent optional
// responder is indistinguishable from a broken one at that seam — reports
// "unknown" forever while the substrate is perfectly healthy.
//
// The key list is deliberately the CONSUMER's. SemStreams exports the bucket
// name but not the key set, because declaring a key you do not depend on makes
// you defer on someone else's outage, and omitting one you do depend on is a
// bug that should be visible at your own call site.
package graphstatus

import (
	"context"
	"fmt"
	"sync"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
)

// Producer keys are the publishing components' names. They are string literals
// rather than framework constants because SemStreams does not export them — see
// the package comment on why the key list belongs to the consumer.
const (
	// KeyGraphIndex is the structural index (NAME_INDEX and friends): it backs
	// byName, code_context, and code_impact.
	KeyGraphIndex = "graph-index"

	// KeyGraphEmbedding is the semantic index: it backs code_search.
	KeyGraphEmbedding = "graph-embedding"

	// KeyGraphIngest is the entity-state writer. Its envelope carries
	// bootstrap_scope, so `bootstrap_complete && bootstrap_scope == 0` is an
	// authoritative "there was nothing to do" rather than "you asked too early".
	KeyGraphIngest = "graph-ingest"
)

// Reader binds the GRAPH_STATUS bucket lazily and reads producer envelopes from
// it. The zero value is not usable; construct with New.
type Reader struct {
	client *natsclient.Client

	mu     sync.Mutex
	bucket semgraph.CatalogReader
}

// New returns a Reader over the given NATS client.
func New(client *natsclient.Client) *Reader {
	return &Reader{client: client}
}

// Raw returns the raw readiness envelope a producer published under key.
//
// Binding is lazy and retried on every failure rather than cached-once: readers
// bind must-exist through the framework catalog seam and never create, so a
// GRAPH_STATUS that does not exist yet means its owner has not started. That is
// a transient startup condition, and caching the failure would make an
// early-booting consumer report unknown for the life of the process.
func (r *Reader) Raw(ctx context.Context, key string) ([]byte, error) {
	bucket, err := r.resolveBucket(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := bucket.Get(ctx, key)
	if err != nil {
		// An absent key is NOT "not ready" and NOT "empty" — it is unknown, and
		// unknown defers. The caller distinguishes the two; this layer only
		// reports honestly that nothing was published.
		return nil, fmt.Errorf("read %s readiness key %q: %w", semgraph.BucketGraphStatus, key, err)
	}
	return entry.Value(), nil
}

func (r *Reader) resolveBucket(ctx context.Context) (semgraph.CatalogReader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.bucket != nil {
		return r.bucket, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("open %s: NATS client is unavailable", semgraph.BucketGraphStatus)
	}
	// OpenCatalogReader binds must-exist through the reader seam: it never
	// creates, and an absent bucket yields a classified not-ready error naming
	// the catalog owner.
	bucket, err := semgraph.OpenCatalogReader(ctx, r.client, semgraph.BucketGraphStatus)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", semgraph.BucketGraphStatus, err)
	}
	r.bucket = bucket
	return bucket, nil
}
