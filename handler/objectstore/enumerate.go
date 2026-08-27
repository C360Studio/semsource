// Package objectstore ingests document artifacts that live in an S3-compatible
// object store rather than on a local filesystem.
//
// It owns enumeration, change detection, and skip accounting; the documents
// themselves are built by the existing doc pipeline, which the source hands
// object bytes and a logical path. There is no local materialization step —
// object bytes never touch disk as a cache, so identity cannot come to depend
// on where a fetch happened to land.
package objectstore

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semsource/storage/s3store"
)

// ObjectStore is the slice of an object store this source needs: enumerate a
// prefix, and read one object's bytes.
//
// It is deliberately narrower than storage.Store, which can also put and
// delete. A source that cannot reach those methods cannot call them by
// accident, so "an object-store source never writes to the bucket" holds under
// maintenance rather than under review. *s3store.Store satisfies it as-is.
type ObjectStore interface {
	// Objects returns metadata for every object under prefix, or an error.
	// Implementations must consume the store's full paginated listing and must
	// never return a partial result alongside a nil error.
	Objects(ctx context.Context, prefix string) ([]s3store.ObjectInfo, error)

	// Get returns the bytes of one object.
	Get(ctx context.Context, key string) ([]byte, error)
}

// *s3store.Store satisfies ObjectStore as it stands. No adapter sits between
// them, so there is nothing for the two to drift apart across.
var _ ObjectStore = (*s3store.Store)(nil)

// Pass is what one enumeration of a prefix observed.
//
// A Pass exists only for a listing that ran to completion across every
// continuation page — Enumerate returns nil alongside its error otherwise —
// and every question about which objects exist is asked of one. That is the
// point. A listing that died on its third page knows nothing about what the
// remaining pages held, and the conclusion it must never be allowed to reach
// is "the rest of the corpus is gone".
//
// The nil Pass answers every such question with "nothing", which is the safe
// direction: a caller that skipped its error check retracts nothing rather
// than everything.
type Pass struct {
	prefix string
	seen   map[string]s3store.ObjectInfo
}

// Enumerate lists every object under prefix and returns what it observed.
//
// The store is asked for a prefix-scoped listing and the result is scoped
// again here. The second filter is not redundant bookkeeping: it makes the
// source's own promise — that objects outside the configured prefix are never
// ingested — independent of whether a particular S3 implementation honors the
// prefix parameter exactly.
func Enumerate(ctx context.Context, store ObjectStore, prefix string) (*Pass, error) {
	infos, err := store.Objects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("objectstore: enumerate %q: %w", prefix, err)
	}

	seen := make(map[string]s3store.ObjectInfo, len(infos))
	for _, info := range infos {
		if !strings.HasPrefix(info.Key, prefix) {
			continue
		}
		seen[info.Key] = info
	}
	return &Pass{prefix: prefix, seen: seen}, nil
}

// Prefix returns the prefix this pass enumerated.
func (p *Pass) Prefix() string {
	if p == nil {
		return ""
	}
	return p.prefix
}

// Len returns how many objects the pass observed.
func (p *Pass) Len() int {
	if p == nil {
		return 0
	}
	return len(p.seen)
}

// Objects returns everything the pass observed, ordered by key.
func (p *Pass) Objects() []s3store.ObjectInfo {
	if p == nil {
		return nil
	}
	out := make([]s3store.ObjectInfo, 0, len(p.seen))
	for _, info := range p.seen {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
