package docsource

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"

	"github.com/c360studio/semsource/graph"
)

// changeTracker records, per entity, the statements this process last handed
// to the publisher, so a republish is reconciled only when this process knows
// the entity's content changed (#336, D4). A first sighting — the seed, or a
// document created while watching — is not a known change: the stored entity
// may predate this process, and nothing it last wrote is known here.
type changeTracker struct {
	mu   sync.Mutex
	last map[string][sha256.Size]byte
}

// changed reports whether id was handed off before with different statements.
func (t *changeTracker) changed(id string, fp [sha256.Size]byte) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, seen := t.last[id]
	return seen && prev != fp
}

// record notes the statements of a publish the publisher accepted. Called
// only after Send succeeds, so a refused publish is compared again next time.
func (t *changeTracker) record(id string, fp [sha256.Size]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = map[string][sha256.Size]byte{}
	}
	t.last[id] = fp
}

// fingerprint hashes a payload's statements without their timestamps, which
// differ on every publish.
func fingerprint(p *graph.EntityPayload) [sha256.Size]byte {
	lines := make([]string, 0, len(p.TripleData))
	for _, t := range p.TripleData {
		lines = append(lines, fmt.Sprintf("%s\x00%v\x00%s", t.Predicate, t.Object, t.Datatype))
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{0})
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

// markReconcile sets the payload's SkipReconcile and returns its fingerprint
// for record. Only a republish whose statements differ from what this
// process last published reconciles, so the seed — every entity's first
// sighting — never does.
func (c *Component) markReconcile(payload *graph.EntityPayload) [sha256.Size]byte {
	fp := fingerprint(payload)
	payload.SkipReconcile = !c.changes.changed(payload.ID, fp)
	return fp
}
