package fusion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/ontology"
)

// --- fake graph + test lens ---------------------------------------------------

type fakeGraph struct {
	status      IndexStatus
	entities    map[string]*Entity
	out         map[string][]Edge // id → outgoing edges
	resolve     map[string][]string
	names       []string
	entitiesErr error // injected backend failure for Entities
}

func newFakeGraph() *fakeGraph {
	return &fakeGraph{
		status:   IndexStatus{Ready: true, State: StateReady, Revision: "abc"},
		entities: map[string]*Entity{},
		out:      map[string][]Edge{},
		resolve:  map[string][]string{},
	}
}

// add registers an entity (6-part ID), its display fields, body, and call edges.
func (f *fakeGraph) add(id, name, typ, path string, start, end int, class, body string, calls ...string) {
	tr := []message.Triple{
		{Subject: id, Predicate: "dc.terms.title", Object: name},
		{Subject: id, Predicate: "code.artifact.type", Object: typ},
		{Subject: id, Predicate: "code.artifact.path", Object: path},
		{Subject: id, Predicate: "code.metric.start_line", Object: start},
		{Subject: id, Predicate: "code.metric.end_line", Object: end},
		{Subject: id, Predicate: "test.body", Object: body},
	}
	if class != "" {
		tr = append(tr, message.Triple{Subject: id, Predicate: ontology.ClassPredicate, Object: class})
	}
	f.entities[id] = &Entity{ID: id, Triples: tr}
	f.names = append(f.names, name)
	for _, c := range calls {
		f.out[id] = append(f.out[id], Edge{Predicate: "code.relationship.calls", Target: c})
	}
}

func (f *fakeGraph) Status(context.Context) (IndexStatus, error) { return f.status, nil }

func (f *fakeGraph) Resolve(_ context.Context, query string, _ ResolveMode, _ int) ([]string, error) {
	return f.resolve[query], nil
}

func (f *fakeGraph) Entity(_ context.Context, id string) (*Entity, error) {
	return f.entities[id], nil
}

func (f *fakeGraph) Entities(_ context.Context, ids []string) ([]*Entity, error) {
	if f.entitiesErr != nil {
		return nil, f.entitiesErr
	}
	var out []*Entity
	for _, id := range ids {
		if e := f.entities[id]; e != nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeGraph) Neighbors(_ context.Context, id string, preds []string, dir Direction) ([]Edge, error) {
	predOK := func(p string) bool { return slices.Contains(preds, p) }
	if dir == Outgoing {
		var out []Edge
		for _, e := range f.out[id] {
			if predOK(e.Predicate) {
				out = append(out, e)
			}
		}
		return out, nil
	}
	// Incoming: find who points at id.
	var in []Edge
	for src, edges := range f.out {
		for _, e := range edges {
			if e.Target == id && predOK(e.Predicate) {
				in = append(in, Edge{Predicate: e.Predicate, Target: src})
			}
		}
	}
	return in, nil
}

func (f *fakeGraph) Names(_ context.Context, _ string, limit int) ([]string, error) {
	if len(f.names) > limit {
		return f.names[:limit], nil
	}
	return f.names, nil
}

// testLens is a minimal code-flavored lens reading test triples.
type testLens struct{}

func (testLens) Name() string { return "test" }
func (testLens) ResolveMode(q string) ResolveMode {
	if strings.Contains(q, " ") {
		return ResolveSemantic
	}
	return ResolveSymbol
}
func (testLens) Edges() []EdgeSpec {
	return []EdgeSpec{{Predicate: "code.relationship.calls", OutgoingRole: "callee", IncomingRole: "caller"}}
}
func (testLens) Label(e *Entity) string { return e.First("dc.terms.title") }
func (testLens) Kind(e *Entity) string  { return e.First("code.artifact.type") }
func (testLens) Location(e *Entity) Locator {
	return Locator{
		Path:  e.First("code.artifact.path"),
		Lines: [2]int{e.FirstInt("code.metric.start_line"), e.FirstInt("code.metric.end_line")},
	}
}
func (testLens) Hydrate(_ context.Context, e *Entity) (string, error) {
	return e.First("test.body"), nil
}

func fuse(t *testing.T, g *fakeGraph, req Request) Response {
	t.Helper()
	resp, err := NewEngine(g).Fuse(context.Background(), req, testLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	return resp
}

// --- tests --------------------------------------------------------------------

func TestNotReadyEnvelope(t *testing.T) {
	g := newFakeGraph()
	g.status = IndexStatus{Ready: false, State: StateBuilding}
	g.add("o.semsource.golang.s.function.f", "Foo", "function", "a.go", 1, 3, cco.Algorithm, "body")
	g.resolve["Foo"] = []string{"o.semsource.golang.s.function.f"}

	resp := fuse(t, g, Request{Query: "Foo"})
	if resp.Index.Ready {
		t.Fatal("expected not ready")
	}
	if len(resp.Nodes) != 0 || len(resp.Misses) != 0 {
		t.Fatal("not-ready must be empty + no misses")
	}
}

func TestReadyHit(t *testing.T) {
	g := newFakeGraph()
	id := "o.semsource.golang.s.function.f"
	g.add(id, "Foo", "function", "a.go", 1, 3, cco.Algorithm, "func Foo() {}")
	g.resolve["Foo"] = []string{id}

	resp := fuse(t, g, Request{Query: "Foo", Want: []Want{WantBody}})
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "Foo" {
		t.Fatalf("expected node Foo, got %+v", resp.Nodes)
	}
	if resp.Nodes[0].Body != "func Foo() {}" {
		t.Fatalf("expected verbatim body, got %q", resp.Nodes[0].Body)
	}
	if resp.Nodes[0].Handle != id {
		t.Fatalf("handle should be the opaque entity id, got %q", resp.Nodes[0].Handle)
	}
	if resp.Provenance != ProvenanceDeterministic {
		t.Fatalf("symbol resolve should be deterministic, got %q", resp.Provenance)
	}
}

func TestReadyMissWithDidYouMean(t *testing.T) {
	g := newFakeGraph()
	g.add("o.semsource.golang.s.struct.pm", "PlanManager", "struct", "a.go", 1, 3, cco.SoftwareCode, "x")
	// "PlanMgr" resolves to nothing.

	resp := fuse(t, g, Request{Query: "PlanMgr"})
	if len(resp.Nodes) != 0 {
		t.Fatalf("absent query must return no nodes, got %+v", resp.Nodes)
	}
	if len(resp.Misses) != 1 || !slices.Contains(resp.Misses[0].DidYouMean, "PlanManager") {
		t.Fatalf("expected a miss suggesting PlanManager, got %+v", resp.Misses)
	}
}

func TestRelations(t *testing.T) {
	g := newFakeGraph()
	dispatch := "o.semsource.golang.s.function.d"
	onEvent := "o.semsource.golang.s.method.oe"
	g.add(dispatch, "dispatch", "function", "d.go", 1, 5, cco.Algorithm, "", onEvent)
	g.add(onEvent, "OnEvent", "method", "c.go", 10, 20, cco.Algorithm, "")
	g.resolve["OnEvent"] = []string{onEvent}
	g.resolve["dispatch"] = []string{dispatch}

	caller := fuse(t, g, Request{Query: "OnEvent", Want: []Want{WantRelations}})
	if got := caller.Nodes[0].Relations["caller"]; len(got) != 1 || got[0].Name != "dispatch" {
		t.Fatalf("expected caller dispatch, got %+v", caller.Nodes[0].Relations)
	}
	callee := fuse(t, g, Request{Query: "dispatch", Want: []Want{WantRelations}})
	if got := callee.Nodes[0].Relations["callee"]; len(got) != 1 || got[0].Name != "OnEvent" {
		t.Fatalf("expected callee OnEvent, got %+v", callee.Nodes[0].Relations)
	}
}

// TestOntologyCoherenceReranks: an outlier class that resolved first is demoted
// below the modal-class members by the ontology coherence signal.
func TestOntologyCoherenceReranks(t *testing.T) {
	g := newFakeGraph()
	person := "o.semsource.git.s.author.p"
	a1 := "o.semsource.golang.s.function.a1"
	a2 := "o.semsource.golang.s.function.a2"
	g.add(person, "alphaAuthor", "author", "", 0, 0, cco.Person, "")
	g.add(a1, "alphaOne", "function", "x.go", 1, 2, cco.Algorithm, "")
	g.add(a2, "alphaTwo", "function", "y.go", 1, 2, cco.Algorithm, "")
	// Resolve puts the Person outlier FIRST.
	g.resolve["alpha"] = []string{person, a1, a2}

	resp := fuse(t, g, Request{Query: "alpha"})
	if resp.Nodes[0].Name == "alphaAuthor" {
		t.Fatalf("ontology coherence should demote the class outlier from first; order=%v", nodeNames(resp.Nodes))
	}
}

func TestBudgetTruncates(t *testing.T) {
	g := newFakeGraph()
	ids := []string{}
	for _, n := range []string{"QueryA", "QueryB", "QueryC"} {
		id := "o.semsource.golang.s.function." + n
		g.add(id, n, "function", "a.go", 1, 1, cco.Algorithm, n+"-body")
		ids = append(ids, id)
	}
	g.resolve["Query"] = ids

	resp := fuse(t, g, Request{Query: "Query", Budget: Budget{MaxNodes: 2}})
	if len(resp.Nodes) != 2 || !resp.Truncated {
		t.Fatalf("expected 2 nodes + truncated, got %d truncated=%v", len(resp.Nodes), resp.Truncated)
	}
}

func TestPaths(t *testing.T) {
	g := newFakeGraph()
	a := "o.semsource.golang.s.function.a"
	b := "o.semsource.golang.s.function.b"
	c := "o.semsource.golang.s.function.c"
	g.add(a, "A", "function", "a.go", 1, 1, cco.Algorithm, "", b)
	g.add(b, "B", "function", "b.go", 1, 1, cco.Algorithm, "", c)
	g.add(c, "C", "function", "c.go", 1, 1, cco.Algorithm, "")
	g.resolve["A"] = []string{a}

	resp := fuse(t, g, Request{Query: "A", Want: []Want{WantPaths}})
	if !hasPath(resp.Paths, []string{"A", "B", "C"}) {
		t.Fatalf("expected path A→B→C, got %v", resp.Paths)
	}
}

func TestImpact(t *testing.T) {
	g := newFakeGraph()
	target := "o.semsource.golang.s.function.t"
	mid := "o.semsource.golang.s.function.m"
	top := "o.semsource.golang.s.function.tp"
	g.add(target, "Target", "function", "t.go", 1, 1, cco.Algorithm, "")
	g.add(mid, "Mid", "function", "m.go", 1, 1, cco.Algorithm, "", target)
	g.add(top, "Top", "function", "p.go", 1, 1, cco.Algorithm, "", mid)
	g.resolve["Target"] = []string{target}

	resp := fuse(t, g, Request{Query: "Target", Want: []Want{WantImpact}})
	if resp.Impact == nil || resp.Impact.Nodes != 2 {
		t.Fatalf("expected 2 affected nodes (Mid, Top), got %+v", resp.Impact)
	}
}

// TestBackendFailureSurfacesNotMiss guards the ready≠not-found contract under
// failure: a transient backend error fetching seeds that existed at resolve time
// must surface, never become a confident "not found".
func TestBackendFailureSurfacesNotMiss(t *testing.T) {
	g := newFakeGraph()
	id := "o.semsource.golang.s.function.f"
	g.add(id, "Foo", "function", "a.go", 1, 1, cco.Algorithm, "x")
	g.resolve["Foo"] = []string{id}
	g.entitiesErr = errors.New("nats timeout")

	if _, err := NewEngine(g).Fuse(context.Background(), Request{Query: "Foo"}, testLens{}); err == nil {
		t.Fatal("backend failure fetching seeds must surface as an error, not a clean miss")
	}
}

func TestProvenanceSemantic(t *testing.T) {
	g := newFakeGraph()
	id := "o.semsource.golang.s.function.f"
	g.add(id, "Foo", "function", "a.go", 1, 1, cco.Algorithm, "x")
	g.resolve["where is foo"] = []string{id}

	resp := fuse(t, g, Request{Query: "where is foo"})
	if resp.Provenance != ProvenanceEmbedding {
		t.Fatalf("NL (semantic) resolve should be embedding provenance, got %q", resp.Provenance)
	}
}

// --- helpers ------------------------------------------------------------------

func nodeNames(nodes []Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Name
	}
	return out
}

func hasPath(paths [][]string, want []string) bool {
	for _, p := range paths {
		if strings.Join(p, ">") == strings.Join(want, ">") {
			return true
		}
	}
	return false
}
