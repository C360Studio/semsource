package objectstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/pkg/types"
	"github.com/c360studio/semstreams/storage"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	doc "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/handler/objectstore"
	sourcevocab "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semsource/storage/s3store"
)

const testOrg = "acme"

// bodyStore is the verbatim passage store the doc pipeline requires. A local
// filestore stands in — where passage bodies land has no bearing on identity,
// which is derived from the object key alone.
func bodyStore(t *testing.T) storage.Store {
	t.Helper()
	s, err := filestore.New(t.TempDir(), true)
	if err != nil {
		t.Fatalf("filestore.New: %v", err)
	}
	return s
}

// corpusStore is a fake object store holding real document bytes.
//
// Guarded because the watch tests change the corpus from the test goroutine
// while the poll loop is listing it — which is exactly what a real bucket does
// under a running source, and what the fake has to model faithfully.
type corpusStore struct {
	mu      sync.RWMutex
	objects []s3store.ObjectInfo
	bodies  map[string][]byte

	listErr error
	getErr  map[string]error

	// writes records any attempt to mutate the bucket. The read-only contract
	// is kept by the ObjectStore interface exposing no way to write at all, so
	// a non-zero count here would mean the interface itself had grown one.
	writes int
}

func newCorpus(t *testing.T, docs map[string]string) *corpusStore {
	t.Helper()

	c := &corpusStore{bodies: make(map[string][]byte, len(docs))}
	for key, body := range docs {
		c.bodies[key] = []byte(body)
		c.objects = append(c.objects, s3store.ObjectInfo{
			Key:          key,
			ETag:         "etag-" + key,
			Size:         int64(len(body)),
			LastModified: modTime,
		})
	}
	return c
}

func (c *corpusStore) Objects(_ context.Context, prefix string) ([]s3store.ObjectInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.listErr != nil {
		return nil, c.listErr
	}
	var out []s3store.ObjectInfo
	for _, info := range c.objects {
		if strings.HasPrefix(info.Key, prefix) {
			out = append(out, info)
		}
	}
	return out, nil
}

func (c *corpusStore) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err, failing := c.getErr[key]; failing {
		return nil, err
	}
	body, present := c.bodies[key]
	if !present {
		return nil, storage.ErrObjectNotFound
	}
	return body, nil
}

// replace changes an object's content and moves its token, the way an
// re-upload at the same key does.
func (c *corpusStore) replace(key, body, etag string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bodies[key] = []byte(body)
	for i := range c.objects {
		if c.objects[i].Key == key {
			c.objects[i].ETag = etag
			c.objects[i].Size = int64(len(body))
			return
		}
	}
}

// failRead makes a key's fetch fail.
func (c *corpusStore) failRead(key string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.getErr == nil {
		c.getErr = make(map[string]error)
	}
	c.getErr[key] = err
}

// remove deletes an object from the fake store's view.
func (c *corpusStore) remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.bodies, key)
	kept := c.objects[:0]
	for _, info := range c.objects {
		if info.Key != key {
			kept = append(kept, info)
		}
	}
	c.objects = kept
}

// sourceCfg is a minimal SourceConfig for an object-store source.
type sourceCfg struct {
	url   string
	watch bool
}

func (s sourceCfg) GetType() string             { return objectstore.SourceType }
func (s sourceCfg) GetPath() string             { return "" }
func (s sourceCfg) GetPaths() []string          { return nil }
func (s sourceCfg) GetURL() string              { return s.url }
func (s sourceCfg) GetBranch() string           { return "" }
func (s sourceCfg) IsWatchEnabled() bool        { return s.watch }
func (s sourceCfg) GetKeyframeMode() string     { return "" }
func (s sourceCfg) GetKeyframeInterval() string { return "" }
func (s sourceCfg) GetSceneThreshold() float64  { return 0 }

func newHandler(t *testing.T, store objectstore.ObjectStore, opts ...objectstore.Option) *objectstore.Handler {
	t.Helper()
	docs := doc.New(doc.WithBodyStore(bodyStore(t), "test-bodies"))
	return objectstore.New(store, docs, testOrg, opts...)
}

func cfgFor(bucket, prefix string) sourceCfg {
	return sourceCfg{url: objectstore.SourceURL(bucket, prefix)}
}

// ─── Interface and dispatch ─────────────────────────────────────────────────

func TestHandler_SourceTypeAndSupports(t *testing.T) {
	h := newHandler(t, newCorpus(t, nil))

	if h.SourceType() != "s3" {
		t.Errorf("SourceType = %q, want %q", h.SourceType(), "s3")
	}
	if !h.Supports(sourceCfg{url: "s3://artifacts/reports/"}) {
		t.Error("Supports should accept its own source type")
	}
	if h.Supports(mismatchedCfg{}) {
		t.Error("Supports must not claim a source it cannot handle")
	}
}

type mismatchedCfg struct{ sourceCfg }

func (mismatchedCfg) GetType() string { return "docs" }

func TestParseSourceURL(t *testing.T) {
	bucket, prefix, err := objectstore.ParseSourceURL("s3://artifacts/reports/q3/")
	if err != nil {
		t.Fatalf("ParseSourceURL: %v", err)
	}
	if bucket != "artifacts" {
		t.Errorf("bucket = %q", bucket)
	}
	// No leading slash: object keys have no root, and "/reports/" matches
	// nothing in a store whose keys begin "reports/".
	if prefix != "reports/q3/" {
		t.Errorf("prefix = %q, want %q", prefix, "reports/q3/")
	}

	whole, emptyPrefix, err := objectstore.ParseSourceURL(objectstore.SourceURL("artifacts", ""))
	if err != nil {
		t.Fatalf("ParseSourceURL for a whole bucket: %v", err)
	}
	if whole != "artifacts" || emptyPrefix != "" {
		t.Errorf("whole-bucket URL parsed as %q/%q", whole, emptyPrefix)
	}

	for _, bad := range []string{"", "artifacts/reports", "https://example.com/x", "s3:///reports"} {
		if _, _, err := objectstore.ParseSourceURL(bad); err == nil {
			t.Errorf("ParseSourceURL(%q) should fail", bad)
		}
	}
}

// ─── Ingest ─────────────────────────────────────────────────────────────────

func TestIngest_ProducesDocumentEntities(t *testing.T) {
	store := newCorpus(t, map[string]string{
		"reports/q3.md":    "# Q3 Review\n\nRevenue grew.\n",
		"reports/q4.md":    "# Q4 Review\n\nRevenue grew again.\n",
		"reports/logo.png": "not a document",
	})
	h := newHandler(t, store)

	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	if len(result.Ingested) != 2 {
		t.Fatalf("ingested %d objects, want 2", len(result.Ingested))
	}
	if len(result.States()) == 0 {
		t.Fatal("no entity states produced")
	}
	if result.Bucket != "artifacts" || result.Prefix != "reports/" {
		t.Errorf("result names %q/%q", result.Bucket, result.Prefix)
	}

	// The unsupported object never had its body read, and is counted.
	if counts := result.SkipCounts(); counts["unsupported_format"] != 1 {
		t.Errorf("SkipCounts = %v, want one unsupported_format", counts)
	}

	for _, ing := range result.Ingested {
		if ing.Operation != handler.OperationCreate {
			t.Errorf("%q ingested as %q, want create", ing.Key, ing.Operation)
		}
		if ing.Token == "" {
			t.Errorf("%q ingested with no change token", ing.Key)
		}
	}
}

// TestIngest_TypedStatesOnly pins the publishing contract: this source emits
// canonical entity state, so there is nothing for a normalizer pass to do and
// no RawEntity to populate.
func TestIngest_TypedStatesOnly(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nbody\n"})
	h := newHandler(t, store)

	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	for _, state := range result.States() {
		if state.ID == "" {
			t.Error("an entity state with no ID would publish nothing identifiable")
		}
		if len(state.Triples) == 0 {
			t.Errorf("%s carries no triples", state.ID)
		}
	}
}

func TestIngest_UnchangedObjectsAreNotRefetched(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nbody\n"})
	h := newHandler(t, store)
	cfg := cfgFor("artifacts", "reports/")

	if _, err := h.IngestEntityStates(t.Context(), cfg); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	// The fetch path is closed off entirely: any read on the second pass is an
	// error, not a slow path.
	store.failRead("reports/q3.md", errors.New("second pass must not read this object"))

	second, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if len(second.Ingested) != 0 {
		t.Errorf("second pass re-ingested %d objects", len(second.Ingested))
	}
	if second.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", second.Unchanged)
	}
	if len(second.Removed) != 0 {
		t.Errorf("Removed = %v, want none", second.Removed)
	}
}

func TestIngest_ReplacedObjectKeepsItsIdentity(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nfirst\n"})
	h := newHandler(t, store)
	cfg := cfgFor("artifacts", "reports/")

	first, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	firstID := first.Ingested[0].States[0].ID

	store.replace("reports/q3.md", "# Q3\n\nsecond\n", "etag-v2")

	second, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if len(second.Ingested) != 1 {
		t.Fatalf("re-ingested %d objects, want 1", len(second.Ingested))
	}
	if second.Ingested[0].Operation != handler.OperationModify {
		t.Errorf("operation = %q, want modify", second.Ingested[0].Operation)
	}
	if got := second.Ingested[0].States[0].ID; got != firstID {
		t.Errorf("a replaced object minted a sibling entity:\n  before %s\n  after  %s", firstID, got)
	}
}

func TestIngest_RemovedObjectsAreNamedWithTheirEntities(t *testing.T) {
	store := newCorpus(t, map[string]string{
		"reports/q3.md": "# Q3\n\nbody\n",
		"reports/q4.md": "# Q4\n\nbody\n",
	})
	h := newHandler(t, store)
	cfg := cfgFor("artifacts", "reports/")

	first, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	var q4ID string
	for _, ing := range first.Ingested {
		if ing.Key == "reports/q4.md" {
			q4ID = ing.States[0].ID
		}
	}

	store.remove("reports/q4.md")

	second, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if len(second.Removed) != 1 {
		t.Fatalf("Removed = %v, want one entry", second.Removed)
	}
	if second.Removed[0].Key != "reports/q4.md" {
		t.Errorf("removed key = %q", second.Removed[0].Key)
	}
	// The retraction has to name the entity the ingest published, or it
	// retracts nothing while reporting success.
	if second.Removed[0].EntityID != q4ID {
		t.Errorf("removal names %s, but ingest published %s", second.Removed[0].EntityID, q4ID)
	}
}

// TestIngest_FailedListingRetractsNothing is the end-to-end form of the
// retraction-safety contract: the listing fails, and the pass reports an error
// rather than a corpus that vanished.
func TestIngest_FailedListingRetractsNothing(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nbody\n"})
	h := newHandler(t, store)
	cfg := cfgFor("artifacts", "reports/")

	if _, err := h.IngestEntityStates(t.Context(), cfg); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	store.listErr = errors.New("connection reset mid-pagination")

	result, err := h.IngestEntityStates(t.Context(), cfg)
	if err == nil {
		t.Fatal("expected an error")
	}
	if result != nil {
		t.Errorf("a failed pass must produce no result, got %+v", result)
	}
	if h.Tracked() != 1 {
		t.Errorf("a failed pass discarded change-detection state: tracking %d keys", h.Tracked())
	}
}

// ─── The two failure modes ──────────────────────────────────────────────────

// TestIngest_UnreadableObjectIsSkippedAndCounted is one document's problem.
func TestIngest_UnreadableObjectIsSkippedAndCounted(t *testing.T) {
	store := newCorpus(t, map[string]string{
		"reports/q3.md": "# Q3\n\nbody\n",
		"reports/q4.md": "# Q4\n\nbody\n",
	})
	store.getErr = map[string]error{"reports/q4.md": storage.ErrObjectNotFound}
	h := newHandler(t, store)

	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("an unreadable object must not fail the pass: %v", err)
	}
	if len(result.Ingested) != 1 {
		t.Errorf("ingested %d objects, want the one that was readable", len(result.Ingested))
	}
	if counts := result.SkipCounts(); counts["unreadable"] != 1 {
		t.Errorf("SkipCounts = %v, want one unreadable", counts)
	}
}

// TestIngest_MissingBodyStoreAbortsThePass is the deployment's problem. Every
// document after this one would fail the same way, and a healthy-looking
// ingest of a corpus with no retrievable bodies is worse than a loud failure.
func TestIngest_MissingBodyStoreAbortsThePass(t *testing.T) {
	store := newCorpus(t, map[string]string{
		"reports/q3.md": "# Q3\n\nbody\n",
		"reports/q4.md": "# Q4\n\nbody\n",
	})
	// A doc handler with no body store configured.
	h := objectstore.New(store, doc.New(), testOrg)

	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if !errors.Is(err, doc.ErrBodyStoreRequired) {
		t.Fatalf("expected doc.ErrBodyStoreRequired, got: %v", err)
	}
	if result != nil {
		t.Errorf("an aborted pass must produce no result, got %+v", result)
	}
}

// ─── Identity ───────────────────────────────────────────────────────────────

func TestIdentity_DerivesFromBucketAndKeyOnly(t *testing.T) {
	docs := map[string]string{"reports/q3.md": "# Q3\n\nbody\n"}

	first := newHandler(t, newCorpus(t, docs))
	second := newHandler(t, newCorpus(t, docs))

	// Two handlers, two body stores in two different temp directories —
	// everything local differs. The identifiers must not.
	firstResult, err := first.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("first handler: %v", err)
	}
	secondResult, err := second.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("second handler: %v", err)
	}

	firstIDs := idsOf(firstResult.States())
	secondIDs := idsOf(secondResult.States())
	assertStrings(t, secondIDs, firstIDs)
}

func TestIdentity_SurvivesHostileKeys(t *testing.T) {
	keys := []string{
		"reports/nested/deep/q3.md",
		"reports/Q3 final draft.md",
		"reports/résumé-北京.md",
		"reports/" + strings.Repeat("very-long-segment-", 20) + "a.md",
		"reports/" + strings.Repeat("very-long-segment-", 20) + "b.md",
	}
	docs := make(map[string]string, len(keys))
	for _, key := range keys {
		docs[key] = "# Title\n\nbody\n"
	}

	h := newHandler(t, newCorpus(t, docs))
	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(result.Ingested) != len(keys) {
		t.Fatalf("ingested %d objects, want %d", len(result.Ingested), len(keys))
	}

	seen := make(map[string]string, len(keys))
	for _, ing := range result.Ingested {
		id := ing.States[0].ID
		// The substrate's own validator, not a local approximation: this is
		// the contract graph-ingest applies, and an object key is the most
		// hostile input this source ever hands it.
		if err := types.ValidateEntityID(id); err != nil {
			t.Errorf("key %q produced an ID semstreams rejects: %s: %v", ing.Key, id, err)
		}
		// And a legal NATS KV key, which is what the ID is stored under.
		if err := entityid.ValidateNATSKVKey(id); err != nil {
			t.Errorf("key %q produced an unusable entity ID %s: %v", ing.Key, id, err)
		}
		if other, clash := seen[id]; clash {
			// Two keys differing only past the truncation point must stay
			// distinct — the sanitizer suffixes a hash of the original for
			// exactly this.
			t.Errorf("keys %q and %q collapsed onto one entity ID: %s", other, ing.Key, id)
		}
		seen[id] = ing.Key
	}
}

func TestIdentity_ProjectOverrideReplacesTheBucketSlug(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nbody\n"})

	plain := newHandler(t, store)
	overridden := newHandler(t, store, objectstore.WithProject("quarterly-reports"))

	if plain.System("artifacts") == overridden.System("artifacts") {
		t.Fatal("the project override did not change the system slug")
	}
	if got := overridden.System("artifacts"); got != entityid.SystemSlug("quarterly-reports") {
		t.Errorf("System = %q, want the project slug", got)
	}
	// Two prefixes of one bucket registered as distinct projects must not
	// collide.
	other := newHandler(t, store, objectstore.WithProject("annual-reports"))
	if other.System("artifacts") == overridden.System("artifacts") {
		t.Error("two projects in one bucket share a system slug")
	}
}

// TestIdentity_ContentHashComesFromTheBytes pins that the change token never
// becomes the content hash. A composite multipart ETag is not the MD5 of the
// object, so an entity that carried it would report a hash that changes when
// the same bytes are re-uploaded with different chunking.
func TestIdentity_ContentHashComesFromTheBytes(t *testing.T) {
	const body = "# Q3 Review\n\nRevenue grew.\n"
	store := newCorpus(t, map[string]string{"reports/q3.md": body})
	// The shape S3 returns for a multipart upload.
	store.objects[0].ETag = "9bb58f26192e4ba00f01e2e7b136bbd8-4"

	h := newHandler(t, store)
	result, err := h.IngestEntityStates(t.Context(), cfgFor("artifacts", "reports/"))
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	hash := tripleValue(t, result.Ingested[0].States[0], sourcevocab.DocFileHash)
	if hash == "" {
		t.Fatal("the document carries no content hash")
	}
	if strings.Contains(hash, "-") || hash == store.objects[0].ETag {
		t.Errorf("content hash came from the ETag: %q", hash)
	}
	if want := sha256Hex(body); hash != want {
		t.Errorf("content hash = %q, want the SHA-256 of the bytes %q", hash, want)
	}
}

// ─── Read-only ──────────────────────────────────────────────────────────────

// TestReadOnly_TheInterfaceOffersNoWrite is the structural half of "an
// object-store source never writes to the bucket". The handler holds an
// ObjectStore, which declares Objects and Get and nothing else — so no ingest,
// watch, or retraction path can issue a write, because there is no method to
// call. A test that counted mutating calls would only prove the paths exercised
// that day; this cannot compile if the guarantee is removed.
func TestReadOnly_TheInterfaceOffersNoWrite(t *testing.T) {
	var iface objectstore.ObjectStore = newCorpus(t, nil)

	if _, isStore := iface.(interface {
		Put(context.Context, string, []byte) error
	}); isStore {
		t.Error("ObjectStore exposes Put")
	}
	if _, isStore := iface.(interface {
		Delete(context.Context, string) error
	}); isStore {
		t.Error("ObjectStore exposes Delete")
	}
}

// TestReadOnly_AFullCycleWritesNothing exercises ingest, change, and removal
// against a store that records any mutation, so the claim covers the paths and
// not only the type.
func TestReadOnly_AFullCycleWritesNothing(t *testing.T) {
	store := newCorpus(t, map[string]string{
		"reports/q3.md": "# Q3\n\nbody\n",
		"reports/q4.md": "# Q4\n\nbody\n",
	})
	h := newHandler(t, store)
	cfg := cfgFor("artifacts", "reports/")

	if _, err := h.IngestEntityStates(t.Context(), cfg); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	store.replace("reports/q3.md", "# Q3\n\nchanged\n", "etag-v2")
	if _, err := h.IngestEntityStates(t.Context(), cfg); err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	store.remove("reports/q4.md")
	result, err := h.IngestEntityStates(t.Context(), cfg)
	if err != nil {
		t.Fatalf("retract pass: %v", err)
	}
	if len(result.Removed) != 1 {
		t.Fatalf("expected one removal, got %v", result.Removed)
	}

	if store.writes != 0 {
		t.Errorf("the source issued %d writes against the bucket", store.writes)
	}
}

// ─── Watch ──────────────────────────────────────────────────────────────────

func TestWatch_DisabledReturnsNoChannel(t *testing.T) {
	h := newHandler(t, newCorpus(t, nil))

	events, err := h.Watch(t.Context(), sourceCfg{url: "s3://artifacts/reports/", watch: false})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if events != nil {
		t.Error("watching is off; there should be no channel")
	}
}

func TestWatch_EmitsTypedEventsForChangesAndRemovals(t *testing.T) {
	store := newCorpus(t, map[string]string{"reports/q3.md": "# Q3\n\nbody\n"})
	h := newHandler(t, store, objectstore.WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	events, err := h.Watch(ctx, sourceCfg{url: "s3://artifacts/reports/", watch: true})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	created := awaitEvent(t, events)
	if created.Operation != handler.OperationCreate {
		t.Errorf("first event is %q, want create", created.Operation)
	}
	if created.Path != "reports/q3.md" {
		t.Errorf("event path = %q", created.Path)
	}
	if len(created.EntityStates) == 0 {
		t.Error("a create event carries no entity states")
	}
	// Typed state only: a normalizer pass has nothing to do with this source.
	if len(created.Entities) != 0 {
		t.Errorf("event populated %d RawEntity values", len(created.Entities))
	}

	store.remove("reports/q3.md")

	deleted := awaitEvent(t, events)
	if deleted.Operation != handler.OperationDelete {
		t.Errorf("second event is %q, want delete", deleted.Operation)
	}
	if deleted.Path != "reports/q3.md" {
		t.Errorf("delete event path = %q", deleted.Path)
	}
}

func TestWatch_ClosesOnContextCancel(t *testing.T) {
	h := newHandler(t, newCorpus(t, nil), objectstore.WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(t.Context())
	events, err := h.Watch(ctx, sourceCfg{url: "s3://artifacts/reports/", watch: true})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	cancel()

	select {
	case _, open := <-events:
		if open {
			t.Error("expected the channel to be closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watch channel was never closed")
	}
}

func awaitEvent(t *testing.T, events <-chan handler.ChangeEvent) handler.ChangeEvent {
	t.Helper()
	select {
	case event, open := <-events:
		if !open {
			t.Fatal("the watch channel closed before an event arrived")
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a watch event")
		return handler.ChangeEvent{}
	}
}

func idsOf(states []*handler.EntityState) []string {
	out := make([]string, 0, len(states))
	for _, state := range states {
		out = append(out, state.ID)
	}
	sort.Strings(out)
	return out
}

func tripleValue(t *testing.T, state *handler.EntityState, predicate string) string {
	t.Helper()
	for _, triple := range state.Triples {
		if triple.Predicate == predicate {
			if s, isString := triple.Object.(string); isString {
				return s
			}
		}
	}
	return ""
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
