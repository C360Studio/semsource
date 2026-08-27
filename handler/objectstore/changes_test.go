package objectstore_test

import (
	"testing"
	"time"

	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/storage/s3store"
)

// plan enumerates and plans in one step, asserting no body was read on the
// way. Change detection decides from listing metadata alone; a planning path
// that grew a fetch would fail every test that goes through here.
func plan(t *testing.T, store *fakeStore, prefix string, previous map[string]string) objectstore.Plan {
	t.Helper()

	pass, err := objectstore.Enumerate(t.Context(), store, prefix)
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	p := pass.Plan(previous)

	if reads := store.gets.Load(); reads != 0 {
		t.Errorf("planning read %d object bodies, want 0 — change detection runs on listing metadata", reads)
	}
	return p
}

func fetchedKeys(p objectstore.Plan) []string {
	out := make([]string, 0, len(p.Fetch))
	for _, info := range p.Fetch {
		out = append(out, info.Key)
	}
	return out
}

// ─── Change detection ───────────────────────────────────────────────────────

func TestPlan_NewObjectsAreFetched(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("reports/b.md", "etag-b", 20),
	}}

	got := plan(t, store, "reports/", nil)

	assertStrings(t, fetchedKeys(got), []string{"reports/a.md", "reports/b.md"})
	if len(got.Removed) != 0 {
		t.Errorf("Removed = %v, want none", got.Removed)
	}
}

// TestPlan_UnchangedObjectsAreNotFetched is the second-pass case: nothing
// moved, so nothing is read and nothing is republished.
func TestPlan_UnchangedObjectsAreNotFetched(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("reports/b.md", "etag-b", 20),
	}}

	tracker := objectstore.NewTracker()
	for _, info := range store.objects {
		tracker.Record(info.Key, objectstore.ChangeToken(info))
	}

	got := plan(t, store, "reports/", tracker.Observed())

	if len(got.Fetch) != 0 {
		t.Errorf("Fetch = %v, want nothing — no object changed", fetchedKeys(got))
	}
	if len(got.Removed) != 0 {
		t.Errorf("Removed = %v, want none", got.Removed)
	}
}

func TestPlan_ChangedObjectIsRefetched(t *testing.T) {
	tracker := objectstore.NewTracker()
	tracker.Record("reports/a.md", "etag-a")
	tracker.Record("reports/b.md", "etag-b")

	// a.md was replaced at the same key; b.md was not touched.
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a-v2", 11),
		obj("reports/b.md", "etag-b", 20),
	}}

	got := plan(t, store, "reports/", tracker.Observed())

	assertStrings(t, fetchedKeys(got), []string{"reports/a.md"})
	if len(got.Removed) != 0 {
		t.Errorf("a replaced object is not a removed one: Removed = %v", got.Removed)
	}
}

// TestChangeToken_PrefersETagAndFallsBack covers the store that sends no ETag.
// Size and last-modified together are a weaker signal — an in-place edit
// preserving both goes unnoticed — but they are what the spec names, and they
// beat re-fetching everything every pass.
func TestChangeToken_PrefersETagAndFallsBack(t *testing.T) {
	withETag := obj("reports/a.md", "etag-a", 10)
	if got := objectstore.ChangeToken(withETag); got != "etag-a" {
		t.Errorf("ChangeToken = %q, want the ETag", got)
	}

	noETag := s3store.ObjectInfo{Key: "reports/a.md", Size: 10, LastModified: modTime}
	base := objectstore.ChangeToken(noETag)
	if base == "" {
		t.Fatal("an object with no ETag must still get a token")
	}

	sameAgain := objectstore.ChangeToken(noETag)
	if sameAgain != base {
		t.Errorf("token is unstable across calls: %q then %q", base, sameAgain)
	}

	resized := noETag
	resized.Size = 11
	if objectstore.ChangeToken(resized) == base {
		t.Error("a size change must move the token")
	}

	touched := noETag
	touched.LastModified = modTime.Add(time.Second)
	if objectstore.ChangeToken(touched) == base {
		t.Error("a last-modified change must move the token")
	}
}

func TestPlan_ObjectWithoutETagUsesTheFallback(t *testing.T) {
	unchanged := s3store.ObjectInfo{Key: "reports/a.md", Size: 10, LastModified: modTime}
	tracker := objectstore.NewTracker()
	tracker.Record(unchanged.Key, objectstore.ChangeToken(unchanged))

	store := &fakeStore{objects: []s3store.ObjectInfo{unchanged}}
	if got := plan(t, store, "reports/", tracker.Observed()); len(got.Fetch) != 0 {
		t.Errorf("Fetch = %v, want nothing — neither size nor last-modified moved", fetchedKeys(got))
	}

	grown := unchanged
	grown.Size = 12
	store = &fakeStore{objects: []s3store.ObjectInfo{grown}}
	if got := plan(t, store, "reports/", tracker.Observed()); len(got.Fetch) != 1 {
		t.Errorf("Fetch = %v, want the resized object", fetchedKeys(got))
	}
}

// ─── The skip gate ──────────────────────────────────────────────────────────

// TestPlan_SkipsUnsupportedFormat pins the divergence from the filesystem
// walk, which skips silently. A .go file in a repository is not a failure; a
// .pdf in a bucket of reports is something an operator needs to see.
func TestPlan_SkipsUnsupportedFormat(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("reports/scan.pdf", "etag-p", 4096),
		obj("reports/data.csv", "etag-c", 512),
		obj("reports/no-extension", "etag-n", 128),
	}}

	got := plan(t, store, "reports/", nil)

	assertStrings(t, fetchedKeys(got), []string{"reports/a.md"})

	want := map[string]objectstore.SkipReason{
		"reports/data.csv":     objectstore.SkipUnsupportedFormat,
		"reports/no-extension": objectstore.SkipUnsupportedFormat,
		"reports/scan.pdf":     objectstore.SkipUnsupportedFormat,
	}
	assertSkips(t, got.Skipped, want)
}

func TestPlan_SkipsZeroByteObject(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("reports/empty.md", "etag-e", 0),
	}}

	got := plan(t, store, "reports/", nil)

	assertStrings(t, fetchedKeys(got), []string{"reports/a.md"})
	assertSkips(t, got.Skipped, map[string]objectstore.SkipReason{
		"reports/empty.md": objectstore.SkipEmptyObject,
	})
}

// TestPlan_ZeroByteUnsupportedReportsTheFormat picks the actionable reason: the
// extension is why the object will never be ingested, whatever its size
// becomes.
func TestPlan_ZeroByteUnsupportedReportsTheFormat(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{obj("reports/empty.png", "etag-e", 0)}}

	assertSkips(t, plan(t, store, "reports/", nil).Skipped, map[string]objectstore.SkipReason{
		"reports/empty.png": objectstore.SkipUnsupportedFormat,
	})
}

// TestPlan_SkippedObjectsAreNotRemovals keeps the two counts apart. A skipped
// object was never ingested, so there is nothing about it to retract, and
// reporting it as removed would make an operator hunt for a deletion that
// never happened.
func TestPlan_SkippedObjectsAreNotRemovals(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{obj("reports/scan.pdf", "etag-p", 4096)}}

	got := plan(t, store, "reports/", nil)

	if len(got.Removed) != 0 {
		t.Errorf("Removed = %v, want none", got.Removed)
	}
	if len(got.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want one entry", got.Skipped)
	}
}

// ─── Removal ────────────────────────────────────────────────────────────────

func TestPlan_AbsentKeysAreRemoved(t *testing.T) {
	tracker := objectstore.NewTracker()
	tracker.Record("reports/a.md", "etag-a")
	tracker.Record("reports/gone.md", "etag-g")

	store := &fakeStore{objects: []s3store.ObjectInfo{obj("reports/a.md", "etag-a", 10)}}

	got := plan(t, store, "reports/", tracker.Observed())

	assertStrings(t, got.Removed, []string{"reports/gone.md"})
	if len(got.Fetch) != 0 {
		t.Errorf("Fetch = %v, want nothing", fetchedKeys(got))
	}
}

// TestPlan_LegitimatelyEmptiedPrefixRemovesEverything is the case that must
// NOT be defended against. A completed listing that found nothing is a real
// answer, and refusing to act on it would leave a deleted corpus in the graph
// forever.
func TestPlan_LegitimatelyEmptiedPrefixRemovesEverything(t *testing.T) {
	tracker := objectstore.NewTracker()
	tracker.Record("reports/a.md", "etag-a")
	tracker.Record("reports/b.md", "etag-b")

	got := plan(t, &fakeStore{}, "reports/", tracker.Observed())

	assertStrings(t, got.Removed, []string{"reports/a.md", "reports/b.md"})
}

// TestPlan_FailedListingRemovesNothing is the same shape from the other side,
// and the reason Enumerate hands back nil rather than a partial pass. A caller
// that skipped its error check retracts nothing instead of the entire corpus.
func TestPlan_FailedListingRemovesNothing(t *testing.T) {
	tracker := objectstore.NewTracker()
	tracker.Record("reports/a.md", "etag-a")
	tracker.Record("reports/b.md", "etag-b")

	var failed *objectstore.Pass // what Enumerate returns when the listing dies

	got := failed.Plan(tracker.Observed())

	if len(got.Removed) != 0 {
		t.Errorf("a listing that did not finish concluded %v were deleted", got.Removed)
	}
	if len(got.Fetch) != 0 || len(got.Skipped) != 0 {
		t.Errorf("a failed pass implies no work at all, got %+v", got)
	}
	if failed.Len() != 0 || failed.Objects() != nil || failed.Prefix() != "" {
		t.Error("a failed pass must answer every question with nothing")
	}
}

// ─── Tracker ────────────────────────────────────────────────────────────────

func TestTracker_RecordForgetAndCopy(t *testing.T) {
	tracker := objectstore.NewTracker()
	tracker.Record("reports/a.md", "etag-a")
	tracker.Record("reports/b.md", "etag-b")

	if tracker.Len() != 2 {
		t.Errorf("Len = %d, want 2", tracker.Len())
	}

	observed := tracker.Observed()
	observed["reports/c.md"] = "etag-c"
	delete(observed, "reports/a.md")
	if tracker.Len() != 2 {
		t.Error("Observed returned the live map — mutating the copy changed the tracker")
	}

	tracker.Forget("reports/a.md")
	if _, present := tracker.Observed()["reports/a.md"]; present {
		t.Error("a forgotten key is still tracked")
	}

	// A key that reappears after removal ingests as new.
	store := &fakeStore{objects: []s3store.ObjectInfo{obj("reports/a.md", "etag-a", 10)}}
	if got := plan(t, store, "reports/", tracker.Observed()); len(got.Fetch) != 1 {
		t.Errorf("a reappearing key must ingest as new, Fetch = %v", fetchedKeys(got))
	}
}

func assertSkips(t *testing.T, got []objectstore.Skip, want map[string]objectstore.SkipReason) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("Skipped = %v, want %d entries", got, len(want))
	}
	for _, skip := range got {
		reason, expected := want[skip.Key]
		if !expected {
			t.Errorf("unexpected skip for %q", skip.Key)
			continue
		}
		if skip.Reason != reason {
			t.Errorf("%q skipped as %q, want %q", skip.Key, skip.Reason, reason)
		}
		if skip.Reason == "" {
			t.Errorf("%q was skipped with no reason", skip.Key)
		}
	}
}
