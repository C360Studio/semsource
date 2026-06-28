// Package natsgraph implements fusion.GraphQueryClient over the semstreams
// graph.query.* / graph.index.query.* NATS subjects. It is the production
// backing for the fusion engine; tests use fusiontest.MemGraph instead.
package natsgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"

	"github.com/c360studio/semsource/source/fusion"
)

// defaultTimeout bounds each graph request/reply.
const defaultTimeout = 5 * time.Second

// Client wraps a NATS client to satisfy fusion.GraphQueryClient.
type Client struct {
	nats    *natsclient.Client
	timeout time.Duration
}

// New creates a graph client over the given NATS client.
func New(nats *natsclient.Client) *Client {
	return &Client{nats: nats, timeout: defaultTimeout}
}

// request marshals req, calls subject, and unmarshals the reply into out.
func (c *Client) request(ctx context.Context, subject string, req any, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", subject, err)
	}
	resp, err := c.nats.RequestClassified(ctx, subject, body, c.timeout)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp, out); err != nil {
		return fmt.Errorf("unmarshal %s reply: %w", subject, err)
	}
	return nil
}

// entityState mirrors graph.EntityState's wire shape (only the fields fusion needs).
type entityState struct {
	ID      string           `json:"id"`
	Triples []message.Triple `json:"triples"`
}

func (e entityState) toEntity() *fusion.Entity {
	return &fusion.Entity{ID: e.ID, Triples: e.Triples}
}

// Status maps graph.query.status's phase to a readiness envelope.
func (c *Client) Status(ctx context.Context) (fusion.IndexStatus, error) {
	var resp struct {
		Phase string `json:"phase"`
	}
	if err := c.request(ctx, "graph.query.status", struct{}{}, &resp); err != nil {
		return fusion.IndexStatus{}, err
	}
	switch resp.Phase {
	case "ready":
		return fusion.IndexStatus{Ready: true, State: fusion.StateReady}, nil
	case "degraded":
		return fusion.IndexStatus{Ready: false, State: fusion.StateDegraded}, nil
	default: // "seeding" or unknown → not ready
		return fusion.IndexStatus{Ready: false, State: fusion.StateBuilding}, nil
	}
}

// Resolve maps a query to seed entity IDs by mode.
func (c *Client) Resolve(ctx context.Context, query string, mode fusion.ResolveMode, limit int) ([]string, error) {
	switch mode {
	case fusion.ResolvePrefix:
		return c.resolvePrefix(ctx, query, limit)
	case fusion.ResolveSymbol:
		return c.resolveSymbol(ctx, query, limit)
	default:
		return c.resolveSemantic(ctx, query, limit)
	}
}

// resolveSemantic queries the embedding-backed semantic search.
func (c *Client) resolveSemantic(ctx context.Context, query string, limit int) ([]string, error) {
	var resp struct {
		Results []struct {
			EntityID string `json:"entity_id"`
		} `json:"results"`
	}
	req := map[string]any{"query": query, "limit": limit}
	if err := c.request(ctx, "graph.query.semantic", req, &resp); err != nil {
		return nil, err
	}
	ids := make([]string, len(resp.Results))
	for i, r := range resp.Results {
		ids[i] = r.EntityID
	}
	return ids, nil
}

// resolvePrefix lists entities under a path prefix (full entities returned;
// only their IDs are taken here — the engine's caching client batch-fetches
// bodies, which collapses to a cache hit per the prefix warm-up if added later).
func (c *Client) resolvePrefix(ctx context.Context, prefix string, limit int) ([]string, error) {
	var resp struct {
		Entities []entityState `json:"entities"`
	}
	req := map[string]any{"prefix": prefix, "limit": limit}
	if err := c.request(ctx, "graph.query.prefix", req, &resp); err != nil {
		return nil, err
	}
	ids := make([]string, len(resp.Entities))
	for i, e := range resp.Entities {
		ids[i] = e.ID
	}
	return ids, nil
}

// resolveSymbol resolves a name via the alias index, falling back to semantic
// search when the name is not a registered alias. NOTE: there is no dedicated
// name→ranked-IDs index in graph.query.* today (tracked upstream); until there
// is, an un-aliased symbol resolves through semantic search.
func (c *Client) resolveSymbol(ctx context.Context, name string, limit int) ([]string, error) {
	var alias struct {
		ID string `json:"id"`
	}
	err := c.request(ctx, "graph.query.entityByAlias", map[string]any{"aliasOrID": name}, &alias)
	if err == nil && alias.ID != "" {
		return []string{alias.ID}, nil
	}
	return c.resolveSemantic(ctx, name, limit)
}

// Entity returns one entity, via the batch path so a missing ID is a clean
// absence (nil, nil) rather than a not-found error to classify.
func (c *Client) Entity(ctx context.Context, id string) (*fusion.Entity, error) {
	ents, err := c.Entities(ctx, []string{id})
	if err != nil || len(ents) == 0 {
		return nil, err
	}
	return ents[0], nil
}

// Entities batch-fetches entities; missing IDs are omitted by the server.
func (c *Client) Entities(ctx context.Context, ids []string) ([]*fusion.Entity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var resp struct {
		Entities []entityState `json:"entities"`
	}
	if err := c.request(ctx, "graph.query.batch", map[string]any{"ids": ids}, &resp); err != nil {
		return nil, err
	}
	out := make([]*fusion.Entity, 0, len(resp.Entities))
	for _, e := range resp.Entities {
		out = append(out, e.toEntity())
	}
	return out, nil
}

// Neighbors returns edges from id in a direction, filtered to predicates.
// Incoming returns the source (FromEntityID) as Edge.Target; outgoing the target.
//
// It uses the lower-level graph.index.query.{outgoing,incoming} subjects on
// purpose, NOT the graph.query.relationships facade: the facade renames the
// predicate field to "edge_type" and flattens the envelope, which would make the
// predicate filter below match nothing. Do not "simplify" onto that subject.
func (c *Client) Neighbors(ctx context.Context, id string, preds []string, dir fusion.Direction) ([]fusion.Edge, error) {
	want := make(map[string]bool, len(preds))
	for _, p := range preds {
		want[p] = true
	}
	if dir == fusion.Outgoing {
		var resp struct {
			Data struct {
				Relationships []struct {
					ToEntityID string `json:"to_entity_id"`
					Predicate  string `json:"predicate"`
				} `json:"relationships"`
			} `json:"data"`
		}
		if err := c.request(ctx, "graph.index.query.outgoing", map[string]any{"entity_id": id}, &resp); err != nil {
			return nil, err
		}
		var edges []fusion.Edge
		for _, r := range resp.Data.Relationships {
			if want[r.Predicate] {
				edges = append(edges, fusion.Edge{Predicate: r.Predicate, Target: r.ToEntityID})
			}
		}
		return edges, nil
	}
	var resp struct {
		Data struct {
			Relationships []struct {
				FromEntityID string `json:"from_entity_id"`
				Predicate    string `json:"predicate"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := c.request(ctx, "graph.index.query.incoming", map[string]any{"entity_id": id}, &resp); err != nil {
		return nil, err
	}
	var edges []fusion.Edge
	for _, r := range resp.Data.Relationships {
		if want[r.Predicate] {
			edges = append(edges, fusion.Edge{Predicate: r.Predicate, Target: r.FromEntityID})
		}
	}
	return edges, nil
}

// Names synthesizes did_you_mean suggestions: semantic search for nearby
// entities, then their titles. There is no dedicated name-suggestion subject.
func (c *Client) Names(ctx context.Context, query string, limit int) ([]string, error) {
	ids, err := c.resolveSemantic(ctx, query, limit)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	ents, err := c.Entities(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if t := e.First("dc.terms.title"); t != "" {
			names = append(names, t)
		}
	}
	return names, nil
}

// compile-time check that Client satisfies the interface.
var _ fusion.GraphQueryClient = (*Client)(nil)
