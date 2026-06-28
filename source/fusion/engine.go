package fusion

import "context"

// Engine runs the deterministic fusion pipeline over a GraphQueryClient.
type Engine struct {
	graph GraphQueryClient
}

// NewEngine creates a fusion engine over the given graph client.
func NewEngine(graph GraphQueryClient) *Engine {
	return &Engine{graph: graph}
}

// Bounds keep the multi-hop graph traversal cheap and predictable.
const (
	resolveLimit        = 40
	maxRelationsPerNode = 12
	maxPathDepth        = 6
	maxPaths            = 24
	maxImpactNodes      = 2000
)

// Fuse resolves req against the graph through lens and returns the fused
// response. Readiness is load-bearing: a not-ready graph yields an empty
// envelope (the caller must fall back); ready+absent yields a miss with
// near-matches — never an ambiguous empty.
func (eng *Engine) Fuse(ctx context.Context, req Request, lens Lens) (Response, error) {
	// Run over a per-request caching client so an entity touched as a seed, a
	// neighbor, a path node, and an impact node is fetched once, not four times.
	run := &Engine{graph: newCachingClient(eng.graph)}

	status, err := run.graph.Status(ctx)
	if err != nil {
		return Response{}, err
	}
	if !status.Ready {
		return Response{Index: status, Provenance: ProvenanceDeterministic, ContractVersion: ContractVersion}, nil
	}

	mode := lens.ResolveMode(req.Query)
	seeds, err := run.graph.Resolve(ctx, req.Query, mode, resolveLimit)
	if err != nil {
		return Response{}, err
	}
	// A backend failure fetching seeds is surfaced, NOT silently turned into a
	// "not found" — that would violate the ready≠not-found contract.
	entities, err := run.graph.Entities(ctx, seeds)
	if err != nil {
		return Response{}, err
	}
	if len(entities) == 0 {
		return run.miss(ctx, status, mode, req.Query), nil
	}

	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = lens.Label(e)
	}
	ranked := rankEntities(entities, names, req.Query)

	wants := wantSet(req.Want)
	resp := Response{Index: status, Provenance: provenanceFor(mode), ContractVersion: ContractVersion}
	resp.Nodes, resp.Truncated = run.buildNodes(ctx, ranked, lens, wants, newBudget(req.Budget))
	if wants[WantPaths] && len(resp.Nodes) > 0 {
		resp.Paths = run.paths(ctx, ranked[0], lens)
	}
	if wants[WantImpact] && len(resp.Nodes) > 0 {
		resp.Impact = run.impact(ctx, ranked, lens)
	}
	if len(resp.Nodes) == 0 {
		resp.Misses = []Miss{{Query: req.Query}}
	}
	return resp, nil
}

// miss builds a ready+absent response with near-matches.
func (eng *Engine) miss(ctx context.Context, status IndexStatus, mode ResolveMode, query string) Response {
	names, _ := eng.graph.Names(ctx, query, 5)
	return Response{
		Index:           status,
		Provenance:      provenanceFor(mode),
		Misses:          []Miss{{Query: query, DidYouMean: names}},
		ContractVersion: ContractVersion,
	}
}

// buildNodes materializes ranked entities into Nodes within the budget.
func (eng *Engine) buildNodes(ctx context.Context, ranked []*Entity, lens Lens, wants map[Want]bool, bg *budgeter) ([]Node, bool) {
	var nodes []Node
	for _, e := range ranked {
		node := eng.nodeFor(ctx, e, lens, wants)
		if !bg.admit(len(node.Body)) {
			return nodes, true
		}
		nodes = append(nodes, node)
	}
	return nodes, false
}

// nodeFor builds one Node, including only the requested facets.
func (eng *Engine) nodeFor(ctx context.Context, e *Entity, lens Lens, wants map[Want]bool) Node {
	loc := lens.Location(e)
	node := Node{Name: lens.Label(e), Kind: lens.Kind(e), Path: loc.Path, Fragment: loc.Fragment, Lines: lineRange(loc.Lines), Class: classOf(e), Handle: e.ID}
	if wants[WantBody] {
		if body, err := lens.Hydrate(ctx, e); err == nil {
			node.Body = body
		}
	}
	if wants[WantRelations] {
		node.Relations = eng.relations(ctx, e, lens)
	}
	return node
}

// relations expands a node's forward and reverse edges into role → refs.
func (eng *Engine) relations(ctx context.Context, e *Entity, lens Lens) map[string][]Ref {
	preds, fwd, rev := edgePredicates(lens.Edges())
	if len(preds) == 0 {
		return nil
	}
	rels := map[string][]Ref{}
	eng.collectEdges(ctx, e.ID, preds, Outgoing, fwd, lens, rels)
	eng.collectEdges(ctx, e.ID, preds, Incoming, rev, lens, rels)
	if len(rels) == 0 {
		return nil
	}
	return rels
}

// collectEdges appends refs for one direction's edges, capped per role.
func (eng *Engine) collectEdges(ctx context.Context, id string, preds []string, dir Direction, roleByPred map[string]string, lens Lens, rels map[string][]Ref) {
	edges, err := eng.graph.Neighbors(ctx, id, preds, dir)
	if err != nil {
		return
	}
	for _, ed := range edges {
		role := roleByPred[ed.Predicate]
		if role == "" || len(rels[role]) >= maxRelationsPerNode {
			continue
		}
		if ref, ok := eng.refFor(ctx, ed.Target, lens); ok {
			rels[role] = append(rels[role], ref)
		}
	}
}

// refFor resolves a target entity ID to a human Ref.
func (eng *Engine) refFor(ctx context.Context, id string, lens Lens) (Ref, bool) {
	te, err := eng.graph.Entity(ctx, id)
	if err != nil || te == nil {
		return Ref{}, false
	}
	loc := lens.Location(te)
	return Ref{Name: lens.Label(te), Path: loc.Path, Fragment: loc.Fragment, Line: loc.Lines[0]}, true
}

// lineRange renders a [start,end] pair as a wire slice, or nil for line-less
// domains so the field is omitted.
func lineRange(l [2]int) []int {
	if l[0] == 0 && l[1] == 0 {
		return nil
	}
	return []int{l[0], l[1]}
}

// provenanceFor reports how seeds were resolved: semantic uses embeddings, the
// rest are deterministic lookups.
func provenanceFor(mode ResolveMode) Provenance {
	if mode == ResolveSemantic {
		return ProvenanceEmbedding
	}
	return ProvenanceDeterministic
}

// budgeter accumulates nodes up to the request budget, reporting truncation.
type budgeter struct {
	maxNodes, maxBytes, nodes, bytes int
}

// newBudget builds a budgeter from a request budget, applying defaults.
func newBudget(b Budget) *budgeter {
	bg := &budgeter{maxNodes: b.MaxNodes, maxBytes: b.MaxBytes}
	if bg.maxNodes <= 0 {
		bg.maxNodes = defaultMaxNodes
	}
	if bg.maxBytes <= 0 {
		bg.maxBytes = defaultMaxBytes
	}
	return bg
}

// admit reports whether a node carrying bodyBytes still fits, updating totals.
// At least one node is always admitted so a single oversized node is not dropped.
func (b *budgeter) admit(bodyBytes int) bool {
	if b.nodes >= b.maxNodes {
		return false
	}
	if b.nodes > 0 && b.bytes+bodyBytes > b.maxBytes {
		return false
	}
	b.nodes++
	b.bytes += bodyBytes
	return true
}
