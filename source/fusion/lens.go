package fusion

import "context"

// EdgeSpec declares one relationship predicate the engine should expand around a
// seed, with the role labels for its forward and (optional) reverse directions.
// For code: {Predicate: "code.relationship.calls", OutgoingRole: "callee",
// IncomingRole: "caller"}.
type EdgeSpec struct {
	Predicate    string
	OutgoingRole string
	IncomingRole string // "" to skip the reverse direction
}

// Lens supplies the only domain-specific parts of fusion. The engine owns
// resolve / expand / rank / budget / envelope; the lens declares which edges to
// walk, how to read an entity's human-facing fields, and how to hydrate its
// verbatim body. One engine, many lenses (code, docs, …) — the proof that fusion
// is a general primitive, not code in disguise (ADR-0004).
type Lens interface {
	// Name identifies the lens (e.g. "code", "docs").
	Name() string
	// ResolveMode picks how to turn the raw query into seeds.
	ResolveMode(query string) ResolveMode
	// Edges are the relationship predicates to expand around each seed.
	Edges() []EdgeSpec
	// Label is the entity's human name (e.g. from dc.terms.title).
	Label(e *Entity) string
	// Kind is a short human kind for the entity (e.g. "function", "doc").
	Kind(e *Entity) string
	// Location returns the entity's domain-general place (file path or URL,
	// optional section fragment, optional line range).
	Location(e *Entity) Locator
	// Hydrate returns the entity's verbatim body (source/passage), or "".
	Hydrate(ctx context.Context, e *Entity) (string, error)
}

// edgePredicates flattens a lens's EdgeSpecs into the predicate list and the
// forward/reverse role lookups used during expansion.
func edgePredicates(specs []EdgeSpec) (preds []string, fwd, rev map[string]string) {
	fwd = make(map[string]string, len(specs))
	rev = make(map[string]string, len(specs))
	for _, s := range specs {
		preds = append(preds, s.Predicate)
		if s.OutgoingRole != "" {
			fwd[s.Predicate] = s.OutgoingRole
		}
		if s.IncomingRole != "" {
			rev[s.Predicate] = s.IncomingRole
		}
	}
	return preds, fwd, rev
}
