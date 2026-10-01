package supersession

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	source "github.com/c360studio/semsource/source/vocabulary"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/projection"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

type replayRequester struct {
	pages   map[string]gtypes.PrefixQueryResponse
	fail    string
	cursors []string
	raw     []byte
}

func (r *replayRequester) RequestClassified(_ context.Context, _ string, raw []byte, _ time.Duration) ([]byte, error) {
	var req gtypes.PrefixQueryRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if strings.HasSuffix(req.Prefix, ".") {
		return nil, errors.New("graph prefix has empty trailing segment")
	}
	if r.raw != nil {
		return r.raw, nil
	}
	r.cursors = append(r.cursors, req.Cursor)
	if r.fail != "" && req.Cursor == r.fail {
		return nil, errors.New("query unavailable")
	}
	page, ok := r.pages[req.Cursor]
	if !ok {
		return nil, errors.New("unexpected cursor")
	}
	return json.Marshal(page)
}

type replayMutator struct {
	entities map[string]gtypes.EntityState
	fail     string
	writes   []string
}

func (m *replayMutator) ReadAuthoritative(_ context.Context, id string) (*gtypes.ExactEntity, error) {
	e, ok := m.entities[id]
	if !ok {
		return nil, errors.New("entity absent")
	}
	return &gtypes.ExactEntity{Entity: &e, KVRevision: 1}, nil
}
func (m *replayMutator) Reconcile(_ context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	if r.EntityID == m.fail {
		return projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationRevisionConflict, Code: gtypes.ErrorCodeRevisionMismatch, Commit: projection.CommitNotCommitted, Err: errors.New("revision conflict")}
	}
	e := m.entities[r.EntityID]
	out := []message.Triple{}
	for _, tr := range e.Triples {
		if tr.Predicate != source.EntityLifecycleStale {
			out = append(out, tr)
		}
	}
	e.Triples = append(out, r.Desired...)
	m.entities[e.ID] = e
	m.writes = append(m.writes, e.ID)
	return projection.MutationReceipt{Entity: &e, KVRevision: 2, Commit: projection.CommitVerified}, nil
}
func replayBinding() sourceintent.Binding {
	return sourceintent.Binding{Authority: entityid.Authority{Org: "acme", Platform: "semsource"}, Namespace: "test", Handle: "docs", Generation: 1, BootEpoch: "boot", Factory: "doc-source", ConfigDigest: "digest"}
}
func replayRemoval() sourceintent.RemovalProjection {
	b := replayBinding()
	return sourceintent.RemovalProjection{Binding: b, Effects: &replayEffectFence{}, Scope: sourceintent.SourceScope{Authority: b.Authority, Handle: b.Handle, Factory: b.Factory, ConfigDigest: b.ConfigDigest, Selectors: []sourceintent.Selector{{Prefix: "acme.semsource.docs.web.", System: "docs", Domain: "web"}}}}
}
func replayEntity(id, reason string) gtypes.EntityState {
	e := gtypes.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: source.DocFilePath, Object: "guide.md"}, {Subject: id, Predicate: semvocab.EntityIndexingProfile, Object: graph.IndexingProfileContent}}}
	if reason != "" {
		e.Triples = append(e.Triples, staleTriple(id, reason))
	}
	return e
}
func replayComponent(p *replayRequester, m *replayMutator) *Component {
	return &Component{authority: replayBinding().Authority, running: true, config: DefaultConfig(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), queryClient: &prefixQuerier{client: p}, mutClient: m}
}

func TestRemovalReplayPages(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	b := replayEntity("acme.semsource.docs.web.chunk.b", "")
	for _, mode := range []string{"complete", "cycle", "query_failure", "foreign_entity"} {
		t.Run(mode, func(t *testing.T) {
			p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a}, NextCursor: "opaque/+="}, "opaque/+=": {Entities: []gtypes.EntityState{b}}}}
			if mode == "cycle" {
				p.pages["opaque/+="] = gtypes.PrefixQueryResponse{Entities: []gtypes.EntityState{b}, NextCursor: "opaque/+="}
			}
			if mode == "query_failure" {
				p.fail = "opaque/+="
			}
			if mode == "foreign_entity" {
				p.pages["opaque/+="] = gtypes.PrefixQueryResponse{Entities: []gtypes.EntityState{replayEntity("other.semsource.docs.web.doc.c", "")}}
			}
			m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}}
			c := replayComponent(p, m)
			r, err := c.ApplyRemoval(context.Background(), replayRemoval())
			if mode == "complete" {
				if err != nil || !r.Complete || r.Enumerated != 2 || r.Marked != 2 {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				if len(p.cursors) != 2 {
					t.Fatalf("cursors %v", p.cursors)
				}
			} else {
				if err == nil || r.Complete {
					t.Fatalf("false completion %+v err=%v", r, err)
				}
				if len(m.writes) != 0 {
					t.Fatalf("mutated incomplete enumeration %v", m.writes)
				}
			}
		})
	}
}
func TestRemovalReplayIdempotentAndPartialMutation(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", graph.LifecycleReasonFileDeleted)
	b := replayEntity("acme.semsource.docs.web.chunk.b", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a, b}}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}, fail: b.ID}
	c := replayComponent(p, m)
	r, err := c.ApplyRemoval(context.Background(), replayRemoval())
	if err == nil || r.Complete || r.Marked != 1 || len(r.Failures) != 1 {
		t.Fatalf("partial result=%+v err=%v", r, err)
	}
	if tripleString(m.entities[a.ID].Triples, source.EntityLifecycleStale) != graph.LifecycleReasonSourceRemoved {
		t.Fatal("did not replace older stale reason")
	}
	m.fail = ""
	r, err = c.ApplyRemoval(context.Background(), replayRemoval())
	if err != nil || !r.Complete || r.Marked != 1 {
		t.Fatalf("retry %+v %v", r, err)
	}
	r, err = c.ApplyRemoval(context.Background(), replayRemoval())
	if err != nil || !r.Complete || r.Marked != 0 {
		t.Fatalf("duplicate %+v %v", r, err)
	}
}
func TestReactivationCurrentEntityReceipts(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", graph.LifecycleReasonSourceRemoved)
	b := replayEntity("acme.semsource.docs.web.doc.b", graph.LifecycleReasonSourceRemoved)
	for _, mode := range []string{"current", "old_epoch", "mismatch", "newer_reason"} {
		t.Run(mode, func(t *testing.T) {
			aa := a
			if mode == "newer_reason" {
				aa = replayEntity(a.ID, graph.LifecycleReasonFileDeleted)
			}
			m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: aa, b.ID: b}}
			c := replayComponent(&replayRequester{}, m)
			fp, err := sourceintent.RetainedFingerprint(a.ID, a.Triples, a.StorageRef)
			if err != nil {
				t.Fatal(err)
			}
			bind := replayBinding()
			entries := []sourceintent.ManifestEntry{{EntityID: a.ID, Fingerprint: fp}}
			digest, _ := sourceintent.ManifestDigest(entries)
			req := sourceintent.ReactivationProjection{Binding: bind, Manifest: sourceintent.SeedManifest{Binding: bind, BatchID: "initial", Initial: true, Successful: true, Entries: entries, Digest: digest}, Receipts: []sourceintent.Receipt{{Binding: bind, BatchID: "initial", EntityID: a.ID, Fingerprint: fp, Acknowledged: true}}}
			if mode == "old_epoch" {
				req.Receipts[0].Binding.BootEpoch = "old"
			}
			if mode == "mismatch" {
				req.Receipts[0].Fingerprint = "wrong"
			}
			r, err := c.ApplyReactivation(context.Background(), req)
			if mode == "current" {
				var blocker *sourceintent.Blocker
				if !errors.As(err, &blocker) || blocker.Code != sourceintent.CodeConditionalReconcileUnavailable || r.Complete || r.Cleared != 0 || len(m.writes) != 0 {
					t.Fatalf("current %+v %v", r, err)
				}
			} else if mode == "newer_reason" {
				if err != nil || !r.Complete || r.Cleared != 0 {
					t.Fatalf("reason %+v %v", r, err)
				}
			} else if err == nil || r.Complete || len(m.writes) > 0 {
				t.Fatalf("unsafe clear %+v %v %v", r, err, m.writes)
			}
			if !isMarkedStale(m.entities[b.ID].Triples) {
				t.Fatal("offline-deleted entity revived")
			}
		})
	}
}
func TestRemovalProjectionCancellation(t *testing.T) {
	c := replayComponent(&replayRequester{}, &replayMutator{})
	release, err := c.runGate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ApplyRemoval(ctx, replayRemoval())
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("canceled gate returned %v", err)
	}
}
func TestSourceRemovedStickyAcrossLegacyOracles(t *testing.T) {
	id := "acme.semsource.docs.web.doc.a"
	e := replayEntity(id, graph.LifecycleReasonSourceRemoved)
	for _, stat := range []func(string) bool{func(string) bool { return true }, func(string) bool { return false }, nil} {
		m, c, _ := decideLifecycleActions([]gtypes.EntityState{e}, graph.LifecycleReasonFileDeleted, stat)
		if len(m) > 0 || len(c) > 0 {
			t.Fatalf("legacy changed source_removed: %v %v", m, c)
		}
	}
	oracle, err := livenessOracle(graph.LifecycleRunRequest{Absent: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	m, c, _ := decideLifecycleActions([]gtypes.EntityState{e}, graph.LifecycleReasonFileDeleted, oracle)
	if len(m) > 0 || len(c) > 0 {
		t.Fatal("empty absent cleared source_removed")
	}
}
func TestPassageUsesExactParent(t *testing.T) {
	p1 := replayEntity("acme.semsource.docs.web.doc.a", "")
	p1.Triples = append(p1.Triples, message.Triple{Predicate: source.DocChunkCount, Object: 0})
	p2 := replayEntity("acme.semsource.docs.web.doc.b", "")
	p2.Triples = append(p2.Triples, message.Triple{Predicate: source.DocChunkCount, Object: 2})
	ch := replayEntity("acme.semsource.docs.web.chunk.b0", "")
	ch.Triples = append(ch.Triples, message.Triple{Predicate: source.DocChunkIndex, Object: 0}, message.Triple{Predicate: source.CodeBelongs, Object: p2.ID, Datatype: message.EntityReferenceDatatype})
	m, c, _ := decideLifecycleActions([]gtypes.EntityState{p1, p2, ch}, graph.LifecycleReasonFileDeleted, func(string) bool { return true })
	if len(m) > 0 || len(c) > 0 {
		t.Fatalf("sibling parent's count applied: %v %v", m, c)
	}
}

func TestRemovalReplayTraversesBeyondLegacyCap(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	b := replayEntity("acme.semsource.docs.web.doc.b", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a}, NextCursor: "next"}, "next": {Entities: []gtypes.EntityState{b}}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}}
	c := replayComponent(p, m)
	c.config.MaxEntities = 1
	result, err := c.ApplyRemoval(context.Background(), replayRemoval())
	if err != nil || !result.Complete || result.Enumerated != 2 || result.Marked != 2 || len(p.cursors) != 2 {
		t.Fatalf("retained replay inherited legacy cap: %+v %v %v", result, err, p.cursors)
	}
}
func TestLegacyLifecycleTruncationFailsBeforeMutation(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a}, NextCursor: "next"}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a}}
	c := replayComponent(p, m)
	c.config.MaxEntities = 1
	result, err := c.runLifecyclePass(context.Background(), graph.LifecycleRunRequest{Org: "acme", Systems: []string{"docs"}, Reason: graph.LifecycleReasonFileDeleted})
	if err == nil || result.Marked != 0 || len(m.writes) != 0 {
		t.Fatalf("legacy truncated set committed: %+v %v %v", result, err, m.writes)
	}
}
func TestRemovalReplayScopeAndArtifactIsolation(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	b := replayEntity("acme.semsource.docs.web.doc.b", "")
	b.Triples[0].Object = "sibling.md"
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a, b}}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}}
	c := replayComponent(p, m)
	req := replayRemoval()
	req.Scope.Selectors[0].ArtifactPredicate = source.DocFilePath
	req.Scope.Selectors[0].Artifacts = []string{"guide.md"}
	result, err := c.ApplyRemoval(context.Background(), req)
	if err != nil || !result.Complete || result.Marked != 1 || isMarkedStale(m.entities[b.ID].Triples) {
		t.Fatalf("artifact isolation: %+v %v", result, err)
	}
	for _, bad := range []string{"foreign_authority", "mismatched_handle", "wide_prefix", "empty_selector", "ambiguous_artifact"} {
		t.Run(bad, func(t *testing.T) {
			req := replayRemoval()
			switch bad {
			case "foreign_authority":
				req.Binding.Authority.Org = "other"
			case "mismatched_handle":
				req.Scope.Handle = "sibling"
			case "wide_prefix":
				req.Scope.Selectors[0].Prefix = "acme.semsource."
			case "empty_selector":
				req.Scope.Selectors = nil
			case "ambiguous_artifact":
				req.Scope.Selectors[0].Artifacts = []string{"guide.md"}
			}
			before := len(m.writes)
			r, err := c.ApplyRemoval(context.Background(), req)
			if err == nil || r.Complete || len(m.writes) != before {
				t.Fatalf("invalid scope accepted: %+v %v", r, err)
			}
		})
	}
}
func TestSourceProjectionRPCRefusesAllEffects(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a}}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a}}
	c := replayComponent(p, m)
	request := replayRemoval()
	raw, err := json.Marshal(sourcelifecycle.ProjectionRequest{Removal: &request})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{raw, []byte(`{}`), []byte(`{"unknown":true}`), []byte(`{} {}`), []byte(`{"removal":{},"reactivation":{}}`)} {
		reply, err := c.handleSourceProjection(context.Background(), body)
		if err != nil {
			t.Fatal(err)
		}
		var response sourcelifecycle.ProjectionReply
		if err := json.Unmarshal(reply, &response); err != nil {
			t.Fatal(err)
		}
		if response.Error == nil || response.Result.Complete || response.Result.Marked != 0 || len(m.writes) != 0 || len(p.cursors) != 0 {
			t.Fatalf("wire request authorized effects: %+v queries=%v writes=%v", response, p.cursors, m.writes)
		}
	}
}
func TestSourceProjectionPortsAreDeclaredWithCustomOutputs(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{"ports":{"outputs":[]}}`), json.RawMessage(`{"ports":null}`)} {
		ports, err := DeclarePorts(raw, "supersession")
		if err != nil {
			t.Fatal(err)
		}
		if len(ports.Inputs) != 1 || ports.Inputs[0].Config.ResourceID() != "nats-request:"+sourcelifecycle.ProjectionSubject {
			t.Fatalf("source projection input missing: %+v", ports)
		}
	}
}
func TestLegacyLifecycleFailureAndSharedCancellation(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a}}}}
	m := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a}, fail: a.ID}
	c := replayComponent(p, m)
	req := graph.LifecycleRunRequest{Org: "acme", Systems: []string{"docs"}, Reason: graph.LifecycleReasonFileDeleted}
	r, err := c.runLifecyclePass(context.Background(), req)
	if err == nil || r.Marked != 0 {
		t.Fatalf("legacy hid failure: %+v %v", r, err)
	}
	release, err := c.runGate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.runLifecyclePass(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("legacy gate ignored cancellation: %v", err)
	}
	if _, err := c.runPass(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("lineage gate ignored cancellation: %v", err)
	}
}

func TestRemovalReplayRejectsMalformedPages(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"entities":"broken"}`, `{"entities":[{"id":"malformed"}]}`, `[1]`} {
		p := &replayRequester{raw: []byte(raw)}
		m := &replayMutator{}
		c := replayComponent(p, m)
		result, err := c.ApplyRemoval(context.Background(), replayRemoval())
		if err == nil || result.Complete || len(m.writes) > 0 {
			t.Fatalf("malformed page accepted %s: %+v %v", raw, result, err)
		}
	}
}
func TestRemovalReplayEmptyRetainedSet(t *testing.T) {
	c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {}}}, &replayMutator{})
	result, err := c.ApplyRemoval(context.Background(), replayRemoval())
	if err != nil || !result.Complete || result.Enumerated != 0 || result.Marked != 0 {
		t.Fatalf("empty retained set: %+v %v", result, err)
	}
}

type lostReplyMutator struct {
	*replayMutator
	lost bool
}

func (m *lostReplyMutator) Reconcile(ctx context.Context, request projection.ReconcileMutation) (projection.MutationReceipt, error) {
	receipt, err := m.replayMutator.Reconcile(ctx, request)
	if err == nil && !m.lost {
		m.lost = true
		return projection.MutationReceipt{}, errors.New("reply lost after commit")
	}
	return receipt, err
}
func TestRemovalReadbackCannotResolveLostReplyFence(t *testing.T) {
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}
	mut := &lostReplyMutator{replayMutator: &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}}
	c := replayComponent(p, mut.replayMutator)
	c.mutClient = mut
	request := replayRemoval()
	fence := request.Effects.(*replayEffectFence)
	result, err := c.ApplyRemoval(t.Context(), request)
	if err == nil || result.Complete || result.Marked != 0 || fence.active == nil {
		t.Fatalf("lost reply counted as committed: %+v %v fence=%+v", result, err, fence.active)
	}
	// Only the current retained set is observable on readback. It cannot resolve
	// the unknown attempt; the coordinator refuses actual replay while fenced.
	result, err = c.ApplyRemoval(t.Context(), request)
	if err != nil || !result.Complete || result.Marked != 0 || len(mut.writes) != 1 || fence.active == nil || len(fence.outcomes) != 1 || fence.outcomes[0] != sourceintent.EffectUnknown {
		t.Fatalf("readback resolved unknown fence: %+v %v writes=%v fence=%+v outcomes=%v", result, err, mut.writes, fence.active, fence.outcomes)
	}
}
func replayReactivation(t *testing.T, e gtypes.EntityState) sourceintent.ReactivationProjection {
	t.Helper()
	b := replayBinding()
	fp, err := sourceintent.RetainedFingerprint(e.ID, e.Triples, e.StorageRef)
	if err != nil {
		t.Fatal(err)
	}
	entries := []sourceintent.ManifestEntry{{EntityID: e.ID, Fingerprint: fp}}
	digest, err := sourceintent.ManifestDigest(entries)
	if err != nil {
		t.Fatal(err)
	}
	return sourceintent.ReactivationProjection{Binding: b, Manifest: sourceintent.SeedManifest{Binding: b, BatchID: "initial", Initial: true, Successful: true, Entries: entries, Digest: digest}, Receipts: []sourceintent.Receipt{{Binding: b, BatchID: "initial", EntityID: e.ID, Fingerprint: fp, Acknowledged: true}}}
}
func TestReactivationRejectsUnsealedOrIncompleteEvidence(t *testing.T) {
	for _, mode := range []string{"digest", "failed_seed", "no_epoch", "missing_receipt", "duplicate_receipt", "unacknowledged", "foreign_entity", "retained_facts_changed", "graph_read_failed", "empty_current"} {
		t.Run(mode, func(t *testing.T) {
			e := replayEntity("acme.semsource.docs.web.doc.a", graph.LifecycleReasonSourceRemoved)
			req := replayReactivation(t, e)
			mut := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
			c := replayComponent(&replayRequester{}, mut)
			switch mode {
			case "digest":
				req.Manifest.Digest = "wrong"
			case "failed_seed":
				req.Manifest.Successful = false
			case "no_epoch":
				req.Binding.BootEpoch = ""
			case "missing_receipt":
				req.Receipts = nil
			case "duplicate_receipt":
				req.Receipts = append(req.Receipts, req.Receipts[0])
			case "unacknowledged":
				req.Receipts[0].Acknowledged = false
			case "foreign_entity":
				req.Manifest.Entries[0].EntityID = "other.semsource.docs.web.doc.a"
				req.Receipts[0].EntityID = req.Manifest.Entries[0].EntityID
				req.Manifest.Digest, _ = sourceintent.ManifestDigest(req.Manifest.Entries)
			case "retained_facts_changed":
				changed := e
				changed.Triples = append([]message.Triple(nil), e.Triples...)
				changed.Triples[0].Object = "changed.md"
				mut.entities[e.ID] = changed
			case "graph_read_failed":
				delete(mut.entities, e.ID)
			case "empty_current":
				req.Manifest.Entries = nil
				req.Manifest.Digest, _ = sourceintent.ManifestDigest(nil)
				req.Receipts = nil
			}
			result, err := c.ApplyReactivation(context.Background(), req)
			if mode == "empty_current" {
				if err != nil || !result.Complete || result.Cleared != 0 {
					t.Fatalf("empty current seed: %+v %v", result, err)
				}
			} else if err == nil || result.Complete {
				t.Fatalf("invalid proof accepted: %+v %v", result, err)
			}
			if len(mut.writes) > 0 {
				t.Fatalf("unfenced clear: %v", mut.writes)
			}
		})
	}
}

func TestReactivationLiveBatchRequiresSameEpochInitialProof(t *testing.T) {
	for _, mode := range []string{"missing_initial", "old_initial", "failed_initial", "missing_initial_receipt", "valid_both"} {
		t.Run(mode, func(t *testing.T) {
			e := replayEntity("acme.semsource.docs.web.doc.a", graph.LifecycleReasonSourceRemoved)
			req := replayReactivation(t, e)
			initial := req.Manifest
			receipts := append([]sourceintent.Receipt(nil), req.Receipts...)
			req.Manifest.Initial = false
			req.Manifest.BatchID = "watch-1"
			req.Receipts[0].BatchID = "watch-1"
			if mode != "missing_initial" {
				req.InitialManifest = &initial
				req.InitialReceipts = receipts
			}
			switch mode {
			case "old_initial":
				req.InitialManifest.Binding.BootEpoch = "old"
			case "failed_initial":
				req.InitialManifest.Successful = false
			case "missing_initial_receipt":
				req.InitialReceipts = nil
			}
			mut := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
			c := replayComponent(&replayRequester{}, mut)
			result, err := c.ApplyReactivation(context.Background(), req)
			var blocker *sourceintent.Blocker
			if !errors.As(err, &blocker) || result.Complete || len(mut.writes) != 0 {
				t.Fatalf("unfenced live batch: %+v %v", result, err)
			}
			if mode == "valid_both" && blocker.Code != sourceintent.CodeConditionalReconcileUnavailable {
				t.Fatalf("valid sealed batches not admitted to conditional boundary: %v", err)
			}
			if mode != "valid_both" && blocker.Code == sourceintent.CodeConditionalReconcileUnavailable {
				t.Fatalf("invalid initial proof admitted: %v", err)
			}
		})
	}
}
