package docsource

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
)

// notePassageCount coverage. A document that SHRANK is invisible to the
// lifecycle pass's filesystem check — the passages it dropped still point at a
// file that is still on disk — so doc-source notices the drop itself and
// announces scope. These tests exercise the DECISION, not the NATS round trip:
// the component runs with a nil NATS client, which PublishLifecycleTrigger
// rejects immediately without touching the network.
//
// The decision is observed through the component's logger, because that is the
// only seam the current code exposes: notePassageCount returns nothing, and the
// trigger it fires is a bare goroutine over a concrete *natsclient.Client with
// no injection point. Two records matter, and both are matched on ATTRIBUTES
// rather than message text so a reworded log does not fail the suite:
//
//   - the synchronous decision record, identified by its "was" attribute. It is
//     emitted on the line immediately before the trigger goroutine is spawned,
//     so its presence or absence after notePassageCount returns is a
//     deterministic, sleep-free answer to "did this announce scope?".
//   - the trigger record, identified by a "reason" attribute. It comes from the
//     spawned goroutine and carries the reason that actually reached
//     PublishLifecycleTrigger.

// --- log recorder ----------------------------------------------------------

// loggedRecord is one captured slog record, flattened to the attributes the
// assertions key on.
type loggedRecord struct {
	msg   string
	attrs map[string]any
}

// logRecorder is a slog.Handler that captures every record. Mutex-guarded
// because the lifecycle trigger logs from a background goroutine.
type logRecorder struct {
	mu      sync.Mutex
	records []loggedRecord
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	entry := loggedRecord{msg: rec.Message, attrs: make(map[string]any, rec.NumAttrs())}
	rec.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.Any()
		return true
	})
	r.mu.Lock()
	r.records = append(r.records, entry)
	r.mu.Unlock()
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

// find returns the first captured record matching pred.
func (r *logRecorder) find(pred func(loggedRecord) bool) (loggedRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.records {
		if pred(rec) {
			return rec, true
		}
	}
	return loggedRecord{}, false
}

// waitFor polls for a record matching pred, for records written from the
// trigger goroutine. Bounded and only ever used for a POSITIVE assertion: a
// negative is answered from the synchronous decision record instead, which
// needs no waiting at all.
func (r *logRecorder) waitFor(t *testing.T, pred func(loggedRecord) bool, what string) loggedRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if rec, ok := r.find(pred); ok {
			return rec
		}
		if time.Now().After(deadline) {
			r.mu.Lock()
			seen := append([]loggedRecord(nil), r.records...)
			r.mu.Unlock()
			t.Fatalf("timed out waiting for %s; captured records were %+v", what, seen)
		}
		time.Sleep(time.Millisecond)
	}
}

// isDecisionRecord matches the synchronous "document shrank" record, keyed on
// its "was" attribute rather than its wording.
func isDecisionRecord(rec loggedRecord) bool {
	_, ok := rec.attrs["was"]
	return ok
}

// attrInt reads a numeric attribute. slog normalises every integer kind to
// int64 on the way through, so the raw value never compares equal to an int
// literal.
func attrInt(rec loggedRecord, key string) (int64, bool) {
	v, ok := rec.attrs[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

// triggerReason returns the reason a lifecycle trigger carried, if one fired.
func triggerReason(rec loggedRecord) (string, bool) {
	v, ok := rec.attrs["reason"]
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, true
}

func isTriggerRecord(rec loggedRecord) bool {
	_, ok := triggerReason(rec)
	return ok
}

// --- fixtures --------------------------------------------------------------

// docRoot is the configured watch root every fixture path lives under. Made
// absolute because rootForPath compares against filepath.Abs of the config.
func docRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp root: %v", err)
	}
	return root
}

// passageComponent builds a doc-source Component wired for passage-count
// tracking: a recording logger, a nil NATS client (so the trigger fails fast
// without a network), and one configured watch root.
func passageComponent(t *testing.T, watchEnabled bool, root string) (*Component, *logRecorder) {
	t.Helper()
	rec := &logRecorder{}
	c := &Component{
		name:          "doc-source",
		config:        Config{Org: "acme", Paths: []string{root}, WatchEnabled: watchEnabled},
		logger:        slog.New(rec),
		passageCounts: make(map[string]int),
	}
	return c, rec
}

// docStates builds what the doc handler hands a change event for a document of
// `passages` passages: one parent document state plus one state per passage,
// each classified by its DocType fact exactly as the producer emits it.
func docStates(path string, passages int) []*handler.EntityState {
	parent := &handler.EntityState{
		ID: "acme.semsource.web.docs.doc.guide-md",
		Triples: []message.Triple{
			{Predicate: source.DocFilePath, Object: path},
			{Predicate: source.DocType, Object: "document"},
		},
	}
	states := []*handler.EntityState{parent}
	for i := 0; i < passages; i++ {
		states = append(states, &handler.EntityState{
			ID: fmt.Sprintf("acme.semsource.web.docs.passage.guide-md-p%d", i),
			Triples: []message.Triple{
				{Predicate: source.DocFilePath, Object: path},
				{Predicate: source.DocType, Object: "passage"},
				{Predicate: source.DocChunkIndex, Object: i},
			},
		})
	}
	return states
}

// assertNoTrigger checks the deterministic negative: notePassageCount has
// returned, and the decision record it would have written before spawning the
// trigger goroutine is absent, so no goroutine was spawned.
func assertNoTrigger(t *testing.T, rec *logRecorder, context string) {
	t.Helper()
	if got, ok := rec.find(isDecisionRecord); ok {
		t.Errorf("%s: announced scope to the lifecycle pass (record %+v), want no trigger", context, got)
	}
	if got, ok := rec.find(isTriggerRecord); ok {
		reason, _ := triggerReason(got)
		t.Errorf("%s: fired a lifecycle trigger with reason %q, want no trigger", context, reason)
	}
}

// --- 11. a drop triggers a passage_removed run -----------------------------

func TestNotePassageCount_DropTriggersPassageRemovedRun(t *testing.T) {
	root := docRoot(t)
	path := filepath.Join(root, "guide.md")
	c, rec := passageComponent(t, true, root)
	ctx := context.Background()

	c.notePassageCount(ctx, path, docStates(path, 9))
	assertNoTrigger(t, rec, "first sighting of guide.md")

	c.notePassageCount(ctx, path, docStates(path, 6))

	decision, ok := rec.find(isDecisionRecord)
	if !ok {
		t.Fatal("a document that shrank from 9 passages to 6 did not announce scope to the lifecycle pass")
	}
	if got, ok := attrInt(decision, "was"); !ok || got != 9 {
		t.Errorf("decision record was = %v (present=%v), want 9", got, ok)
	}
	if got, ok := attrInt(decision, "now"); !ok || got != 6 {
		t.Errorf("decision record now = %v (present=%v), want 6", got, ok)
	}
	if got := decision.attrs["path"]; got != path {
		t.Errorf("decision record path = %v, want %q", got, path)
	}

	trigger := rec.waitFor(t, isTriggerRecord, "the lifecycle trigger fired by the shrink")
	reason, _ := triggerReason(trigger)
	if reason != graph.LifecycleReasonPassageRemoved {
		t.Errorf("lifecycle run triggered with reason %q, want %q — only passage_removed describes a document that shrank while its file stayed on disk",
			reason, graph.LifecycleReasonPassageRemoved)
	}
	if got := trigger.attrs["root"]; got != root {
		t.Errorf("lifecycle run scoped to root %v, want %q", got, root)
	}

	// The new count replaces the old one, so the next unchanged publish is not
	// read as a second drop.
	if got := c.passageCounts[path]; got != 6 {
		t.Errorf("tracked count = %d, want 6 (the count just published)", got)
	}
}

// --- 12. same or higher never triggers -------------------------------------

func TestNotePassageCount_SameOrHigherNeverTriggers(t *testing.T) {
	tests := []struct {
		name  string
		first int
		then  int
	}{
		{name: "unchanged document", first: 7, then: 7},
		{name: "document grew by one", first: 7, then: 8},
		{name: "document grew a lot", first: 1, then: 40},
		{name: "empty stays empty", first: 0, then: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := docRoot(t)
			path := filepath.Join(root, "guide.md")
			c, rec := passageComponent(t, true, root)
			ctx := context.Background()

			c.notePassageCount(ctx, path, docStates(path, tt.first))
			c.notePassageCount(ctx, path, docStates(path, tt.then))

			assertNoTrigger(t, rec, tt.name)
			if got := c.passageCounts[path]; got != tt.then {
				t.Errorf("tracked count = %d, want %d", got, tt.then)
			}
		})
	}
}

// --- 13. first sighting has nothing to compare against ---------------------

func TestNotePassageCount_FirstSightingNeverTriggers(t *testing.T) {
	root := docRoot(t)
	path := filepath.Join(root, "guide.md")
	c, rec := passageComponent(t, true, root)

	// Zero passages is the sharpest first sighting: an unseen path reads back
	// previous == 0, so a drop test written as `passages < previous` is
	// indistinguishable from `0 < 0` and only the `seen` guard separates "no
	// passages yet" from "the passages went away". Today the `>=` comparison
	// already covers this on its own and `seen` is belt-and-braces; this test
	// pins the OUTCOME, so it holds whichever of the two is doing the work.
	c.notePassageCount(context.Background(), path, docStates(path, 0))

	assertNoTrigger(t, rec, "first sighting with zero passages")
	if _, seen := c.passageCounts[path]; !seen {
		t.Error("first sighting did not record a count, so the next publish could not detect a drop")
	}
}

// --- 14. a frozen source is exempt -----------------------------------------

func TestNotePassageCount_FrozenSourceNeverTriggers(t *testing.T) {
	// watch:false is a frozen source, exempt from staleness by design (D5).
	// Mirrors TestDeleteTriggersLifecycleRun for the shrink path: a frozen
	// source's vanished passages must never be marked.
	root := docRoot(t)
	path := filepath.Join(root, "guide.md")
	c, rec := passageComponent(t, false, root)
	ctx := context.Background()

	c.notePassageCount(ctx, path, docStates(path, 9))
	c.notePassageCount(ctx, path, docStates(path, 2))

	assertNoTrigger(t, rec, "frozen (watch:false) source whose document shrank 9 → 2")

	// The count is still tracked — freezing suppresses the announcement, not
	// the bookkeeping — so unfreezing does not need a restart to work.
	if got := c.passageCounts[path]; got != 2 {
		t.Errorf("tracked count = %d, want 2 (a frozen source still tracks, it just never announces)", got)
	}
}

// A path outside every configured root has no root to scope a run to, so it
// cannot trigger even on a genuine drop.
func TestNotePassageCount_PathOutsideConfiguredRootsNeverTriggers(t *testing.T) {
	root := docRoot(t)
	outside := filepath.Join(docRoot(t), "elsewhere.md")
	c, rec := passageComponent(t, true, root)
	ctx := context.Background()

	c.notePassageCount(ctx, outside, docStates(outside, 9))
	c.notePassageCount(ctx, outside, docStates(outside, 2))

	if got, ok := rec.find(isTriggerRecord); ok {
		reason, _ := triggerReason(got)
		t.Errorf("fired a lifecycle trigger (reason %q) for a path under no configured root, which has no scope to announce", reason)
	}
}

// --- 15. a bare struct literal survives a change event ---------------------

func TestHandleChangeEventOnBareComponentSurvivesNilPassageMap(t *testing.T) {
	// A Component assembled directly rather than through NewComponent has a nil
	// passageCounts map. Writing to it would panic, taking down the watch
	// goroutine on the first document change.
	root := docRoot(t)
	path := filepath.Join(root, "guide.md")
	rec := &logRecorder{}
	c := &Component{
		name:   "doc-source",
		config: Config{Org: "acme", Paths: []string{root}, WatchEnabled: true},
		logger: slog.New(rec),
		// passageCounts deliberately nil; natsClient and publisher deliberately nil.
	}
	if c.passageCounts != nil {
		t.Fatal("fixture is not the bare shape: passageCounts should start nil")
	}

	// Entity IDs that fail publish-boundary validation, so the change loop
	// never reaches the nil publisher and the test isolates the map write.
	states := docStates(path, 9)
	for _, st := range states {
		st.ID = "unpublishable"
	}

	ctx := context.Background()
	c.handleChangeEvent(ctx, handler.ChangeEvent{
		Path:         path,
		Operation:    handler.OperationModify,
		EntityStates: states,
	})

	if c.passageCounts == nil {
		t.Fatal("passageCounts is still nil after a change event; the next write would panic")
	}
	if got := c.passageCounts[path]; got != 9 {
		t.Errorf("tracked count = %d, want 9 (the nil map was replaced but not populated)", got)
	}

	// And the now-initialised map is usable: a following shrink is detected.
	shrunk := docStates(path, 4)
	for _, st := range shrunk {
		st.ID = "unpublishable"
	}
	c.handleChangeEvent(ctx, handler.ChangeEvent{
		Path:         path,
		Operation:    handler.OperationModify,
		EntityStates: shrunk,
	})

	if _, ok := rec.find(isDecisionRecord); !ok {
		t.Error("a bare-literal component did not detect a shrink after initialising its own map")
	}
}
