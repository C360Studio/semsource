package doc_test

import (
	"context"
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

// assertOffloadedBody pins the offloaded shape on one state — parent or passage
// alike — and returns the body key: handle triples present, inline content
// dropped, and StorageRef pointing at the SAME blob as the handle (ADR-063
// unification).
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
	if instance != "objectstore" || key == "" {
		t.Fatalf("body handle triples on %s (%s): instance=%q key=%q, want instance=%q and a non-empty key",
			state.ID, docTypeOf(state), instance, key, "objectstore")
	}
	if hasContent {
		t.Errorf("%s (%s) kept an inline %s triple; it must be dropped when the body is offloaded",
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

// assertInlineBody is the complement of assertOffloadedBody: the state kept its
// body inline, with neither a handle nor a StorageRef.
func assertInlineBody(t *testing.T, state *handler.EntityState) {
	t.Helper()

	if state.StorageRef != nil {
		t.Errorf("StorageRef on %s (%s) = %+v, want nil when the body is not offloaded",
			state.ID, docTypeOf(state), state.StorageRef)
	}
	hasContent, hasHandle := false, false
	for _, tr := range state.Triples {
		switch tr.Predicate {
		case source.DocContent:
			hasContent = true
		case source.DocBodyStore, source.DocBodyKey:
			hasHandle = true
		}
	}
	if !hasContent {
		t.Errorf("%s (%s) has no %s triple; the body must stay inline when it is not offloaded",
			state.ID, docTypeOf(state), source.DocContent)
	}
	if hasHandle {
		t.Errorf("%s (%s) carries body handle triples; they must be absent when the body is not offloaded",
			state.ID, docTypeOf(state))
	}
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

	h := dochandler.New()
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

// TestDocHandler_IngestEntityStates_BodyHandle: with a fusion body store, every
// entity's body is offloaded to a single CONTENT blob wired two ways — the
// DocBodyStore/DocBodyKey handle triples (docs lens hydrates by handle, ADR-062)
// and EntityState.StorageRef (graph-embedding embeds the body via the shared
// StoreRegistry, ADR-063). The inline DocContent triple is dropped.
//
// The offload contract now covers BOTH populations, so both are asserted: the
// parent's blob is the document byte for byte, and the passage blobs concatenate
// back to that same document. The store must hold nothing but the blobs those
// handles reference — content-addressing collapses identical bodies (here the
// lone passage IS the whole file), so the blob count is derived from the
// distinct referenced keys rather than hard-coded.
func TestDocHandler_IngestEntityStates_BodyHandle(t *testing.T) {
	dir := t.TempDir()
	body := "# Retry\n\nUse exponential backoff."
	writeMD(t, dir, "retry.md", body)

	store := newMemStore()
	h := dochandler.New(dochandler.WithBodyStore(store, "objectstore"))
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

	referenced := make(map[string]bool)
	for _, state := range states {
		referenced[assertOffloadedBody(t, state)] = true
	}
	if got := store.keys(); len(got) != len(referenced) {
		t.Fatalf("store blob count: got %d %v, want %d (one blob per distinct referenced key)",
			len(got), got, len(referenced))
	}

	// The parent's blob is the document byte for byte.
	parentKey := tripleValue(t, parents[0], source.DocBodyKey)
	got, err := store.Get(context.Background(), parentKey)
	if err != nil || string(got) != body {
		t.Fatalf("parent offloaded body = %q (err %v); want the document %q", got, err, body)
	}

	// The passage blobs tile the document: nothing is lost by the split.
	var joined strings.Builder
	for _, passage := range passages {
		key := tripleValue(t, passage, source.DocBodyKey)
		blob, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("passage %s body key %q not in the store: %v", passage.ID, key, err)
		}
		joined.Write(blob)
	}
	if joined.String() != body {
		t.Errorf("passage bodies concatenated = %q, want the document %q", joined.String(), body)
	}
}

func TestDocHandler_IngestEntityStates_IDHasSixParts(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nBody.")

	h := dochandler.New()
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

func TestDocHandler_IngestEntityStates_TriplesUseVocabularyPredicates(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Doc\nSome content.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	predicates := make(map[string]bool)
	for _, tr := range states[0].Triples {
		predicates[tr.Predicate] = true
	}

	wantPredicates := []string{
		source.DocType,
		source.DocFilePath,
		source.DocMimeType,
		source.DocFileHash,
		source.DocContent,
		source.DocSummary,
	}
	for _, p := range wantPredicates {
		if !predicates[p] {
			t.Errorf("missing triple with predicate %q", p)
		}
	}
}

func TestDocHandler_IngestEntityStates_TriplesAreSelfSubject(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Doc\nSome content.")

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

	h := dochandler.New()
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

func TestDocHandler_IngestEntityStates_StoreFailureFallsBackToInline(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Doc\n"+strings.Repeat("word ", 200))

	// A body store whose Put always fails: the entity degrades to inline content
	// with no body handle and no StorageRef — a best-effort facet, not a failed
	// ingest. The degradation is per-entity, so parent AND every passage must
	// fall back; a passage that kept a dangling handle would be unhydratable.
	h := dochandler.NewWithOrg("acme", dochandler.WithBodyStore(&failStore{}, "objectstore"))
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if parents := parentStates(states); len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (doc.md); %d states in total", len(parents), len(states))
	}
	if passages := passageStates(states); len(passages) == 0 {
		t.Fatalf("passage state count: got 0, want at least 1 for doc.md; %d states in total", len(states))
	}
	for _, state := range states {
		assertInlineBody(t, state)
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

func TestDocHandler_IngestEntityStates_NoStoreAllInline(t *testing.T) {
	dir := t.TempDir()
	largeContent := "# Large Doc\n" + strings.Repeat("word ", 200)
	writeMD(t, dir, "large.md", largeContent)

	// No store configured — all content stays inline regardless of size, for the
	// parent and for every passage.
	h := dochandler.NewWithOrg("acme")
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}

	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (large.md); %d states in total", len(parents), len(states))
	}
	passages := passageStates(states)
	if len(passages) == 0 {
		t.Fatalf("passage state count: got 0, want at least 1 for large.md; %d states in total", len(states))
	}
	for _, state := range states {
		assertInlineBody(t, state)
	}

	// Inline means the whole body, not a truncated one: the parent carries the
	// document, and the passages tile it.
	if got := tripleValue(t, parents[0], source.DocContent); got != largeContent {
		t.Errorf("parent %s length = %d, want %d (the full document, inline)",
			source.DocContent, len(got), len(largeContent))
	}
	var joined strings.Builder
	for _, passage := range passages {
		joined.WriteString(tripleValue(t, passage, source.DocContent))
	}
	if joined.String() != largeContent {
		t.Errorf("passage %s concatenated length = %d, want %d (passages tile the document)",
			source.DocContent, joined.Len(), len(largeContent))
	}
}
