package fusion

import (
	"context"
	"sort"
)

// paths returns up to maxPaths name-sequences reachable from the seed by
// following the lens's edges forward. A leaf, the depth bound, or a cycle (every
// onward target already on the path) all terminate a path — a cycle is emitted
// truncated rather than dropped, so a recursive chain still yields a path.
func (eng *Engine) paths(ctx context.Context, seed *Entity, lens Lens) [][]string {
	preds, _, _ := edgePredicates(lens.Edges())
	if len(preds) == 0 {
		return nil
	}
	var out [][]string
	eng.walkPaths(ctx, seed.ID, []string{lens.Label(seed)}, map[string]bool{seed.ID: true}, preds, lens, &out)
	return out
}

// walkPaths is the bounded DFS behind paths.
func (eng *Engine) walkPaths(ctx context.Context, id string, trail []string, onPath map[string]bool, preds []string, lens Lens, out *[][]string) {
	if len(*out) >= maxPaths {
		return
	}
	edges, _ := eng.graph.Neighbors(ctx, id, preds, Outgoing)
	next := uniqueTargets(edges)
	if len(next) == 0 || len(trail) >= maxPathDepth {
		*out = append(*out, append([]string(nil), trail...))
		return
	}
	recursed := false
	for _, tid := range next {
		if onPath[tid] {
			continue
		}
		te, err := eng.graph.Entity(ctx, tid)
		if err != nil || te == nil {
			continue
		}
		recursed = true
		onPath[tid] = true
		eng.walkPaths(ctx, tid, append(trail, lens.Label(te)), onPath, preds, lens, out)
		onPath[tid] = false
		if len(*out) >= maxPaths {
			return
		}
	}
	if !recursed {
		*out = append(*out, append([]string(nil), trail...))
	}
}

// impact computes the transitive set of entities that (directly or indirectly)
// point at the seeds along the lens's edges — what a change to the seeds could
// affect — and summarizes the count of distinct nodes and files.
func (eng *Engine) impact(ctx context.Context, seeds []*Entity, lens Lens) *Impact {
	preds, _, _ := edgePredicates(lens.Edges())
	if len(preds) == 0 {
		return nil
	}
	visited := map[string]bool{}
	files := map[string]bool{}
	var queue []string
	for _, s := range seeds {
		queue = append(queue, s.ID)
	}
	truncated := false
	for len(queue) > 0 {
		if len(visited) >= maxImpactNodes {
			truncated = true // counts are a floor from here on
			break
		}
		id := queue[0]
		queue = queue[1:]
		edges, _ := eng.graph.Neighbors(ctx, id, preds, Incoming)
		for _, ed := range edges {
			src := ed.Target
			if visited[src] {
				continue
			}
			visited[src] = true
			if te, err := eng.graph.Entity(ctx, src); err == nil && te != nil {
				if loc := lens.Location(te); loc.Path != "" {
					files[loc.Path] = true
				}
			}
			queue = append(queue, src)
		}
	}
	return &Impact{Files: len(files), Nodes: len(visited), Truncated: truncated}
}

// uniqueTargets returns the distinct edge targets in a deterministic order.
func uniqueTargets(edges []Edge) []string {
	seen := make(map[string]struct{}, len(edges))
	var out []string
	for _, e := range edges {
		if _, ok := seen[e.Target]; ok {
			continue
		}
		seen[e.Target] = struct{}{}
		out = append(out, e.Target)
	}
	sort.Strings(out)
	return out
}
