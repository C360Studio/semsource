package impact

import (
	"context"
	"strconv"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"

	"github.com/c360studio/semsource/source/fusion/fusiontest"
)

// testLens is a minimal fusion.Lens with a single reversible edge ("calls"),
// enough to drive the impact walk without coupling to a real domain lens.
type testLens struct{ edges []fusion.EdgeSpec }

func (testLens) Name() string                          { return "test" }
func (testLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModeSymbol }
func (l testLens) Edges() []fusion.EdgeSpec            { return l.edges }
func (testLens) Label(e *fusion.Entity) string         { return e.First("dc.terms.title") }
func (testLens) Kind(*fusion.Entity) string            { return "" }
func (testLens) Location(e *fusion.Entity) fusion.Locator {
	return fusion.Locator{Path: e.First("path")}
}
func (testLens) Hydrate(context.Context, *fusion.Entity) (*message.StorageReference, error) {
	return nil, nil
}

const callsPred = "calls"

func callsLens() testLens {
	return testLens{edges: []fusion.EdgeSpec{{Predicate: callsPred, OutgoingRole: "callee", IncomingRole: "caller"}}}
}

// TestCompute_ReverseClosure: two callers of the seed, one of them itself called
// by a third — the closure is all three, across their distinct files.
func TestCompute_ReverseClosure(t *testing.T) {
	g := fusiontest.NewMemGraph()
	g.AddEntity("seed", map[string]any{"path": "seed.go"})
	g.AddEntity("a", map[string]any{"path": "a.go"})
	g.AddEntity("b", map[string]any{"path": "b.go"})
	g.AddEntity("c", map[string]any{"path": "a.go"}) // shares a.go with a
	g.AddEdge("a", callsPred, "seed")
	g.AddEdge("b", callsPred, "seed")
	g.AddEdge("c", callsPred, "a") // c → a → seed (transitive)
	g.SetResolve("seed", "seed")

	sum, err := Compute(context.Background(), g, callsLens(), "seed")
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil || sum.Nodes != 3 {
		t.Fatalf("expected 3 reverse-reachable nodes (a,b,c), got %+v", sum)
	}
	if sum.Files != 2 { // a.go (a+c) and b.go
		t.Fatalf("expected 2 distinct files, got %d", sum.Files)
	}
	if sum.Truncated {
		t.Error("small closure should not be truncated")
	}
}

// TestCompute_NoEdges: a lens with no edges has no reverse closure → nil summary.
func TestCompute_NoEdges(t *testing.T) {
	g := fusiontest.NewMemGraph()
	g.SetResolve("seed", "seed")
	sum, err := Compute(context.Background(), g, testLens{}, "seed")
	if err != nil || sum != nil {
		t.Fatalf("no-edges lens should yield (nil,nil), got (%+v,%v)", sum, err)
	}
}

// TestCompute_Truncated: past maxImpactNodes callers, the walk stops and reports
// a floor with Truncated set.
func TestCompute_Truncated(t *testing.T) {
	g := fusiontest.NewMemGraph()
	g.AddEntity("seed", map[string]any{"path": "seed.go"})
	for i := 0; i < maxImpactNodes+50; i++ {
		id := "caller" + strconv.Itoa(i)
		g.AddEntity(id, map[string]any{"path": id + ".go"})
		g.AddEdge(id, callsPred, "seed")
	}
	g.SetResolve("seed", "seed")

	sum, err := Compute(context.Background(), g, callsLens(), "seed")
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil || !sum.Truncated {
		t.Fatalf("expected Truncated past the node cap, got %+v", sum)
	}
	if sum.Nodes < maxImpactNodes {
		t.Fatalf("truncated count should be a floor at the cap, got %d", sum.Nodes)
	}
}

// TestCompute_NodeFaultTolerant: a caller with no entity record still counts as a
// visited node (it extends the closure) but contributes no file — the walk does
// not fail on the missing entity.
func TestCompute_NodeFaultTolerant(t *testing.T) {
	g := fusiontest.NewMemGraph()
	g.AddEntity("seed", map[string]any{"path": "seed.go"})
	// "ghost" points at seed via an edge but has no AddEntity record.
	g.AddEdge("ghost", callsPred, "seed")
	g.SetResolve("seed", "seed")

	sum, err := Compute(context.Background(), g, callsLens(), "seed")
	if err != nil {
		t.Fatal(err)
	}
	if sum == nil || sum.Nodes != 1 {
		t.Fatalf("ghost caller should still count as a node, got %+v", sum)
	}
	if sum.Files != 0 {
		t.Fatalf("ghost caller has no entity/path, so no file, got %d", sum.Files)
	}
}
