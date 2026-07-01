// Package impact is the semsource-local impact facet over pkg/fusion, kept
// because ADR-062 deliberately deferred Paths/Impact from the framework engine
// (see docs/upstream/semstreams-asks.md). It preserves code-context's "impact"
// verb — the transitive reverse-relation closure a change to the seeds could
// affect — by walking fusion.RetrievalClient.Neighbors, so nothing in the
// framework has to know about it. When the framework lands the facet upstream
// this package collapses into it.
package impact

import (
	"context"

	"github.com/c360studio/semstreams/pkg/fusion"
)

const (
	// resolveLimit bounds the seed set (matches the engine's own resolveLimit).
	resolveLimit = 40
	// maxImpactNodes caps the reverse walk so a hub entity cannot fan out
	// unbounded; past it the counts are a floor and Truncated is set.
	maxImpactNodes = 2000
)

// Summary counts the distinct entities and files reachable by walking the lens's
// edges in reverse from the query's seeds — what a change to the seeds could
// affect. It mirrors the shape source/fusion returned before the pkg/fusion port.
type Summary struct {
	Files     int  `json:"files"`
	Nodes     int  `json:"nodes"`
	Truncated bool `json:"truncated"`
}

// Compute resolves query through lens to seeds, then BFS-walks incoming edges to
// the transitive set that points at them, summarizing the distinct node and file
// counts. It returns nil (no impact facet) when the lens declares no edges — a
// domain with no relationships has no reverse closure to report. A Resolve
// backend failure is surfaced; per-node Neighbors/Entity faults degrade (that
// node just does not extend the closure), matching the engine's best-effort walk.
//
// The walk is level-by-level: each frontier's newly-discovered nodes are
// batch-fetched with Entities (one round-trip per BFS level for the file paths,
// not one per node), so the NATS cost is O(depth) fetches rather than O(nodes).
func Compute(ctx context.Context, client fusion.RetrievalClient, lens fusion.Lens, query string) (*Summary, error) {
	preds := edgePredicates(lens.Edges())
	if len(preds) == 0 {
		return nil, nil
	}
	seeds, err := client.Resolve(ctx, query, lens.ResolveMode(query), resolveLimit)
	if err != nil {
		return nil, err
	}

	visited := map[string]bool{}
	files := map[string]bool{}
	frontier := append([]string(nil), seeds...)
	truncated := false
	for len(frontier) > 0 && !truncated {
		// Expand the frontier: collect this level's newly-discovered sources.
		var next []string
		for _, id := range frontier {
			edges, _ := client.Neighbors(ctx, id, preds, fusion.Incoming)
			for _, ed := range edges {
				if visited[ed.Target] {
					continue
				}
				visited[ed.Target] = true
				next = append(next, ed.Target)
				if len(visited) >= maxImpactNodes {
					truncated = true // counts are a floor from here on
					break
				}
			}
			if truncated {
				break
			}
		}
		// One batched fetch for the whole level, for the file count.
		if len(next) > 0 {
			ents, _ := client.Entities(ctx, next)
			for _, te := range ents {
				if loc := lens.Location(te); loc.Path != "" {
					files[loc.Path] = true
				}
			}
		}
		frontier = next
	}
	return &Summary{Files: len(files), Nodes: len(visited), Truncated: truncated}, nil
}

// edgePredicates flattens a lens's EdgeSpecs into the predicate list the reverse
// walk filters on (pkg/fusion keeps its own edgePredicates unexported).
func edgePredicates(specs []fusion.EdgeSpec) []string {
	preds := make([]string, 0, len(specs))
	for _, s := range specs {
		preds = append(preds, s.Predicate)
	}
	return preds
}
