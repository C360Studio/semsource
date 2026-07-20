package supersession

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// Passage-liveness coverage for decideLifecycleActions. A document that SHRANK
// is structurally invisible to the pass's filesystem check: every passage of a
// ten-passage document that is now seven passages long still carries the path
// of a file that is very much still on disk, so stat() reports "present" for
// all ten. The parent's DocChunkCount is the only evidence, which makes
// liveness for a passage `index < count` rather than stat().

// --- fixtures --------------------------------------------------------------

// asInt encodes a numeric triple object the way the producer emits it: a Go
// int. Passed as docFixture.num for the hand-built (non-round-tripped) shape.
func asInt(n int) any { return n }

// asFloat64 encodes a numeric triple object the way it comes BACK from
// QueryPrefixAll: JSON has no integers, so every count and ordinal arrives as
// a float64. This is the production shape, not an exotic one.
func asFloat64(n int) any { return float64(n) }

// docFixture describes one document's worth of enumerated entities: a parent
// carrying DocFilePath + DocChunkCount, plus `passages` passage entities
// carrying DocFilePath + DocChunkIndex at ordinals 0..passages-1.
type docFixture struct {
	path string // shared DocFilePath — the grouping key

	count   int  // the parent's DocChunkCount (live passage count)
	noCount bool // omit the DocChunkCount predicate entirely (pre-chunking doc)

	passages   int   // how many passage entities to emit
	markedIdx  []int // passage ordinals that already carry the stale marker
	parentMark bool  // whether the parent already carries the stale marker

	// num encodes every emitted numeric object. nil defaults to asInt.
	num func(int) any

	// omitParent drops the parent entity entirely, modelling an enumeration
	// truncated before it reached the parent.
	omitParent bool
}

// parentID / passageID name the entities a docFixture emits. Kept as functions
// so assertions can name the exact entity they expect without duplicating the
// ID grammar.
func (f docFixture) parentID() string {
	return "acme.semsource.web.docs.doc." + slugOf(f.path)
}

func (f docFixture) passageID(idx int) string {
	return fmt.Sprintf("acme.semsource.web.docs.passage.%s-p%d", slugOf(f.path), idx)
}

// slugOf turns a fixture path into an ID-safe instance segment.
func slugOf(path string) string {
	out := make([]rune, 0, len(path))
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

// build materialises the fixture as the []gtypes.EntityState shape
// decideLifecycleActions receives from QueryPrefixAll.
func (f docFixture) build() []gtypes.EntityState {
	num := f.num
	if num == nil {
		num = asInt
	}
	marked := make(map[int]bool, len(f.markedIdx))
	for _, i := range f.markedIdx {
		marked[i] = true
	}

	var out []gtypes.EntityState
	if !f.omitParent {
		id := f.parentID()
		tr := []message.Triple{
			{Subject: id, Predicate: source.DocFilePath, Object: f.path},
			{Subject: id, Predicate: source.DocType, Object: "document"},
		}
		if !f.noCount {
			tr = append(tr, message.Triple{Subject: id, Predicate: source.DocChunkCount, Object: num(f.count)})
		}
		if f.parentMark {
			tr = append(tr, staleMarkerTriple(id))
		}
		out = append(out, gtypes.EntityState{ID: id, Triples: tr})
	}

	for i := 0; i < f.passages; i++ {
		id := f.passageID(i)
		tr := []message.Triple{
			{Subject: id, Predicate: source.DocFilePath, Object: f.path},
			{Subject: id, Predicate: source.DocType, Object: "passage"},
			{Subject: id, Predicate: source.DocChunkIndex, Object: num(i)},
		}
		if marked[i] {
			tr = append(tr, staleMarkerTriple(id))
		}
		out = append(out, gtypes.EntityState{ID: id, Triples: tr})
	}
	return out
}

// staleMarkerTriple builds the already-present marker a prior pass would have
// written. The reason recorded here is deliberately passage_removed so a
// clear-then-remark bug cannot hide behind a reason mismatch.
func staleMarkerTriple(id string) message.Triple {
	return message.Triple{
		Subject:   id,
		Predicate: source.EntityLifecycleStale,
		Object:    graph.LifecycleReasonPassageRemoved,
	}
}

// roundTripJSON marshals entities and reads them back, reproducing exactly what
// QueryPrefixAll hands the pass: every numeric triple object that the producer
// emitted as a Go int is a float64 on the far side.
func roundTripJSON(t *testing.T, entities []gtypes.EntityState) []gtypes.EntityState {
	t.Helper()
	data, err := json.Marshal(entities)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var out []gtypes.EntityState
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return out
}

// statPresent / statMissing are the two injected filesystem verdicts.
func statPresent(_ string) bool { return true }
func statMissing(_ string) bool { return false }

// --- assertion helpers -----------------------------------------------------

// markedByID indexes emitted marker triples by subject → reason. It fails on a
// duplicate subject: the mutation lane APPENDS rather than replacing by
// predicate, so a doubled marker is a real defect, not a harmless repeat.
func markedByID(t *testing.T, toMark []message.Triple) map[string]string {
	t.Helper()
	out := make(map[string]string, len(toMark))
	for _, tr := range toMark {
		if tr.Predicate != source.EntityLifecycleStale {
			t.Errorf("emitted marker has predicate %q, want %q", tr.Predicate, source.EntityLifecycleStale)
		}
		if _, dup := out[tr.Subject]; dup {
			t.Errorf("entity %s marked twice in one pass; the append-only lane would duplicate the triple", tr.Subject)
		}
		reason, _ := tr.Object.(string)
		out[tr.Subject] = reason
	}
	return out
}

// assertMarkedExactly checks that precisely wantIDs were marked, each with
// wantReason, naming the difference on failure.
func assertMarkedExactly(t *testing.T, toMark []message.Triple, wantReason string, wantIDs ...string) {
	t.Helper()
	got := markedByID(t, toMark)
	if len(got) != len(wantIDs) {
		t.Errorf("marked %d entities (%v), want %d (%v)", len(got), sortedKeys(got), len(wantIDs), sortedCopy(wantIDs))
	}
	for _, id := range wantIDs {
		reason, ok := got[id]
		if !ok {
			t.Errorf("entity %s was NOT marked; marked set was %v", id, sortedKeys(got))
			continue
		}
		if reason != wantReason {
			t.Errorf("entity %s marked with reason %q, want %q", id, reason, wantReason)
		}
		delete(got, id)
	}
	for id := range got {
		t.Errorf("entity %s was marked but should have been left alone", id)
	}
}

// assertNoActions checks the fully-converged case: nothing to write at all.
func assertNoActions(t *testing.T, toMark []message.Triple, toClear []string, context string) {
	t.Helper()
	if len(toMark) != 0 {
		t.Errorf("%s: marked %v, want no marks", context, sortedKeys(markedByID(t, toMark)))
	}
	if len(toClear) != 0 {
		t.Errorf("%s: cleared %v, want no clears", context, sortedCopy(toClear))
	}
}

// assertClearedExactly checks that precisely wantIDs had their marker cleared.
func assertClearedExactly(t *testing.T, toClear []string, wantIDs ...string) {
	t.Helper()
	got, want := sortedCopy(toClear), sortedCopy(wantIDs)
	if len(got) != len(want) {
		t.Fatalf("cleared %d entities %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cleared[%d] = %s, want %s (full set: got %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// --- 1. shrink marks the tail ---------------------------------------------

func TestDecideLifecycleActions_ShrunkDocument_MarksOnlyTheVanishedTail(t *testing.T) {
	// A ten-passage document is now seven passages long. The file is still on
	// disk, so stat() cannot see the loss — only the parent's count can.
	fx := docFixture{path: "guide.md", count: 7, passages: 10}
	toMark, toClear, paths := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)

	if paths != 1 {
		t.Errorf("Paths = %d, want 1 (parent and all passages share one path)", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("cleared %v, want no clears (nothing regrew)", sortedCopy(toClear))
	}
	assertMarkedExactly(t, toMark, graph.LifecycleReasonPassageRemoved,
		fx.passageID(7), fx.passageID(8), fx.passageID(9))

	// Spelled out separately from the "exactly" assertion so a regression that
	// over-marks names the specific survivor it should not have touched.
	marked := markedByID(t, toMark)
	for i := 0; i < 7; i++ {
		if _, ok := marked[fx.passageID(i)]; ok {
			t.Errorf("passage %d is live (index < count 7) but was marked stale", i)
		}
	}
	if _, ok := marked[fx.parentID()]; ok {
		t.Error("parent document was marked stale; the file it points at still exists")
	}
}

// --- 2. live passages are never marked ------------------------------------

func TestDecideLifecycleActions_AllPassagesLive_NoAction(t *testing.T) {
	fx := docFixture{path: "guide.md", count: 10, passages: 10}
	toMark, toClear, paths := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)

	if paths != 1 {
		t.Errorf("Paths = %d, want 1", paths)
	}
	assertNoActions(t, toMark, toClear, "unchanged ten-passage document")
}

// --- 3. regrowth clears ----------------------------------------------------

func TestDecideLifecycleActions_RegrownDocument_ClearsRevivedPassages(t *testing.T) {
	// The document shrank to seven, a prior pass marked 7/8/9, and the prose
	// has since come back: count is ten again, so the three markers are wrong.
	fx := docFixture{path: "guide.md", count: 10, passages: 10, markedIdx: []int{7, 8, 9}}
	toMark, toClear, _ := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)

	if len(toMark) != 0 {
		t.Errorf("marked %v, want no marks (every passage is live again)", sortedKeys(markedByID(t, toMark)))
	}
	assertClearedExactly(t, toClear, fx.passageID(7), fx.passageID(8), fx.passageID(9))
}

// --- 4. idempotent ---------------------------------------------------------

func TestDecideLifecycleActions_ShrunkDocumentAlreadyMarked_IsIdempotent(t *testing.T) {
	// Same shrink as test 1, one pass later. AddTriples APPENDS rather than
	// replacing by predicate, so re-emitting these markers would stack a
	// duplicate triple on every run.
	fx := docFixture{path: "guide.md", count: 7, passages: 10, markedIdx: []int{7, 8, 9}}
	toMark, toClear, _ := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)

	assertNoActions(t, toMark, toClear, "converged shrink (7/8/9 already marked)")
}

// --- 5. a deleted file still wins -----------------------------------------

func TestDecideLifecycleActions_DeletedFile_MarksParentAndEveryPassage(t *testing.T) {
	// The whole document is gone. Passage arithmetic is irrelevant: every
	// entity backed by the missing file is stale, and with the CALLER's
	// reason — passage_removed would misdescribe a deleted file.
	fx := docFixture{path: "guide.md", count: 7, passages: 10}
	toMark, toClear, paths := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statMissing)

	if paths != 1 {
		t.Errorf("Paths = %d, want 1", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("cleared %v, want no clears for a deleted file", sortedCopy(toClear))
	}

	want := []string{fx.parentID()}
	for i := 0; i < 10; i++ {
		want = append(want, fx.passageID(i))
	}
	assertMarkedExactly(t, toMark, graph.LifecycleReasonFileDeleted, want...)
}

// --- 6. no parent, no judgement -------------------------------------------

func TestDecideLifecycleActions_PassagesWithoutParent_NeverMarked(t *testing.T) {
	// Enumeration truncated before the parent, so DocChunkCount — the only
	// evidence a passage has vanished — is absent. Conservative by design:
	// never mark without evidence.
	fx := docFixture{path: "guide.md", passages: 10, omitParent: true}
	toMark, toClear, paths := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)

	if paths != 1 {
		t.Errorf("Paths = %d, want 1", paths)
	}
	assertNoActions(t, toMark, toClear, "orphaned passages with no parent count")

	// Same shape, but a prior pass had already marked the tail. Absence of
	// evidence must not read as evidence of liveness: without the parent's count
	// there is no basis to clear those markers either, and doing so would
	// resurrect genuinely vanished passages every time enumeration truncated
	// before the parent, then re-mark them on the next full pass. No evidence,
	// no action — in both directions.
	withMarks := docFixture{path: "guide.md", passages: 10, markedIdx: []int{7, 8, 9}, omitParent: true}
	toMark, toClear, _ = decideLifecycleActions(withMarks.build(), "/docs", graph.LifecycleReasonFileDeleted, statPresent)
	assertNoActions(t, toMark, toClear, "already-marked passages with no parent count")
}

// --- 7. pre-chunking documents are untouched -------------------------------

func TestDecideLifecycleActions_PreChunkingDocument_BehavesAsBefore(t *testing.T) {
	// A document ingested before passages existed carries no DocChunkCount and
	// has no passage entities. It must follow the original rule exactly:
	// present → clear if marked, missing → mark.
	tests := []struct {
		name        string
		marked      bool
		present     bool
		wantMarked  []string
		wantCleared []string
	}{
		{name: "present and unmarked is a no-op", marked: false, present: true},
		{name: "present and marked is cleared", marked: true, present: true,
			wantCleared: []string{"acme.semsource.web.docs.doc.legacy-md"}},
		{name: "missing and unmarked is marked", marked: false, present: false,
			wantMarked: []string{"acme.semsource.web.docs.doc.legacy-md"}},
		{name: "missing and marked is a no-op", marked: true, present: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := docFixture{path: "legacy.md", noCount: true, parentMark: tt.marked}
			stat := statMissing
			if tt.present {
				stat = statPresent
			}
			toMark, toClear, _ := decideLifecycleActions(fx.build(), "/docs", graph.LifecycleReasonPathMissing, stat)

			assertMarkedExactly(t, toMark, graph.LifecycleReasonPathMissing, tt.wantMarked...)
			assertClearedExactly(t, toClear, tt.wantCleared...)
		})
	}
}

// --- 8. code entities are unaffected --------------------------------------

func TestDecideLifecycleActions_CodeEntitiesUnaffectedByPassageRules(t *testing.T) {
	// Code entities carry CodePath and no chunk predicates. Mixed into the same
	// run as a shrinking document, they must behave exactly as they did before
	// passage liveness existed.
	shrunk := docFixture{path: "guide.md", count: 7, passages: 10}

	live := codeEntityAt("acme.semsource.golang.repo.function.a-go-Foo", "a.go", false)
	entities := append(shrunk.build(), live)

	toMark, toClear, paths := decideLifecycleActions(entities, "/repo", graph.LifecycleReasonFileDeleted, statPresent)
	if paths != 2 {
		t.Errorf("Paths = %d, want 2 (guide.md and a.go)", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("cleared %v, want no clears", sortedCopy(toClear))
	}
	// Only the doc's tail — the live code symbol is untouched even though it
	// shares the run with a group that has a chunk count.
	assertMarkedExactly(t, toMark, graph.LifecycleReasonPassageRemoved,
		shrunk.passageID(7), shrunk.passageID(8), shrunk.passageID(9))

	// And a code entity whose file is gone is still marked with the caller's
	// reason, unchanged by any of this.
	gone := []gtypes.EntityState{codeEntityAt("acme.semsource.golang.repo.function.b-go-Bar", "b.go", false)}
	toMark, _, _ = decideLifecycleActions(gone, "/repo", graph.LifecycleReasonFileDeleted, statMissing)
	assertMarkedExactly(t, toMark, graph.LifecycleReasonFileDeleted, "acme.semsource.golang.repo.function.b-go-Bar")
}

// --- 9. JSON round trip ----------------------------------------------------

// TestDecideLifecycleActions_NumericTriplesSurviveJSONRoundTrip is the load-
// bearing one. These entities reach the pass through QueryPrefixAll, i.e. a
// JSON decode into `any`, where the producer's Go int is a float64. If the
// numeric reader accepted only int, chunkCountOf would report "no count", every
// passage judgement would fail closed, and NO phantom would ever be marked in
// production — while every hand-built int fixture kept passing.
func TestDecideLifecycleActions_NumericTriplesSurviveJSONRoundTrip(t *testing.T) {
	shape := docFixture{path: "guide.md", count: 7, passages: 10}

	variants := []struct {
		name     string
		entities func() []gtypes.EntityState
	}{
		{
			name: "hand-built float64 objects",
			entities: func() []gtypes.EntityState {
				return docFixture{path: "guide.md", count: 7, passages: 10, num: asFloat64}.build()
			},
		},
		{
			name:     "marshalled to JSON and back",
			entities: func() []gtypes.EntityState { return roundTripJSON(t, shape.build()) },
		},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			entities := v.entities()

			// Guard the guard: assert the fixture really is in the float64
			// shape, so this test cannot quietly degrade into a duplicate of
			// test 1 if the encoding changes.
			assertNumericObjectsAreFloat64(t, entities)

			toMark, toClear, _ := decideLifecycleActions(entities, "/docs", graph.LifecycleReasonFileDeleted, statPresent)
			if len(toClear) != 0 {
				t.Errorf("cleared %v, want no clears", sortedCopy(toClear))
			}
			assertMarkedExactly(t, toMark, graph.LifecycleReasonPassageRemoved,
				shape.passageID(7), shape.passageID(8), shape.passageID(9))
		})
	}
}

// assertNumericObjectsAreFloat64 fails if any chunk count/index in entities is
// still a Go int — meaning the fixture never reproduced the query round trip.
func assertNumericObjectsAreFloat64(t *testing.T, entities []gtypes.EntityState) {
	t.Helper()
	seen := 0
	for i := range entities {
		for _, tr := range entities[i].Triples {
			if tr.Predicate != source.DocChunkCount && tr.Predicate != source.DocChunkIndex {
				continue
			}
			seen++
			if _, ok := tr.Object.(float64); !ok {
				t.Fatalf("fixture %s %s object is %T, want float64 — the fixture is not in the query round-trip shape",
					entities[i].ID, tr.Predicate, tr.Object)
			}
		}
	}
	if seen == 0 {
		t.Fatal("fixture carried no chunk count/index triples at all; the round-trip assertion would be vacuous")
	}
}

// TestTripleInt_AcceptsEveryNumericEncoding pins the decode surface directly,
// so a regression names the encoding it dropped rather than surfacing as a
// silent no-op three layers up.
func TestTripleInt_AcceptsEveryNumericEncoding(t *testing.T) {
	tests := []struct {
		name   string
		object any
		want   int
		wantOK bool
	}{
		{name: "producer-emitted Go int", object: 7, want: 7, wantOK: true},
		{name: "int64", object: int64(7), want: 7, wantOK: true},
		{name: "JSON round trip float64", object: float64(7), want: 7, wantOK: true},
		{name: "json.Number", object: json.Number("7"), want: 7, wantOK: true},
		{name: "zero is a real count, not a miss", object: float64(0), want: 0, wantOK: true},
		{name: "non-numeric object", object: "seven", want: 0, wantOK: false},
		{name: "nil object", object: nil, want: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triples := []message.Triple{{Predicate: source.DocChunkCount, Object: tt.object}}
			got, ok := tripleInt(triples, source.DocChunkCount)
			if ok != tt.wantOK {
				t.Fatalf("tripleInt(%T %v) ok = %v, want %v", tt.object, tt.object, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("tripleInt(%T %v) = %d, want %d", tt.object, tt.object, got, tt.want)
			}
		})
	}

	if _, ok := tripleInt(nil, source.DocChunkCount); ok {
		t.Error("tripleInt(nil triples) reported ok, want false")
	}
	absent := []message.Triple{{Predicate: source.DocFilePath, Object: "guide.md"}}
	if _, ok := tripleInt(absent, source.DocChunkCount); ok {
		t.Error("tripleInt reported ok for an absent predicate, want false")
	}
}

// --- 10. rootPath == "" (source removed) -----------------------------------

func TestDecideLifecycleActions_SourceRemoved_MarksPassagesUnconditionally(t *testing.T) {
	// remove_source: there is no filesystem left to check, so passage
	// arithmetic never runs. Every in-scope entity is stale — except the ones
	// already marked, which stay untouched so the append lane cannot double up.
	fx := docFixture{path: "guide.md", count: 10, passages: 10, markedIdx: []int{0, 1}}
	entities := append(fx.build(), codeEntityAt("acme.semsource.golang.repo.function.a-go-Foo", "a.go", false))

	// nil stat, exactly as runLifecyclePass passes it when RootPath is empty:
	// any filesystem check on this path would nil-panic rather than pass.
	toMark, toClear, paths := decideLifecycleActions(entities, "", graph.LifecycleReasonSourceRemoved, nil)

	if paths != 0 {
		t.Errorf("Paths = %d, want 0 (no filesystem check on remove_source)", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("cleared %v, want no clears (remove_source never revives an entity)", sortedCopy(toClear))
	}

	want := []string{fx.parentID(), "acme.semsource.golang.repo.function.a-go-Foo"}
	for i := 2; i < 10; i++ { // 0 and 1 were already marked
		want = append(want, fx.passageID(i))
	}
	assertMarkedExactly(t, toMark, graph.LifecycleReasonSourceRemoved, want...)
}

// Guard for an assumption the passage tests lean on throughout: the doc path
// predicate is what groups a parent with its passages. If a passage ever stops
// carrying DocFilePath it would be silently dropped from every group, and the
// shrink tests above would pass while marking nothing.
func TestPassageEntitiesGroupWithTheirParentByPath(t *testing.T) {
	fx := docFixture{path: "guide.md", count: 7, passages: 10}
	for _, e := range fx.build() {
		p, ok := pathOf(e.Triples)
		if !ok || p != "guide.md" {
			t.Errorf("entity %s pathOf = (%q,%v), want (guide.md,true)", e.ID, p, ok)
		}
	}
	// Reuse the semsourceast import in a way that documents the contrast: code
	// entities group by a different predicate entirely.
	code := codeEntityAt("acme.semsource.golang.repo.function.a-go-Foo", "a.go", false)
	if code.Triples[0].Predicate != semsourceast.CodePath {
		t.Errorf("code entity groups on %q, want %q", code.Triples[0].Predicate, semsourceast.CodePath)
	}
}
