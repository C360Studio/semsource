package codecontext

import (
	"context"
	"strings"

	"github.com/c360studio/semstreams/pkg/fusion"

	"github.com/c360studio/semsource/source/ast"
)

// exactSeedClient tightens symbol-mode seed resolution to byte-exact display
// names (go-callgraph-recall D4). The framework NAME_INDEX is case-folded by
// design — recall is case-insensitive and exact case is a ranking signal — but
// for code answers a case lookalike is a different symbol: the audit found
// unexported `systemSlug` helpers entering the `SystemSlug` impact closure.
// Exactness is product policy, so it lives at the product seam: this decorator
// wraps the RetrievalClient the component already constructs, filters
// ResolveModeSymbol results to entities whose display name (DcTitle — the bare
// identifier for functions, methods, and types alike) equals the trimmed query
// byte-for-byte, and passes NL and prefix modes through untouched. Zero
// survivors feed the engine's normal ready+absent miss path (did_you_mean) —
// the honest answer for a case-sloppy query, never a lookalike closure.
type exactSeedClient struct {
	fusion.RetrievalClient
}

// Resolve filters symbol-mode seed IDs to byte-exact display-name matches,
// preserving the underlying resolve order AND each seed's own relevance score.
//
// Seeds are carried through whole rather than reduced to IDs and rebuilt: since
// beta.157 a Seed carries Similarity/HasSimilarity, and projecting to IDs here
// would discard the resolve mode's own scoring before the engine ever saw it.
func (c exactSeedClient) Resolve(ctx context.Context, q fusion.ResolveQuery) ([]fusion.Seed, error) {
	seeds, err := c.RetrievalClient.Resolve(ctx, q)
	if err != nil || q.Mode != fusion.ResolveModeSymbol || len(seeds) == 0 {
		return seeds, err
	}
	hydration, err := c.RetrievalClient.Entities(ctx, fusion.SeedIDs(seeds))
	if err != nil {
		// A backend failure must stay a failure (ready ≠ not-found): silently
		// returning the unfiltered seeds would defeat the filter exactly when
		// the graph is degraded.
		return nil, err
	}
	want := strings.TrimSpace(q.Query)
	keep := make(map[string]bool, len(hydration.Entities))
	for _, e := range hydration.Entities {
		if e.First(ast.DcTitle) == want {
			keep[e.ID] = true
		}
	}
	// A seed that did not hydrate is dropped, because its display name could not
	// be read and this filter's entire claim is byte-exactness — asserting it for
	// an entity we never saw would admit exactly the lookalike this exists to
	// prevent. Since beta.157 that drop is at least KNOWABLE: hydration.Unhydrated
	// names every such ID with a reason (gh#597), where before it was
	// indistinguishable from a shorter slice.
	exact := make([]fusion.Seed, 0, len(seeds))
	for _, s := range seeds {
		if keep[s.ID] {
			exact = append(exact, s)
		}
	}
	return exact, nil
}
