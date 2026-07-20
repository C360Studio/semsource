package doc_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	source "github.com/c360studio/semsource/source/vocabulary"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sourceConfig adapts test parameters to handler.SourceConfig.
type sourceConfig struct {
	typ   string
	path  string
	url   string
	watch bool
	paths []string
}

func (s sourceConfig) GetType() string             { return s.typ }
func (s sourceConfig) GetPath() string             { return s.path }
func (s sourceConfig) GetPaths() []string          { return s.paths }
func (s sourceConfig) GetURL() string              { return s.url }
func (s sourceConfig) GetBranch() string           { return "" }
func (s sourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s sourceConfig) GetKeyframeMode() string     { return "" }
func (s sourceConfig) GetKeyframeInterval() string { return "" }
func (s sourceConfig) GetSceneThreshold() float64  { return 0 }

// bodyStoreInstance is the StorageReference.StorageInstance every test wires its
// body store under, matching the name doc-source uses in production.
const bodyStoreInstance = "objectstore"

// docsHandler builds a doc handler wired to a fresh in-memory body store, and
// returns both. Every test that ingests goes through it: the verbatim body store
// is MANDATORY, so a handler built without one fails the walk outright with
// dochandler.ErrBodyStoreRequired rather than quietly emitting bodyless
// passages. Tests that do not care about blobs discard the store.
func docsHandler(t *testing.T) (*dochandler.Handler, *memStore) {
	t.Helper()
	store := newMemStore()
	return dochandler.New(dochandler.WithBodyStore(store, bodyStoreInstance)), store
}

// writeMD writes a markdown file and returns its absolute path.
func writeMD(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// tripleValue returns the string object of the first triple on state carrying
// predicate, failing the test when the triple is absent or its object is not a
// string.
func tripleValue(t *testing.T, state *handler.EntityState, predicate string) string {
	t.Helper()
	for _, tr := range state.Triples {
		if tr.Predicate == predicate {
			v, ok := tr.Object.(string)
			if !ok {
				t.Fatalf("triple %q on %s: object is %T (%v), want string", predicate, state.ID, tr.Object, tr.Object)
			}
			return v
		}
	}
	t.Fatalf("no triple with predicate %q on entity %s", predicate, state.ID)
	return ""
}

// tripleInt returns the int object of the first triple on state carrying
// predicate. The passage counters (DocChunkIndex, DocChunkCount) are emitted as
// real ints, not stringified ones, so a consumer can compare them numerically.
func tripleInt(t *testing.T, state *handler.EntityState, predicate string) int {
	t.Helper()
	for _, tr := range state.Triples {
		if tr.Predicate == predicate {
			v, ok := tr.Object.(int)
			if !ok {
				t.Fatalf("triple %q on %s: object is %T (%v), want int", predicate, state.ID, tr.Object, tr.Object)
			}
			return v
		}
	}
	t.Fatalf("no triple with predicate %q on entity %s", predicate, state.ID)
	return 0
}

// optionalTripleValue returns the string object of the first triple carrying
// predicate and whether such a triple exists. Unlike tripleValue it does not
// fail: absence is the thing under test for the conditionally-emitted
// predicates (source.DocSection is only stamped on a headed passage).
func optionalTripleValue(state *handler.EntityState, predicate string) (string, bool) {
	for _, tr := range state.Triples {
		if tr.Predicate == predicate {
			v, ok := tr.Object.(string)
			return v, ok
		}
	}
	return "", false
}

// filePathSet collects the source.DocFilePath value of every state, so a test
// can assert exactly which files entered the corpus. Parents and passages both
// carry the path (deliberately — the staleness pass groups by it), so the set is
// the same whichever population it is handed.
func filePathSet(t *testing.T, states []*handler.EntityState) map[string]bool {
	t.Helper()
	seen := make(map[string]bool, len(states))
	for _, state := range states {
		seen[tripleValue(t, state, source.DocFilePath)] = true
	}
	return seen
}

// parentStates returns the document entities: one per ingested file, carrying
// identity, title, hash and chunk count. A test about which FILES entered the
// corpus counts these, not the raw state slice — one file now yields a parent
// plus N passages.
func parentStates(states []*handler.EntityState) []*handler.EntityState {
	return statesOfDocType(states, "document")
}

// passageStates returns the passage entities: the retrievable slices of the
// documents, in the order the handler emitted them (parent first, then its
// passages in ordinal order, per file).
func passageStates(states []*handler.EntityState) []*handler.EntityState {
	return statesOfDocType(states, "passage")
}

func statesOfDocType(states []*handler.EntityState, docType string) []*handler.EntityState {
	var out []*handler.EntityState
	for _, state := range states {
		if docTypeOf(state) == docType {
			out = append(out, state)
		}
	}
	return out
}

// docTypeOf reports a state's source.DocType, or "" when the triple is absent or
// not a string. It classifies rather than asserts, so that a test can say "every
// state is a document or a passage" and name the offender itself.
func docTypeOf(state *handler.EntityState) string {
	for _, tr := range state.Triples {
		if tr.Predicate == source.DocType {
			if v, ok := tr.Object.(string); ok {
				return v
			}
			return ""
		}
	}
	return ""
}

// assertOffloadedBody pins the offloaded shape on one PASSAGE and returns its
// body key: handle triples present, no inline content, and StorageRef pointing
// at the SAME blob as the handle (ADR-063 unification). Passages are the only
// population that carries a body — see assertBodyless for the parent.
func assertOffloadedBody(t *testing.T, state *handler.EntityState) string {
	t.Helper()

	instance, key := "", ""
	hasContent := false
	for _, tr := range state.Triples {
		switch tr.Predicate {
		case source.DocBodyStore:
			instance, _ = tr.Object.(string)
		case source.DocBodyKey:
			key, _ = tr.Object.(string)
		case source.DocContent:
			hasContent = true
		}
	}
	if instance != bodyStoreInstance || key == "" {
		t.Fatalf("body handle triples on %s (%s): instance=%q key=%q, want instance=%q and a non-empty key",
			state.ID, docTypeOf(state), instance, key, bodyStoreInstance)
	}
	if hasContent {
		t.Errorf("%s (%s) kept an inline %s triple; the body lives in the store, never in a triple",
			state.ID, docTypeOf(state), source.DocContent)
	}
	if state.StorageRef == nil {
		t.Fatalf("StorageRef on %s (%s) = nil, want a reference to the offloaded blob %q",
			state.ID, docTypeOf(state), key)
	}
	if state.StorageRef.StorageInstance != instance {
		t.Errorf("StorageRef.StorageInstance on %s = %q, want %q (the body handle instance)",
			state.ID, state.StorageRef.StorageInstance, instance)
	}
	if state.StorageRef.Key != key {
		t.Errorf("StorageRef.Key on %s = %q, want %q (the body handle key)", state.ID, state.StorageRef.Key, key)
	}
	return key
}

// assertBodyless pins the parent document's contract: it carries NO body in any
// form — no handle triples, no inline content, no StorageRef. A whole-file body
// on the parent would return the same prose twice (once as the document, again
// as its passages) and keep the averaged, truncated whole-file vector that
// passages exist to replace.
func assertBodyless(t *testing.T, state *handler.EntityState) {
	t.Helper()

	for _, tr := range state.Triples {
		switch tr.Predicate {
		case source.DocContent:
			t.Errorf("%s (%s) carries a %s triple with object %v; the parent holds no body",
				state.ID, docTypeOf(state), source.DocContent, tr.Object)
		case source.DocBodyStore, source.DocBodyKey:
			t.Errorf("%s (%s) carries the body handle triple %s = %v; the parent holds no body",
				state.ID, docTypeOf(state), tr.Predicate, tr.Object)
		}
	}
	if state.StorageRef != nil {
		t.Errorf("StorageRef on %s (%s) = %+v, want nil; the parent holds no body",
			state.ID, docTypeOf(state), state.StorageRef)
	}
}

// passageBody fetches a passage's verbatim body from the store by its handle.
// Bodies live only in the store now — there is no inline triple to read — so
// this is how a test looks at a passage's text.
func passageBody(t *testing.T, store *memStore, state *handler.EntityState) string {
	t.Helper()
	key := tripleValue(t, state, source.DocBodyKey)
	blob, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("body key %q of %s (%s) is not in the store: %v", key, state.ID, docTypeOf(state), err)
	}
	return string(blob)
}

// hasPredicate reports whether state carries any triple with predicate.
func hasPredicate(state *handler.EntityState, predicate string) bool {
	for _, tr := range state.Triples {
		if tr.Predicate == predicate {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// SourceType / Supports
// ---------------------------------------------------------------------------

func TestDocHandler_SourceType(t *testing.T) {
	h := dochandler.New()
	if got := h.SourceType(); got != "docs" {
		t.Errorf("SourceType() = %q, want %q", got, "docs")
	}
}

func TestDocHandler_Supports(t *testing.T) {
	h := dochandler.New()

	tests := []struct {
		typ  string
		want bool
	}{
		{"docs", true},
		{"doc", false},
		{"git", false},
		{"", false},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: tt.typ}
		if got := h.Supports(cfg); got != tt.want {
			t.Errorf("Supports(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Watch — disabled returns nil (no fsnotify needed)
// ---------------------------------------------------------------------------

func TestDocHandler_Watch_WatchDisabledReturnsNil(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	if ch != nil {
		t.Error("Watch() should return nil channel when watch is disabled")
	}
}

// ---------------------------------------------------------------------------
// IngestEntityStates — the live path
// ---------------------------------------------------------------------------

// TestDocHandler_IngestEntityStates_ReturnsStates pins the corpus census: the
// question is which FILES were ingested, so it counts parents — one per file —
// and separately requires each file to have contributed at least one passage.
// The content indexing profile is asserted over the whole population, parents
// and passages alike, because every one of them is embedded.
func TestDocHandler_IngestEntityStates_ReturnsStates(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "readme.md", "# Hello\n\nContent here.")
	writeMD(t, dir, "notes.txt", "Plain text.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}

	parents := parentStates(states)
	if len(parents) != 2 {
		t.Fatalf("parent state count: got %d, want 2 (readme.md + notes.txt); %d states in total",
			len(parents), len(states))
	}
	passages := passageStates(states)
	if len(passages) < 2 {
		t.Errorf("passage state count: got %d, want at least 2 (each of the 2 files yields at least one passage)",
			len(passages))
	}
	if got := len(parents) + len(passages); got != len(states) {
		t.Errorf("classified %d of %d states; every state must be a document or a passage", got, len(states))
	}
	for _, state := range states {
		if state.IndexingProfile != semvocab.IndexingProfileContent {
			t.Errorf("IndexingProfile on %s (%s) = %q, want %q",
				state.ID, docTypeOf(state), state.IndexingProfile, semvocab.IndexingProfileContent)
		}
	}
}

// TestDocHandler_IngestEntityStates_BodyHandle pins the offload wiring: each
// PASSAGE body goes to a single CONTENT blob referenced two ways — the
// DocBodyStore/DocBodyKey handle triples (the docs lens hydrates by handle,
// ADR-062) and EntityState.StorageRef (graph-embedding embeds the body via the
// shared StoreRegistry, ADR-063) — while the parent carries no body at all.
//
// The store must hold nothing but the blobs those passage handles reference.
// Content-addressing collapses identical bodies (here the lone passage IS the
// whole file), so the blob count is derived from the distinct referenced keys
// rather than hard-coded; a parent blob would show up as an extra key.
func TestDocHandler_IngestEntityStates_BodyHandle(t *testing.T) {
	dir := t.TempDir()
	body := "# Retry\n\nUse exponential backoff."
	writeMD(t, dir, "retry.md", body)

	h, store := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}

	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (retry.md); %d states in total", len(parents), len(states))
	}
	passages := passageStates(states)
	if len(passages) == 0 {
		t.Fatalf("passage state count: got 0, want at least 1 for retry.md; %d states in total", len(states))
	}

	assertBodyless(t, parents[0])

	referenced := make(map[string]bool)
	for _, passage := range passages {
		referenced[assertOffloadedBody(t, passage)] = true
	}
	if got := store.keys(); len(got) != len(referenced) {
		t.Fatalf("store blob count: got %d %v, want %d (one blob per distinct key a passage references, and nothing else)",
			len(got), got, len(referenced))
	}

	// The passage blobs tile the document: nothing is lost by the split.
	var joined strings.Builder
	for _, passage := range passages {
		joined.WriteString(passageBody(t, store, passage))
	}
	if joined.String() != body {
		t.Errorf("passage bodies concatenated = %q, want the document %q", joined.String(), body)
	}
}

// TestDocHandler_IngestEntityStates_ParentCarriesNoBody pins the parent's half
// of the split. A whole-file body on the parent would return the same prose
// twice — once as the document, again as its passages — and would keep the
// averaged, truncated whole-file vector that passages exist to replace. Asserted
// on a document with SEVERAL passages, because a single-passage document cannot
// distinguish "the parent has no body" from "the parent's body happens to equal
// the only passage's".
func TestDocHandler_IngestEntityStates_ParentCarriesNoBody(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "usage", 4)+
		headedSection("Limits", "limits", 4))

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}

	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (guide.md); %d states in total", len(parents), len(states))
	}
	if passages := passageStates(states); len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2, otherwise a parent body is indistinguishable from the sole passage's",
			len(passages))
	}
	assertBodyless(t, parents[0])
}

// TestDocHandler_IngestEntityStates_ParentStaysNavigable is the complement to
// _ParentCarriesNoBody: dropping the body must not hollow the parent out. It
// remains the stable navigational node — name-resolvable by title, locatable by
// path, change-detectable by hash, and carrying the DocChunkCount the staleness
// pass compares passage indices against.
func TestDocHandler_IngestEntityStates_ParentStaysNavigable(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+headedSection("Usage", "usage", 4))

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (guide.md); %d states in total", len(parents), len(states))
	}
	parent := parents[0]

	if got := tripleValue(t, parent, source.DcTitle); got != "Guide" {
		t.Errorf("%s on %s = %q, want %q; without a title the parent is not name-resolvable",
			source.DcTitle, parent.ID, got, "Guide")
	}
	if got := tripleValue(t, parent, source.DocFilePath); got != "guide.md" {
		t.Errorf("%s on %s = %q, want %q; without a path the parent is not locatable and the staleness pass skips it",
			source.DocFilePath, parent.ID, got, "guide.md")
	}
	if got := tripleValue(t, parent, source.DocMimeType); got != "text/markdown" {
		t.Errorf("%s on %s = %q, want %q", source.DocMimeType, parent.ID, got, "text/markdown")
	}
	if got := tripleValue(t, parent, source.DocFileHash); got == "" {
		t.Errorf("%s on %s is empty; without it a content edit is undetectable", source.DocFileHash, parent.ID)
	}
	if got, want := tripleInt(t, parent, source.DocChunkCount), len(passageStates(states)); got != want {
		t.Errorf("%s on %s = %d, want %d (the number of passages emitted for guide.md)",
			source.DocChunkCount, parent.ID, got, want)
	}
	if got := tripleValue(t, parent, source.DocType); got != "document" {
		t.Errorf("%s on %s = %q, want %q", source.DocType, parent.ID, got, "document")
	}
}

// TestDocHandler_IngestEntityStates_EveryPassageIsHydratable pins the invariant
// that makes the parent's emptiness safe: if the parent holds no body, then
// every passage must hold one, or that stretch of the document is reachable
// through no entity at all. One unhydratable passage is a silent hole in the
// corpus, which is exactly the shape the inline fallback used to hide.
func TestDocHandler_IngestEntityStates_EveryPassageIsHydratable(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "usage", 4)+
		headedSection("Limits", "limits", 4))
	writeMD(t, dir, "short.md", "# Short\n\nOne line.")

	h, store := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	passages := passageStates(states)
	if len(passages) < 3 {
		t.Fatalf("passage state count: got %d, want at least 3 (a multi-section document plus a one-liner); %d states in total",
			len(passages), len(states))
	}

	for _, passage := range passages {
		key := assertOffloadedBody(t, passage)
		if body := passageBody(t, store, passage); body == "" {
			t.Errorf("passage %s hydrates to an empty body from key %q; every passage must carry retrievable text",
				passage.ID, key)
		}
	}
}

// TestDocHandler_IngestEntityStates_NoRetiredSummaryTriple pins the removal of
// source.doc.summary at the producer. The predicate was registered with a
// salience weight of 2.0, so a re-introduced triple would not merely be inert —
// it would float whatever carried it above everything else. Named by its wire
// string because the constant is gone, and checked over BOTH populations.
func TestDocHandler_IngestEntityStates_NoRetiredSummaryTriple(t *testing.T) {
	const retiredSummary = "source.doc.summary"

	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+headedSection("Usage", "usage", 4))
	writeMD(t, dir, "notes.txt", "Plain text with no heading at all.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned; the assertion below would pass vacuously")
	}

	for _, state := range states {
		for _, tr := range state.Triples {
			if tr.Predicate == retiredSummary {
				t.Errorf("%s (%s) emits the retired predicate %q with object %v; it must appear on no entity",
					state.ID, docTypeOf(state), retiredSummary, tr.Object)
			}
		}
	}
}

// TestDocHandler_IngestEntityStates_DocumentTextStoredExactlyOnce is the
// storage-side statement of the split: the document's text reaches the store
// once, through its passages, and nothing duplicates it.
//
// Two independent duplications are ruled out. The blob COUNT must equal the
// number of distinct keys the passages reference, so a parent-level blob (which
// no handle points at) shows up as a surplus key. And no single blob may hold
// the whole file, so a parent blob that happened to be content-addressed onto a
// passage's key — collapsing the count check — is still caught.
func TestDocHandler_IngestEntityStates_DocumentTextStoredExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	content := "# Guide\n\n" + prose("intro", 4) +
		headedSection("Usage", "usage", 4) +
		headedSection("Limits", "limits", 4)
	writeMD(t, dir, "guide.md", content)

	h, store := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	passages := passageStates(states)
	if len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2; with one passage a whole-file blob is legitimate and this test proves nothing",
			len(passages))
	}

	// The passages tile the document: concatenating their bodies reproduces the
	// file byte for byte, so the split loses nothing.
	referenced := make(map[string]bool, len(passages))
	var joined strings.Builder
	for _, passage := range passages {
		referenced[tripleValue(t, passage, source.DocBodyKey)] = true
		joined.WriteString(passageBody(t, store, passage))
	}
	if joined.String() != content {
		t.Errorf("passage bodies concatenated to %d bytes, want the whole %d-byte document",
			joined.Len(), len(content))
	}

	// Nothing in the store is unreferenced: a parent blob would be a surplus key.
	keys := store.keys()
	if len(keys) != len(referenced) {
		t.Errorf("store holds %d blobs %v, want %d (one per distinct passage key; a surplus blob is a duplicated body)",
			len(keys), keys, len(referenced))
	}
	for _, key := range keys {
		if !referenced[key] {
			t.Errorf("store holds blob %q that no passage handle references; the parent must offload nothing", key)
		}
		blob, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("store key %q reported by keys() but not gettable: %v", key, err)
		}
		if string(blob) == content {
			t.Errorf("store blob %q holds the entire %d-byte document; the text must reach the store only in passage-sized pieces",
				key, len(content))
		}
	}
}

func TestDocHandler_IngestEntityStates_IDHasSixParts(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nBody.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	parts := strings.Split(states[0].ID, ".")
	if len(parts) != 6 {
		t.Errorf("entity ID has %d parts, want 6: %q", len(parts), states[0].ID)
	}
	if parts[0] != "acme" {
		t.Errorf("ID org segment = %q, want %q", parts[0], "acme")
	}
	if parts[2] != "web" {
		t.Errorf("ID domain segment = %q, want %q", parts[2], "web")
	}
	if parts[4] != "doc" {
		t.Errorf("ID type segment = %q, want %q", parts[4], "doc")
	}
}

// TestDocHandler_IngestEntityStates_TriplesUseVocabularyPredicates pins the
// parent's predicate set exactly: the six facts it carries, and the body
// predicates it must not.
func TestDocHandler_IngestEntityStates_TriplesUseVocabularyPredicates(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Doc\nSome content.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (doc.md); %d states in total", len(parents), len(states))
	}
	parent := parents[0]

	wantPredicates := []string{
		source.DocType,
		source.DocFilePath,
		source.DocMimeType,
		source.DocFileHash,
		source.DcTitle,
		source.DocChunkCount,
	}
	for _, p := range wantPredicates {
		if !hasPredicate(parent, p) {
			t.Errorf("parent %s is missing a triple with predicate %q", parent.ID, p)
		}
	}

	unwantedPredicates := []string{
		source.DocContent,
		source.DocBodyStore,
		source.DocBodyKey,
	}
	for _, p := range unwantedPredicates {
		if hasPredicate(parent, p) {
			t.Errorf("parent %s carries a triple with predicate %q; the parent holds no body", parent.ID, p)
		}
	}
}

func TestDocHandler_IngestEntityStates_TriplesAreSelfSubject(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Doc\nSome content.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if err := handler.ValidateSelfSubjectStates(states); err != nil {
		t.Fatalf("ValidateSelfSubjectStates() error: %v", err)
	}
	if err := handler.ValidateEntityStateIDs(states); err != nil {
		t.Fatalf("ValidateEntityStateIDs() error: %v", err)
	}
}

func TestDocHandler_IngestEntityStates_DeterministicID(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nContent.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states1, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("first IngestEntityStates() error: %v", err)
	}
	states2, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("second IngestEntityStates() error: %v", err)
	}
	if len(states1) == 0 || len(states2) == 0 {
		t.Fatal("expected states from both calls")
	}
	if states1[0].ID != states2[0].ID {
		t.Errorf("entity ID not deterministic: %q vs %q", states1[0].ID, states2[0].ID)
	}
}

// TestDocHandler_IngestEntityStates_IDStableAcrossContentChange pins the
// entity-staleness spec D3 contract at the typed-EntityState path: an edit
// re-ingests the SAME entity (in-place replace), never orphaning the prior
// version under a new ID — the collision-prone content-hash-prefix instance
// is retired.
func TestDocHandler_IngestEntityStates_IDStableAcrossContentChange(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "doc.md", "# First\nOriginal content.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states1, _ := h.IngestEntityStates(context.Background(), cfg, "acme")

	if err := os.WriteFile(path, []byte("# Second\nDifferent content."), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	states2, _ := h.IngestEntityStates(context.Background(), cfg, "acme")

	if len(states1) == 0 || len(states2) == 0 {
		t.Fatal("expected states from both ingests")
	}
	if states1[0].ID != states2[0].ID {
		t.Errorf("entity ID must stay stable across a content edit (D3): %q vs %q",
			states1[0].ID, states2[0].ID)
	}
}

// TestDocHandler_IngestEntityStates_TitleFromFirstHeading pins the title
// extraction: the DcTitle triple carries the text of the document's first
// markdown H1, not the filename.
func TestDocHandler_IngestEntityStates_TitleFromFirstHeading(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Great Doc\n\nIntro paragraph.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	if got := tripleValue(t, states[0], source.DcTitle); got != "My Great Doc" {
		t.Errorf("%s = %q, want %q (text of the first H1)", source.DcTitle, got, "My Great Doc")
	}
}

// TestDocHandler_IngestEntityStates_NoTitleFallback pins the fallback: a
// document with no markdown H1 still gets a DcTitle triple, carrying the
// filename stem.
func TestDocHandler_IngestEntityStates_NoTitleFallback(t *testing.T) {
	dir := t.TempDir()
	// Plain text file with no markdown heading.
	writeMD(t, dir, "notes.txt", "Just some notes without a heading.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	if got := tripleValue(t, states[0], source.DcTitle); got != "notes" {
		t.Errorf("%s = %q, want %q (filename stem fallback when there is no H1)",
			source.DcTitle, got, "notes")
	}
}

// TestDocHandler_IngestEntityStates_FiltersByExtension pins the corpus
// boundary: only known document extensions produce entity states — a source
// file sitting in the same directory never enters the docs corpus. The census
// is over parents (one per file); the exclusion is checked over every state,
// parents and passages alike, since a passage carries its parent's path and
// would smuggle an excluded file in just as effectively.
func TestDocHandler_IngestEntityStates_FiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "valid.md", "# Doc\nContent.")
	writeMD(t, dir, "guide.mdx", "# MDX Guide\nSome MDX content.")
	writeMD(t, dir, "spec.adoc", "= AsciiDoc Spec\nSome AsciiDoc content.")
	// Non-doc file — should be ignored by the doc handler.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if parents := parentStates(states); len(parents) != 3 {
		t.Fatalf("parent state count: got %d, want 3 (.adoc + .md + .mdx); %d states in total",
			len(parents), len(states))
	}

	seen := filePathSet(t, states)
	for _, want := range []string{"valid.md", "guide.mdx", "spec.adoc"} {
		if !seen[want] {
			t.Errorf("expected %s %q among the states, not found; seen: %v", source.DocFilePath, want, seen)
		}
	}
	if seen["main.go"] {
		t.Errorf("main.go must not enter the docs corpus; seen: %v", seen)
	}
	if len(seen) != 3 {
		t.Errorf("distinct %s values: got %d %v, want 3 (no file beyond the three documents)",
			source.DocFilePath, len(seen), seen)
	}
}

// TestDocHandler_IngestEntityStates_MultiplePaths pins multi-root ingestion:
// every configured root is walked and contributes its documents.
func TestDocHandler_IngestEntityStates_MultiplePaths(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	writeMD(t, dirA, "alpha.md", "# Alpha\nContent in dirA.")
	writeMD(t, dirA, "beta.txt", "Plain text in dirA.")
	writeMD(t, dirB, "gamma.md", "# Gamma\nContent in dirB.")

	h, _ := docsHandler(t)
	// GetPath returns paths[0] when paths is non-empty; GetPaths returns both dirs.
	cfg := sourceConfig{
		typ:   "docs",
		path:  dirA,
		paths: []string{dirA, dirB},
	}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}

	// Three files total: two in dirA, one in dirB — one parent each.
	if parents := parentStates(states); len(parents) != 3 {
		t.Fatalf("parent state count: got %d, want 3 (alpha.md + beta.txt in dirA, gamma.md in dirB); %d states in total",
			len(parents), len(states))
	}

	// The file paths are root-relative, so both roots having contributed is
	// what distinguishes a full walk from a single-root one.
	seen := filePathSet(t, states)
	for _, want := range []string{"alpha.md", "beta.txt", "gamma.md"} {
		if !seen[want] {
			t.Errorf("expected %s %q among the states, not found; seen: %v", source.DocFilePath, want, seen)
		}
	}
}

// TestDocHandler_IngestEntityStates_ContentChangeUpdatesHash is the complement
// to _IDStableAcrossContentChange: identity holds across an edit, but the
// DocFileHash triple must still track the new bytes — otherwise a stable ID
// would mean change detection silently stops working.
func TestDocHandler_IngestEntityStates_ContentChangeUpdatesHash(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "doc.md", "# First\nOriginal content.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states1, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("first IngestEntityStates() error: %v", err)
	}

	// Modify the file.
	if err := os.WriteFile(path, []byte("# Second\nDifferent content entirely."), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	states2, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("second IngestEntityStates() error: %v", err)
	}

	if len(states1) == 0 || len(states2) == 0 {
		t.Fatal("expected states from both ingests")
	}

	if states1[0].ID != states2[0].ID {
		t.Fatalf("entity ID must stay stable across a content edit (D3): %q vs %q",
			states1[0].ID, states2[0].ID)
	}

	hash1 := tripleValue(t, states1[0], source.DocFileHash)
	hash2 := tripleValue(t, states2[0], source.DocFileHash)
	if hash1 == "" || hash2 == "" {
		t.Fatalf("%s must be non-empty on both ingests: %q vs %q", source.DocFileHash, hash1, hash2)
	}
	if hash1 == hash2 {
		t.Errorf("%s must change when the file content changes, got %q both times",
			source.DocFileHash, hash1)
	}
}

func TestDocHandler_IngestEntityStates_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("state count: got %d, want 0 for empty dir", len(states))
	}
}

func TestDocHandler_IngestEntityStates_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nContent.")

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := h.IngestEntityStates(ctx, cfg, "acme")
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Verbatim body offload
// ---------------------------------------------------------------------------

// memStore is an in-memory storage.Store for testing.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }

func (s *memStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return d, nil
}

func (s *memStore) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *memStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *memStore) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

// TestDocHandler_IngestEntityStates_StoreFailureIsFatal replaces the former
// _StoreFailureFallsBackToInline. There is no inline fallback any more: a
// passage without a handle is a passage whose body cannot be hydrated and whose
// text never reaches the semantic index, so a store that cannot be written to
// must fail the ingest instead of reporting a healthy walk over a corpus with no
// retrievable bodies. The states returned alongside the error must be empty —
// a caller that publishes them anyway would put bodyless entities in the graph.
func TestDocHandler_IngestEntityStates_StoreFailureIsFatal(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Doc\n"+strings.Repeat("word ", 200))

	h := dochandler.NewWithOrg("acme", dochandler.WithBodyStore(&failStore{}, bodyStoreInstance))
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err == nil {
		t.Fatalf("IngestEntityStates() error = nil with a failing body store, want an error; got %d states instead",
			len(states))
	}
	if len(states) != 0 {
		t.Errorf("IngestEntityStates() returned %d states alongside its error, want 0; a caller that publishes them stores bodyless entities",
			len(states))
	}
	if !strings.Contains(err.Error(), "simulated store failure") {
		t.Errorf("IngestEntityStates() error = %v, want it to wrap the store's own failure (%q)",
			err, "simulated store failure")
	}
}

// failStore always returns an error from Put.
type failStore struct{}

func (s *failStore) Put(context.Context, string, []byte) error {
	return fmt.Errorf("simulated store failure")
}
func (s *failStore) Get(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *failStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (s *failStore) Delete(context.Context, string) error           { return nil }

// TestDocHandler_IngestEntityStates_NoStoreIsFatal replaces the former
// _NoStoreAllInline. A handler with no body store cannot produce a hydratable
// passage at all, so it reports ErrBodyStoreRequired rather than emitting a
// corpus of unretrievable entities. The sentinel is matched with errors.Is
// because doc-source distinguishes it from an ordinary per-file read error: this
// one means the DEPLOYMENT is broken, so it aborts the whole walk.
func TestDocHandler_IngestEntityStates_NoStoreIsFatal(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "large.md", "# Large Doc\n"+strings.Repeat("word ", 200))

	h := dochandler.NewWithOrg("acme")
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err == nil {
		t.Fatalf("IngestEntityStates() error = nil with no body store, want %v; got %d states instead",
			dochandler.ErrBodyStoreRequired, len(states))
	}
	if !errors.Is(err, dochandler.ErrBodyStoreRequired) {
		t.Errorf("IngestEntityStates() error = %v, want it to wrap %v (errors.Is); doc-source keys the abort on that sentinel",
			err, dochandler.ErrBodyStoreRequired)
	}
	if len(states) != 0 {
		t.Errorf("IngestEntityStates() returned %d states alongside its error, want 0", len(states))
	}
}

// TestDocHandler_IngestEntityStates_UnreadableFileIsSkippedNotFatal pins the
// other side of that distinction, which the abort must not swallow: ONE
// unreadable document is that document's problem, so the walk skips it and every
// other document in the corpus still lands. A dangling symlink is the fixture
// because filepath.Walk lstats — it is reported as a regular .md file — while
// the subsequent read fails, and unlike a chmod-0 file it stays unreadable when
// the suite runs as root.
func TestDocHandler_IngestEntityStates_UnreadableFileIsSkippedNotFatal(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "good.md", "# Good\n\nThis document is readable.")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist.md"), filepath.Join(dir, "broken.md")); err != nil {
		t.Skipf("cannot create a symlink on this platform: %v", err)
	}

	h, _ := docsHandler(t)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error = %v, want nil; one unreadable document must not fail the walk", err)
	}

	seen := filePathSet(t, states)
	if !seen["good.md"] {
		t.Errorf("good.md is missing from the corpus; the unreadable sibling aborted the walk. Paths seen: %v", seen)
	}
	if seen["broken.md"] {
		t.Errorf("broken.md entered the corpus; an unreadable document must be skipped, not ingested empty. Paths seen: %v", seen)
	}
}
