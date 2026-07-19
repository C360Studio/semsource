package entitypub

import (
	"sync"

	"github.com/c360studio/semsource/entityid"
)

// DistinctTracker counts DISTINCT entity IDs (total and per domain.type key).
// Source status previously reported monotone publish counters as entity
// counts, so the 60s periodic reindex inflated them forever (audit
// 2026-07-19: folder ×4, repo ×4 within minutes while the graph itself was
// clean). Distinct cardinality is invariant under republication; raw publish
// throughput is reported separately as publish_total.
type DistinctTracker struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	types map[string]int64
}

// NewDistinctTracker returns an empty tracker.
func NewDistinctTracker() *DistinctTracker {
	return &DistinctTracker{
		seen:  make(map[string]struct{}),
		types: make(map[string]int64),
	}
}

// Observe records an entity ID, returning true the first time it is seen.
// Republications of a known ID change nothing.
func (t *DistinctTracker) Observe(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.seen[id]; ok {
		return false
	}
	t.seen[id] = struct{}{}
	if domain, eType := entityid.Parts(id); domain != "" {
		t.types[domain+"."+eType]++
	}
	return true
}

// Count returns the number of distinct entity IDs observed.
func (t *DistinctTracker) Count() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return int64(len(t.seen))
}

// TypeCounts returns a point-in-time copy of distinct counts per domain.type.
func (t *DistinctTracker) TypeCounts() map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]int64, len(t.types))
	for k, v := range t.types {
		out[k] = v
	}
	return out
}
