package objectstore

import (
	"fmt"
	"path"
	"sort"
	"sync"

	vocabulary "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semsource/storage/s3store"
)

// SkipReason says why an object was enumerated but not ingested. It reaches
// the status surface verbatim, so an operator can tell "there is no such
// document" apart from "that document was never parsed".
type SkipReason string

const (
	// SkipUnsupportedFormat is an object whose extension the document
	// pipeline does not parse. A .go file in a repository is not a failure;
	// an unparseable artifact in a bucket of reports is something to see.
	SkipUnsupportedFormat SkipReason = "unsupported_format"

	// SkipEmptyObject is an object with no content. Publishing it would put a
	// document with an empty body in the graph, which reads as a document
	// that exists and says nothing.
	SkipEmptyObject SkipReason = "empty_object"

	// SkipUnreadable is an object the listing named and the fetch could not
	// read — deleted between the two passes, or denied by an object-level
	// policy. It is one document's problem, so the pass continues; it is
	// counted so a prefix that has quietly become unreadable does not present
	// as a prefix with nothing in it.
	SkipUnreadable SkipReason = "unreadable"
)

// Skip is one object the pass declined to ingest, and why.
type Skip struct {
	Key    string
	Reason SkipReason
}

// Plan is the work one pass implies: what to fetch, what is gone, and what was
// passed over.
type Plan struct {
	// Fetch are the objects whose bytes must be read and ingested — new keys,
	// and keys whose change token moved since the last successful ingest.
	// Ordered by key.
	Fetch []s3store.ObjectInfo

	// Removed are keys that were ingested before and that this pass did not
	// observe. Only a completed pass can populate this. Ordered.
	Removed []string

	// Skipped are objects the document pipeline cannot ingest, each with its
	// reason. Ordered by key.
	Skipped []Skip
}

// ChangeToken reduces an object's metadata to the value change detection
// compares between passes.
//
// The ETag is the signal where there is one. Where there is not — some
// S3-compatible stores omit it, and the spec names the fallback — size and
// last-modified together stand in. That fallback is genuinely weaker: an
// in-place edit that preserves both goes unnoticed until a restart re-ingests
// the prefix. It is still better than re-fetching every object every pass,
// which is the only other option.
//
// A token is never a content hash. Multipart uploads produce a composite ETag
// that is not the MD5 of the object, so anything that needs the content's hash
// computes it from the bytes.
func ChangeToken(info s3store.ObjectInfo) string {
	if info.ETag != "" {
		return info.ETag
	}
	return fmt.Sprintf("%d:%d", info.Size, info.LastModified.UnixNano())
}

// Plan compares what this pass observed against what was previously ingested
// and returns the work that follows.
//
// previous maps key to the change token recorded when that key was last
// ingested successfully — see Tracker.
//
// A nil Pass yields the zero Plan: nothing to fetch, nothing removed, nothing
// skipped. This is what a caller gets if it uses the result of a failed
// Enumerate without checking the error, and it is deliberately the harmless
// answer. The dangerous conclusion — that every previously ingested object has
// been deleted — requires a pass that actually finished.
func (p *Pass) Plan(previous map[string]string) Plan {
	if p == nil {
		return Plan{}
	}

	var plan Plan
	for key, info := range p.seen {
		if reason, skip := skipReason(info); skip {
			plan.Skipped = append(plan.Skipped, Skip{Key: key, Reason: reason})
			continue
		}
		// An object whose token is unchanged is not re-fetched and not
		// republished: the pass already knows everything about it that the
		// last one did.
		if token, ingested := previous[key]; ingested && token == ChangeToken(info) {
			continue
		}
		plan.Fetch = append(plan.Fetch, info)
	}

	for key := range previous {
		if _, present := p.seen[key]; !present {
			plan.Removed = append(plan.Removed, key)
		}
	}

	sort.Slice(plan.Fetch, func(i, j int) bool { return plan.Fetch[i].Key < plan.Fetch[j].Key })
	sort.Slice(plan.Skipped, func(i, j int) bool { return plan.Skipped[i].Key < plan.Skipped[j].Key })
	sort.Strings(plan.Removed)
	return plan
}

// skipReason applies the document gate to an object's metadata alone.
//
// It runs before any body is fetched, which is the divergence from the
// filesystem walk that matters: an unsupported object costs one listing entry
// rather than a download. Format is checked before emptiness because it is the
// more actionable answer for a zero-byte .png — the extension is why it will
// never be ingested, whatever its size becomes.
func skipReason(info s3store.ObjectInfo) (SkipReason, bool) {
	if !vocabulary.IsDocExtension(path.Ext(info.Key)) {
		return SkipUnsupportedFormat, true
	}
	if info.Size == 0 {
		return SkipEmptyObject, true
	}
	return "", false
}

// Tracker remembers the change token of every key this source has ingested
// successfully, for the life of the process.
//
// State is in memory and not persisted: a restart re-ingests the prefix once,
// which is idempotent by construction — same key, same entity ID, per-predicate
// replace — and consistent with seeding being the first pass of the continuous
// loop rather than a separate mode.
//
// Safe for concurrent use.
type Tracker struct {
	mu       sync.RWMutex
	observed map[string]string
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{observed: make(map[string]string)}
}

// Observed returns a copy of the recorded key-to-token map, suitable for
// passing to Plan. It is a copy so planning holds no lock and cannot be
// disturbed by an ingest completing underneath it.
func (t *Tracker) Observed() map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make(map[string]string, len(t.observed))
	for key, token := range t.observed {
		out[key] = token
	}
	return out
}

// Record notes that key has been ingested at the given token.
//
// It must be called only after the object's entities have actually been
// published. Recording at fetch time instead would mean an object whose ingest
// failed is remembered as done, and the next pass — seeing an unchanged
// token — would skip it forever.
func (t *Tracker) Record(key, token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.observed[key] = token
}

// Forget drops a key, after the entities for a removed object have been
// retracted. A key that is forgotten and later reappears ingests as new.
func (t *Tracker) Forget(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.observed, key)
}

// Len returns how many keys are currently tracked.
func (t *Tracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.observed)
}
