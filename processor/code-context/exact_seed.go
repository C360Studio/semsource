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
// preserving the underlying resolve order.
func (c exactSeedClient) Resolve(ctx context.Context, q fusion.ResolveQuery) ([]string, error) {
	ids, err := c.RetrievalClient.Resolve(ctx, q)
	if err != nil || q.Mode != fusion.ResolveModeSymbol || len(ids) == 0 {
		return ids, err
	}
	entities, err := c.RetrievalClient.Entities(ctx, ids)
	if err != nil {
		// A backend failure must stay a failure (ready ≠ not-found): silently
		// returning the unfiltered ids would defeat the filter exactly when
		// the graph is degraded.
		return nil, err
	}
	want := strings.TrimSpace(q.Query)
	keep := make(map[string]bool, len(entities))
	for _, e := range entities {
		if e.First(ast.DcTitle) == want {
			keep[e.ID] = true
		}
	}
	exact := make([]string, 0, len(ids))
	for _, id := range ids {
		if keep[id] {
			exact = append(exact, id)
		}
	}
	return exact, nil
}
