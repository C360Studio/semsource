package objectstore

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	doc "github.com/c360studio/semsource/handler/doc"
)

// SourceType is the configuration key for an object-store source, and the
// scheme of the URL a source entry carries.
const SourceType = "s3"

// DefaultPollInterval is how often Watch re-lists the prefix when a source
// does not choose. An object store announces nothing, so this is the entire
// latency between a new artifact appearing and the graph knowing about it —
// and the cost of a pass with no changes is one listing, not one download per
// object.
const DefaultPollInterval = time.Minute

// Handler ingests document artifacts out of an S3-compatible object store.
//
// It is a peer of handler/doc rather than a mode of it. It owns enumeration,
// change detection, and skip accounting, and hands the bytes of every object
// it decides to ingest to the doc handler's content seam. What comes back are
// ordinary document entities — the same shape a file on disk produces, which
// is why an object store needs no payload type of its own.
//
// Bytes never touch local disk. A materialization cache would put a local,
// non-intrinsic path where identity is derived from, which is the failure the
// entity-identity-safety spec exists to prevent.
type Handler struct {
	store ObjectStore
	docs  *doc.Handler

	org     string
	project string
	version string

	pollInterval time.Duration

	tracker *Tracker
}

// Option configures a Handler.
type Option func(*Handler)

// WithProject overrides the bucket-derived entity-ID system slug with an
// explicit project identity. Two prefixes of one bucket that are meant to be
// separate projects say so this way.
func WithProject(project string) Option {
	return func(h *Handler) { h.project = project }
}

// WithVersion scopes entity identity to an explicit version, so two snapshots
// of one corpus can coexist instead of overwriting each other. Empty keeps
// identifiers byte-for-byte — ScopedSystemSlug with no version is SystemSlug.
func WithVersion(version string) Option {
	return func(h *Handler) { h.version = version }
}

// WithPollInterval sets how often Watch re-lists the prefix.
func WithPollInterval(d time.Duration) Option {
	return func(h *Handler) {
		if d > 0 {
			h.pollInterval = d
		}
	}
}

// New returns a Handler reading objects from store and turning them into
// documents with docs, under the org namespace.
func New(store ObjectStore, docs *doc.Handler, org string, opts ...Option) *Handler {
	h := &Handler{
		store:        store,
		docs:         docs,
		org:          org,
		pollInterval: DefaultPollInterval,
		tracker:      NewTracker(),
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// SourceType returns the handler type identifier used in semsource.json.
func (h *Handler) SourceType() string { return SourceType }

// Supports reports whether cfg describes an object-store source.
func (h *Handler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == SourceType
}

// Ingested is one object whose bytes were fetched and turned into document
// entities.
type Ingested struct {
	// Key is the object key, which is also the document's logical path.
	Key string
	// Token is the change token observed for this object. Hand it back to
	// Record once the states below have been published.
	Token string
	// Operation is create for a key never ingested before, modify for one
	// whose content moved.
	Operation handler.ChangeOperation
	// States are the document's parent and passage entities.
	States []*handler.EntityState
}

// Removal is a document whose object a completed pass no longer observed.
type Removal struct {
	// Key is the object key that is gone.
	Key string
	// EntityID is the document entity built from that key, so a caller can
	// retract without rebuilding the identifier itself.
	EntityID string
}

// Result is what one ingest pass produced.
type Result struct {
	// Ingested is one entry per object fetched this pass.
	Ingested []Ingested
	// Removed is populated only by a pass that ran to completion.
	Removed []Removal
	// Skipped are objects the pass declined, each with its reason.
	Skipped []Skip
	// Unchanged counts objects observed whose token had not moved: not
	// re-fetched, not republished.
	Unchanged int
	// Bucket and Prefix identify what was enumerated, for status and errors.
	Bucket string
	Prefix string
}

// States flattens every ingested document's entities.
func (r *Result) States() []*handler.EntityState {
	if r == nil {
		return nil
	}
	var out []*handler.EntityState
	for _, ing := range r.Ingested {
		out = append(out, ing.States...)
	}
	return out
}

// SkipCounts totals the pass's skips by reason, which is the shape a status
// surface carries: per-object detail is unbounded in a bucket, a count per
// reason is not.
func (r *Result) SkipCounts() map[string]int64 {
	if r == nil || len(r.Skipped) == 0 {
		return nil
	}
	counts := make(map[string]int64, 2)
	for _, skip := range r.Skipped {
		counts[string(skip.Reason)]++
	}
	return counts
}

// IngestEntityStates enumerates the source's prefix and ingests everything
// that changed since the last pass.
//
// A listing that did not finish returns an error and no result. That is the
// whole retraction-safety story: Removed is populated from a pass that
// completed, so a transient listing failure cannot present itself as a corpus
// that was deleted.
//
// The pass records each object it ingests against the tracker. A caller whose
// publication then fails should call Forget on that key so the next pass
// fetches it again — change detection has no other way to learn that entities
// it produced never arrived.
func (h *Handler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig) (*Result, error) {
	bucket, prefix, err := ParseSourceURL(cfg.GetURL())
	if err != nil {
		return nil, fmt.Errorf("objectstore handler: %w", err)
	}

	pass, err := Enumerate(ctx, h.store, prefix)
	if err != nil {
		return nil, err
	}

	previous := h.tracker.Observed()
	plan := pass.Plan(previous)
	system := h.System(bucket)
	now := time.Now().UTC()

	result := &Result{
		Skipped:   plan.Skipped,
		Unchanged: pass.Len() - len(plan.Fetch) - len(plan.Skipped),
		Bucket:    bucket,
		Prefix:    prefix,
	}

	for _, info := range plan.Fetch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		states, err := h.ingestObject(ctx, info.Key, system, now)
		if err != nil {
			// An unreadable object is one document's problem: skip it and say
			// so. A body store that cannot be written to is the deployment's
			// problem, and every document after this one would fail the same
			// way — abort rather than report a healthy ingest of a corpus with
			// no retrievable bodies.
			if errors.Is(err, doc.ErrBodyStoreRequired) {
				return nil, err
			}
			result.Skipped = append(result.Skipped, Skip{Key: info.Key, Reason: SkipUnreadable})
			continue
		}

		operation := handler.OperationCreate
		if _, seen := previous[info.Key]; seen {
			operation = handler.OperationModify
		}

		token := ChangeToken(info)
		result.Ingested = append(result.Ingested, Ingested{
			Key:       info.Key,
			Token:     token,
			Operation: operation,
			States:    states,
		})
		h.tracker.Record(info.Key, token)
	}

	for _, key := range plan.Removed {
		result.Removed = append(result.Removed, Removal{
			Key:      key,
			EntityID: doc.DocumentEntityID(h.org, system, key),
		})
	}
	return result, nil
}

// ingestObject fetches one object and turns its bytes into document entities.
//
// The object key is handed to the seam as the logical path unchanged: it is
// already slash-delimited, it is intrinsic to the store rather than to this
// process, and it is what both the path triple and the instance segment are
// built from.
func (h *Handler) ingestObject(ctx context.Context, key, system string, now time.Time) ([]*handler.EntityState, error) {
	content, err := h.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("objectstore handler: read %q: %w", key, err)
	}
	return h.docs.IngestContentEntityStates(ctx, content, key, system, h.org, now)
}

// Watch re-lists the prefix on an interval and emits one event per object that
// moved, closing the channel when ctx ends.
//
// There is nothing to subscribe to — an object store tells nobody when a key
// changes — so watching is re-listing, and the poll interval is the whole
// latency between an artifact appearing and the graph holding it. A pass in
// which nothing changed costs one listing and reads no bodies.
//
// Events carry typed EntityStates and never RawEntity: this source publishes
// canonical state, so there is nothing for a normalizer pass to do with it.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}
	if _, _, err := ParseSourceURL(cfg.GetURL()); err != nil {
		return nil, fmt.Errorf("objectstore handler: Watch: %w", err)
	}

	events := make(chan handler.ChangeEvent)
	go func() {
		defer close(events)

		ticker := time.NewTicker(h.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			result, err := h.IngestEntityStates(ctx, cfg)
			if err != nil {
				// A failed pass concludes nothing. The next tick tries again,
				// against a tracker that still holds everything the last
				// successful pass established.
				continue
			}
			if !h.emit(ctx, events, result) {
				return
			}
		}
	}()
	return events, nil
}

// emit sends one event per change, reporting whether the channel is still
// live. A removal is sent with no states: the key is the signal, and what to
// retract follows from it.
func (h *Handler) emit(ctx context.Context, events chan<- handler.ChangeEvent, result *Result) bool {
	now := time.Now().UTC()

	for _, ing := range result.Ingested {
		select {
		case <-ctx.Done():
			return false
		case events <- handler.ChangeEvent{
			Path:         ing.Key,
			Operation:    ing.Operation,
			Timestamp:    now,
			EntityStates: ing.States,
		}:
		}
	}

	for _, removed := range result.Removed {
		select {
		case <-ctx.Done():
			return false
		case events <- handler.ChangeEvent{
			Path:      removed.Key,
			Operation: handler.OperationDelete,
			Timestamp: now,
		}:
		}
	}
	return true
}

// System returns the entity-ID system slug for a bucket: the explicit project
// override when set, else the bucket slug.
//
// Neither depends on anything local. The same bucket read from two machines,
// or the same key re-read after a restart, yields the same identifier — which
// is the property that lets object-store entities merge instead of
// accumulating siblings.
func (h *Handler) System(bucket string) string {
	project := bucket
	if h.project != "" {
		project = h.project
	}
	// ScopedSystemSlug with an empty version is SystemSlug, so an unversioned
	// source keeps byte-identical identifiers.
	return entityid.ScopedSystemSlug(project, h.version)
}

// Record notes that a key's entities have been published at the given token,
// so the next pass does not fetch it again.
func (h *Handler) Record(key, token string) { h.tracker.Record(key, token) }

// Forget drops a key from change detection, so the next pass treats it as new.
// Call it after publishing a retraction, and after a publication that failed.
func (h *Handler) Forget(key string) { h.tracker.Forget(key) }

// Tracked returns how many keys change detection is holding.
func (h *Handler) Tracked() int { return h.tracker.Len() }

// SourceURL renders the source URL for a bucket and prefix. An empty prefix
// means the whole bucket.
func SourceURL(bucket, prefix string) string {
	return SourceType + "://" + bucket + "/" + strings.TrimPrefix(prefix, "/")
}

// ParseSourceURL splits an s3://bucket/prefix source URL.
//
// The prefix is everything after the bucket, with no leading slash — object
// keys have no root, and "/reports/" would match nothing in a store whose keys
// begin "reports/".
func ParseSourceURL(raw string) (bucket, prefix string, err error) {
	if raw == "" {
		return "", "", errors.New("no source URL configured (expected s3://bucket/prefix)")
	}
	u, parseErr := url.Parse(raw)
	if parseErr != nil {
		return "", "", fmt.Errorf("source URL %q: %w", raw, parseErr)
	}
	if u.Scheme != SourceType {
		return "", "", fmt.Errorf("source URL %q: scheme must be %q", raw, SourceType)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("source URL %q: no bucket", raw)
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}
