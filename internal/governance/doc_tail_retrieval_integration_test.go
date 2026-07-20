//go:build integration

package governance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semstreams/storage/storeregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
)

// truncationLimit is graph-embedding's hard-coded, unconfigurable text cap
// (`maxTextLen` in processor/graph-embedding/component.go, passed to
// WithMaxSourceTextLen; docs/upstream/semstreams-asks.md #22). Before passage chunking,
// doc ingestion emitted ONE entity per file carrying the whole body, and the substrate
// embeds exactly one vector per entity from text truncated at this many characters at a
// word boundary — with no error, metric, or log at producer level. Everything past the
// cut was silently unindexed; measured on this repository pre-chunking, ~252 KB of prose
// across 47 Markdown files, roughly three quarters of the project README included.
// Passage chunking fixes this by emitting one entity per document passage, each sized
// well under the cap and independently embedded, so no passage's tail is ever the part
// that gets cut.
const truncationLimit = 8000

// tailMarker is a phrase that cannot occur incidentally — it exists only to prove a
// specific byte range of the fixture document reached the index.
const tailMarker = "The nonsense marker phrase zzqfx9182-tail-only-marker appears exactly once, deep in the tail passage."

// TestIntegration_DocTailPhraseSurvivesPassageChunking pins the exact defect passage
// chunking was built to fix (task 9.3 of openspec/changes/doc-passage-chunking): ingest a
// document comfortably larger than graph-embedding's 8000-character truncation cap, place
// a distinctive phrase well past that cut, and prove it is retrievable — where the
// pre-chunking, one-entity-per-file design would have silently dropped it.
//
// Before this change, doc ingestion emitted a single entity carrying the entire file
// body. graph-embedding truncates the text it embeds at 8000 characters (unconfigurable,
// docs/upstream/semstreams-asks.md #22) with no error or metric, so any phrase living past
// that cut was semantically unreachable: it existed on disk and in the raw entity body,
// but no query could ever surface it. Passage chunking instead emits one entity per
// document passage, each independently offloaded and embedded, so a phrase in the
// document's tail lands in its OWN small entity rather than the truncated remainder of one
// giant one.
//
// This test does NOT assert ranking. A separate, known defect exists where retrieval
// prefers an "override" section's passage over a "default" one for unrelated reasons
// (tasks.md 9.5); asserting rank here would make this test fail for a cause it is not
// testing. This test asserts presence and retrievability only: the tail passage exists,
// carries a resolvable body, and the fusion engine returns that body verbatim.
func TestIntegration_DocTailPhraseSurvivesPassageChunking(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		// Bind ONLY the entity ingest subject, never a wildcard — a wildcard also
		// matches graph-query's request/reply forwards and races a PubAck onto the
		// reply inbox (docs/upstream/semstreams-asks.md #6).
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(ctx, tc.Client, nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = query.Stop(5 * time.Second) })

	tailPassage, tailBody, states, storeReg := ingestTailFixture(t, ctx, tc)

	// Publish parent and passages (triples + StorageRefs) into the live graph.
	for _, st := range states {
		publishSemsourceEntity(t, ctx, tc.Client, st.ID,
			semsourcegraph.IndexingProfileContent, st.Triples, st.StorageRef)
	}
	for _, st := range states {
		waitForEntityState(t, ctx, tc.Client, st.ID, 5*time.Second)
	}

	assertTailRetrievableViaFusion(t, ctx, tc.Client, storeReg, tailPassage, tailBody)
}

// ingestTailFixture builds the oversized fixture document, drives the REAL doc handler
// over it, and locates the passage entity carrying the tail marker. Splitting this out
// keeps the top-level test under revive's statement-count ceiling.
func ingestTailFixture(
	t *testing.T,
	ctx context.Context,
	tc *natsclient.TestClient,
) (tailPassage *handler.EntityState, tailBody string, states []*handler.EntityState, storeReg *storeregistry.Registry) {
	t.Helper()

	// One CONTENT store, registered in the shared registry exactly as the
	// ComponentManager populates it after the objectstore component Starts (ADR-063).
	store, err := objectstore.NewStoreWithConfig(ctx, tc.Client, objectstore.Config{
		BucketName:   semsourcegraph.BodyStoreBucket,
		InstanceName: semsourcegraph.BodyStoreInstance,
	})
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	storeReg = storeregistry.New()
	if err := storeReg.Register(semsourcegraph.BodyStoreInstance, store); err != nil {
		t.Fatalf("register store: %v", err)
	}

	docContent := buildOversizedTailDoc(t)

	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "retry-tail.md"), []byte(docContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Drive the REAL doc producer, exactly as production wiring does.
	h := dochandler.New(dochandler.WithBodyStore(store, semsourcegraph.BodyStoreInstance))
	states, err = h.IngestEntityStates(ctx, docSourceConfig{typ: "docs", path: docDir}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	var parent *handler.EntityState
	var passages []*handler.EntityState
	for _, st := range states {
		if docTypeOfState(st) == "passage" {
			passages = append(passages, st)
			continue
		}
		parent = st
	}
	if parent == nil {
		t.Fatal("no parent document entity emitted")
	}
	// Multiple passages matter here: the tail marker must land somewhere other than the
	// first passage, or this test cannot tell "genuinely past the cut" from "the whole
	// short document happened to fit in one entity anyway".
	if len(passages) < 2 {
		t.Fatalf("want a multi-passage document, got %d passages from %d states", len(passages), len(states))
	}

	tailPassage, tailBody = findTailPassage(t, ctx, storeReg, passages)
	assertPastTruncationCut(t, docContent, tailBody, tailPassage.ID)
	assertParentCarriesNoTail(t, parent)

	return tailPassage, tailBody, states, storeReg
}

// buildOversizedTailDoc constructs a markdown fixture comfortably larger than
// truncationLimit, with tailMarker placed well past it, and self-checks both facts so a
// future edit cannot silently shrink the fixture out from under this test.
func buildOversizedTailDoc(t *testing.T) string {
	t.Helper()

	// Three padding sections come before the tail so the marker's section is genuinely
	// the tail, not an artifact of a short document. Each sentence repeats enough times
	// to clear the splitter's 2000/400/6000 (ceiling/floor/hardMax) bounds several times
	// over (handler/doc/splitter.go), so the document yields multiple passages.
	const (
		overviewSentence = "Use exponential backoff with jitter when a call fails. "
		deadlineSentence = "Every call carries a deadline set by its caller. "
		breakerSentence  = "Trip the breaker after repeated failures and half-open later. "
		tailFillSentence = "Fill text before the marker so the tail begins well past the old truncation cut. "
		trailingSentence = "More trailing filler follows the marker to keep it inside its own passage body. "
	)
	docContent := "# Retry Policy\n\n" +
		"## Overview\n\n" + strings.Repeat(overviewSentence, 25) + "\n\n" +
		"## Deadlines\n\n" + strings.Repeat(deadlineSentence, 25) + "\n\n" +
		"## Circuit Breaking\n\n" + strings.Repeat(breakerSentence, 25) + "\n\n" +
		"## Tail Section\n\n" + strings.Repeat(tailFillSentence, 120) +
		tailMarker + "\n\n" + strings.Repeat(trailingSentence, 30) + "\n"

	// The tail SECTION alone must exceed truncationLimit, not merely the document.
	// This is what forces the splitter to subdivide an oversized section rather
	// than emit it whole, which is the mechanism that actually keeps the marker
	// inside the embedding window. With a tail section under the cap the document
	// could split cleanly on headings alone and the test would never exercise
	// subdivision — the very path that protects tail content.
	tailSectionStart := strings.Index(docContent, "## Tail Section")
	if tailSectionStart < 0 {
		t.Fatal("fixture lost its tail section heading")
	}
	if tailLen := len(docContent) - tailSectionStart; tailLen <= truncationLimit {
		t.Fatalf("tail section is only %d bytes; it must exceed the %d-byte cap so the "+
			"splitter is forced to subdivide it, otherwise this test never exercises "+
			"oversized-section subdivision", tailLen, truncationLimit)
	}

	// If a future edit shrinks the document back under the cap, the test would stop
	// exercising the tail-loss defect entirely and silently pass for the wrong reason.
	if len(docContent) <= truncationLimit {
		t.Fatalf("fixture document is only %d bytes; must exceed the %d-byte substrate "+
			"truncation cap or this test exercises nothing", len(docContent), truncationLimit)
	}
	if n := strings.Count(docContent, tailMarker); n != 1 {
		t.Fatalf("tail marker must appear exactly once in the fixture, found %d — a "+
			"repeated marker cannot prove WHICH occurrence a passage carries", n)
	}
	markerOffset := strings.Index(docContent, tailMarker)
	if markerOffset <= truncationLimit {
		t.Fatalf("tail marker sits at byte offset %d, at or before the %d-byte truncation "+
			"cap; move it deeper into the fixture so this test actually exercises tail content",
			markerOffset, truncationLimit)
	}
	return docContent
}

// findTailPassage resolves every passage's body through the shared registry — the same
// path graph-embedding uses (shouldFetchViaStorageRef → queueEmbeddingWithStorageRef) —
// and returns the one containing tailMarker. A miss here IS the silent-loss regression
// this test exists to catch: before passage chunking there was nowhere else for this text
// to live, so it would never have surfaced.
func findTailPassage(
	t *testing.T,
	ctx context.Context,
	storeReg *storeregistry.Registry,
	passages []*handler.EntityState,
) (*handler.EntityState, string) {
	t.Helper()

	var tailPassage *handler.EntityState
	var tailBody string
	for _, p := range passages {
		// Every passage must carry a StorageRef, exactly like Assertion A of
		// TestIntegration_DocBodyOffload_ResolvesViaStoreRegistry: without one,
		// graph-embedding's shouldFetchViaStorageRef never fires and the passage is
		// unindexed in practice — the same silent loss wearing a different hat, whether
		// or not this particular passage turns out to be the one carrying the marker.
		if p.StorageRef == nil {
			t.Fatalf("passage %s carries no StorageRef — it would never be embedded", p.ID)
		}
		resolved, ok := storeReg.Store(p.StorageRef.StorageInstance)
		if !ok {
			t.Fatalf("registry does not resolve StorageInstance %q for %s", p.StorageRef.StorageInstance, p.ID)
		}
		body, gErr := resolved.Get(ctx, p.StorageRef.Key)
		if gErr != nil {
			t.Fatalf("registry fetch for %s: %v", p.ID, gErr)
		}
		if strings.Contains(string(body), tailMarker) {
			tailPassage = p
			tailBody = string(body)
		}
	}
	if tailPassage == nil {
		t.Fatalf("no passage entity carries the tail marker %q among %d passages — this is "+
			"precisely the silent tail-loss defect passage chunking exists to fix",
			tailMarker, len(passages))
	}
	return tailPassage, tailBody
}

// assertPastTruncationCut proves the tail passage's own byte span — not just the
// marker's offset in the source file — falls past truncationLimit. tailBody is an exact,
// contiguous slice of docContent (passage splitting is a byte-span partition;
// handler/doc/splitter.go), and tailMarker is unique in the fixture, so tailBody can only
// appear at the ONE location containing that unique text: strings.Index here is not
// ambiguous even though large stretches of the fixture are repetitive filler.
func assertPastTruncationCut(t *testing.T, docContent, tailBody, passageID string) {
	t.Helper()

	passageStart := strings.Index(docContent, tailBody)
	if passageStart < 0 {
		t.Fatalf("tail passage body is not a verbatim substring of the source document — "+
			"passage %s diverges from disk content", passageID)
	}
	// Checked FIRST because it is the more fundamental property, and because a
	// single oversized passage spanning the whole tail would otherwise trip the
	// start check below and report the less informative complaint.
	//
	// "The marker lives in a passage" is NOT sufficient. If that passage were
	// itself larger than truncationLimit, graph-embedding would truncate it at
	// 8000 exactly as it truncated the whole file before, and the marker would be
	// just as unreachable — while every other assertion here still passed.
	// Chunking fixes the defect only if the passage carrying the tail fits INSIDE
	// the embedding window. splitter.go's hardMax (6000) exists to guarantee that;
	// assert the guarantee rather than trusting it, because this is the test that
	// would catch hardMax being raised above the substrate's cap.
	if len(tailBody) > truncationLimit {
		t.Fatalf("tail passage %s is %d bytes, larger than the %d-byte embedding cap; "+
			"graph-embedding would truncate it and lose the marker exactly as the "+
			"whole-file path did — splitting alone does not fix the defect unless "+
			"each passage fits the window",
			passageID, len(tailBody), truncationLimit)
	}

	if passageStart <= truncationLimit {
		t.Fatalf("tail passage %s starts at byte %d, at or before the %d-byte truncation "+
			"cap; this is not tail content and proves nothing about the defect",
			passageID, passageStart, truncationLimit)
	}
}

// assertParentCarriesNoTail is the counterfactual half of this test: before passage
// chunking there was only ONE entity per file, and its embedded text stopped at
// truncationLimit — so a marker sitting well past the cap (already asserted) was
// unreachable through that entity. Now the parent must hold no body at all, so it cannot
// reintroduce the marker through any path.
func assertParentCarriesNoTail(t *testing.T, parent *handler.EntityState) {
	t.Helper()

	if parent.StorageRef != nil {
		t.Fatalf("parent %s still carries a StorageRef %+v; the parent must hold no body",
			parent.ID, parent.StorageRef)
	}
	for i := range parent.Triples {
		s, ok := parent.Triples[i].Object.(string)
		if ok && strings.Contains(s, tailMarker) {
			t.Fatalf("parent %s carries the tail marker in triple %s=%q; the parent must "+
				"never carry document prose, or the old whole-file embedding path is still alive",
				parent.ID, parent.Triples[i].Predicate, s)
		}
	}
}

// assertTailRetrievableViaFusion drives the fusion docs engine over the shared registry
// resolver — mirroring Assertion B of TestIntegration_DocBodyOffload_ResolvesViaStoreRegistry
// — and asserts the hydrated node body is the verbatim tail passage, marker included.
//
// Deliberately NOT asserted: rank. A separate, known defect (tasks.md 9.5) means retrieval
// currently prefers an "override" section's passage over a "default" one for unrelated
// reasons; coupling this test to top-N rank would fail it for a cause it is not testing.
func assertTailRetrievableViaFusion(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	storeReg *storeregistry.Registry,
	tailPassage *handler.EntityState,
	tailBody string,
) {
	t.Helper()

	engine := fusion.NewEngine(
		fusionnats.New(client, 0),
		fusion.NewBodyResolver(storeReg),
	)
	lens := docsPrefixLens{docs.New()}
	tailLabel := labelOfState(tailPassage)

	var node *fusion.Node
	var last fusion.Response
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: tailPassage.ID, Want: []fusion.Want{fusion.WantBody},
		}, lens)
		if ferr != nil {
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		last = resp
		if n := findNode(resp.Nodes, tailLabel); n != nil && n.Body != "" {
			node = n
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if node == nil {
		t.Fatalf("tail passage node with a hydrated body never appeared — ready=%v state=%s nodes=%+v",
			last.Index.Ready, last.Index.State, last.Nodes)
	}
	if node.Body != tailBody {
		t.Fatalf("hydrated tail passage body = %q; want verbatim %q", node.Body, tailBody)
	}
	if !strings.Contains(node.Body, tailMarker) {
		t.Fatalf("hydrated tail passage body does not contain the tail marker %q — the "+
			"exact regression this test exists to catch", tailMarker)
	}
}
