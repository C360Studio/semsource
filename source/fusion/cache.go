package fusion

import "context"

// cachingClient memoizes entity fetches for the duration of one Fuse call and
// batches misses through Entities. The same entity is touched many times in a
// single fusion (as a seed, a neighbor, a path node, an impact node); without
// this the engine would issue thousands of redundant round-trips. It is used by
// a single goroutine per Fuse, so it needs no locking. The embedded
// GraphQueryClient supplies Status/Resolve/Neighbors/Names unchanged.
type cachingClient struct {
	GraphQueryClient
	seen map[string]*Entity // nil value = known-absent
}

func newCachingClient(inner GraphQueryClient) *cachingClient {
	return &cachingClient{GraphQueryClient: inner, seen: map[string]*Entity{}}
}

// Entity returns a memoized entity, fetching on a miss.
func (c *cachingClient) Entity(ctx context.Context, id string) (*Entity, error) {
	if e, ok := c.seen[id]; ok {
		return e, nil
	}
	e, err := c.GraphQueryClient.Entity(ctx, id)
	if err != nil {
		return nil, err
	}
	c.seen[id] = e
	return e, nil
}

// Entities batch-fetches only the uncached IDs, memoizing results (and absences)
// so repeats are free.
func (c *cachingClient) Entities(ctx context.Context, ids []string) ([]*Entity, error) {
	var missing []string
	for _, id := range ids {
		if _, ok := c.seen[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		fetched, err := c.GraphQueryClient.Entities(ctx, missing)
		if err != nil {
			return nil, err
		}
		for _, e := range fetched {
			if e != nil {
				c.seen[e.ID] = e
			}
		}
		for _, id := range missing {
			if _, ok := c.seen[id]; !ok {
				c.seen[id] = nil // record absence so we never refetch it
			}
		}
	}
	out := make([]*Entity, 0, len(ids))
	for _, id := range ids {
		if e := c.seen[id]; e != nil {
			out = append(out, e)
		}
	}
	return out, nil
}
