// Package fusiontest provides an in-memory fusion.GraphQueryClient for testing
// lenses and the engine without a live graph.
package fusiontest

import (
	"context"
	"slices"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/source/fusion"
)

// MemGraph is an in-memory fusion.GraphQueryClient. Build it with AddEntity /
// AddEdge / SetResolve, then hand it to fusion.NewEngine.
type MemGraph struct {
	status   fusion.IndexStatus
	entities map[string]*fusion.Entity
	out      map[string][]fusion.Edge // id → outgoing edges
	resolve  map[string][]string
	names    []string
}

// NewMemGraph returns a ready in-memory graph.
func NewMemGraph() *MemGraph {
	return &MemGraph{
		status:   fusion.IndexStatus{Ready: true, State: fusion.StateReady, Revision: "test"},
		entities: map[string]*fusion.Entity{},
		out:      map[string][]fusion.Edge{},
		resolve:  map[string][]string{},
	}
}

// SetStatus overrides the reported readiness.
func (m *MemGraph) SetStatus(s fusion.IndexStatus) { m.status = s }

// SetResolve registers the seed IDs a query resolves to.
func (m *MemGraph) SetResolve(query string, ids ...string) { m.resolve[query] = ids }

// titlePredicates are the predicates that may carry an entity's human name,
// across domains — used to populate did_you_mean suggestions.
var titlePredicates = []string{"dc.terms.title", "source.doc.summary", "source.web.title"}

// AddEntity registers an entity from a predicate→object property map.
func (m *MemGraph) AddEntity(id string, props map[string]any) {
	tr := make([]message.Triple, 0, len(props))
	for p, o := range props {
		tr = append(tr, message.Triple{Subject: id, Predicate: p, Object: o})
	}
	m.entities[id] = &fusion.Entity{ID: id, Triples: tr}
	for _, p := range titlePredicates {
		if t, ok := props[p].(string); ok {
			m.names = append(m.names, t)
			break
		}
	}
}

// AddEdge registers a directed predicate edge from → to.
func (m *MemGraph) AddEdge(from, predicate, to string) {
	m.out[from] = append(m.out[from], fusion.Edge{Predicate: predicate, Target: to})
}

// Status implements fusion.GraphQueryClient.
func (m *MemGraph) Status(context.Context) (fusion.IndexStatus, error) { return m.status, nil }

// Resolve implements fusion.GraphQueryClient.
func (m *MemGraph) Resolve(_ context.Context, query string, _ fusion.ResolveMode, _ int) ([]string, error) {
	return m.resolve[query], nil
}

// Entity implements fusion.GraphQueryClient.
func (m *MemGraph) Entity(_ context.Context, id string) (*fusion.Entity, error) {
	return m.entities[id], nil
}

// Entities implements fusion.GraphQueryClient.
func (m *MemGraph) Entities(_ context.Context, ids []string) ([]*fusion.Entity, error) {
	var out []*fusion.Entity
	for _, id := range ids {
		if e := m.entities[id]; e != nil {
			out = append(out, e)
		}
	}
	return out, nil
}

// Neighbors implements fusion.GraphQueryClient (Incoming returns sources as Target).
func (m *MemGraph) Neighbors(_ context.Context, id string, preds []string, dir fusion.Direction) ([]fusion.Edge, error) {
	if dir == fusion.Outgoing {
		var out []fusion.Edge
		for _, e := range m.out[id] {
			if slices.Contains(preds, e.Predicate) {
				out = append(out, e)
			}
		}
		return out, nil
	}
	var in []fusion.Edge
	for src, edges := range m.out {
		for _, e := range edges {
			if e.Target == id && slices.Contains(preds, e.Predicate) {
				in = append(in, fusion.Edge{Predicate: e.Predicate, Target: src})
			}
		}
	}
	return in, nil
}

// Names implements fusion.GraphQueryClient.
func (m *MemGraph) Names(_ context.Context, _ string, limit int) ([]string, error) {
	if len(m.names) > limit {
		return m.names[:limit], nil
	}
	return m.names, nil
}
