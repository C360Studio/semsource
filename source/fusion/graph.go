package fusion

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/message"
)

// ResolveMode selects how a raw query becomes seed entity IDs.
type ResolveMode int

// The resolution modes, each backed by a different graph.query.* surface.
const (
	ResolveSemantic ResolveMode = iota // NL → graph.query.semantic (embeddings)
	ResolveSymbol                      // identifier → name/alias index
	ResolvePrefix                      // path/prefix → graph.query.prefix
)

// Direction selects edge traversal direction.
type Direction int

// Outgoing follows a subject's predicates to targets; Incoming follows the
// reverse (who points at this entity).
const (
	Outgoing Direction = iota
	Incoming
)

// Edge is a relationship from a subject entity to a target entity.
type Edge struct {
	Predicate string
	Target    string
}

// Entity is a graph entity with its triples. The accessors read triple objects
// without the caller touching the triple shape.
type Entity struct {
	ID      string
	Triples []message.Triple
}

// First returns the first object for predicate rendered as a string, or "".
func (e *Entity) First(predicate string) string {
	for i := range e.Triples {
		if e.Triples[i].Predicate == predicate {
			return objectString(e.Triples[i].Object)
		}
	}
	return ""
}

// FirstInt returns the first object for predicate as an int, or 0.
func (e *Entity) FirstInt(predicate string) int {
	for i := range e.Triples {
		if e.Triples[i].Predicate == predicate {
			switch v := e.Triples[i].Object.(type) {
			case int:
				return v
			case int64:
				return int(v)
			case float64:
				return int(v)
			}
		}
	}
	return 0
}

// objectString renders a triple object as a string (objects are string IRIs/
// literals, or numeric literals).
func objectString(o any) string {
	switch v := o.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// GraphQueryClient is the slice of graph.query.* the fusion engine composes
// over. Production wraps NATS request/reply (graph.query.{status,semantic,
// prefix,relationships,entity}); tests use an in-memory fake. Keeping the engine
// behind this interface is what lets it stay deterministic and unit-testable
// without a live graph.
type GraphQueryClient interface {
	// Status reports graph readiness (graph.query.status).
	Status(ctx context.Context) (IndexStatus, error)
	// Resolve maps a query to seed entity IDs by mode, most relevant first.
	Resolve(ctx context.Context, query string, mode ResolveMode, limit int) ([]string, error)
	// Entity returns an entity by ID, or (nil, nil) if absent.
	Entity(ctx context.Context, id string) (*Entity, error)
	// Entities batch-fetches entities by ID (maps to graph.query.batch). Absent
	// IDs are omitted from the result; a non-nil error means a backend failure,
	// which callers must distinguish from genuine absence.
	Entities(ctx context.Context, ids []string) ([]*Entity, error)
	// Neighbors returns edges from id along the given predicates in a direction.
	Neighbors(ctx context.Context, id string, predicates []string, dir Direction) ([]Edge, error)
	// Names suggests entity display names near a query (for did_you_mean).
	Names(ctx context.Context, query string, limit int) ([]string, error)
}
