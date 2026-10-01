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
func replayEntity(id, reason string) gtypes.EntityState {
	e := gtypes.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: source.DocFilePath, Object: "guide.md"}, {Subject: id, Predicate: semvocab.EntityIndexingProfile, Object: graph.IndexingProfileContent}}}
	if reason != "" {
		e.Triples = append(e.Triples, staleTriple(id, reason))
	}
	return e
}
func replayComponent(p *replayRequester, m *replayMutator) *Component {
	return &Component{authority: entityid.Authority{Org: "acme", Platform: "semsource"}, running: true, config: DefaultConfig(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), queryClient: &prefixQuerier{client: p}, mutClient: m}
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
func TestSourceProjectionPortsAreDeclaredWithCustomOutputs(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{"ports":{"outputs":[]}}`), json.RawMessage(`{"ports":null}`)} {
		ports, err := DeclarePorts(raw, "supersession")
		if err != nil {
			t.Fatal(err)
		}
		if len(ports.Inputs) != 1 || ports.Inputs[0].Config.ResourceID() != "nats-request:"+sourceProjectionSubject {
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
	release, err := c.runGate.acquire(context.Background())
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
