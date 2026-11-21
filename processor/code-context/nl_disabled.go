package codecontext

import (
	"context"
	"errors"

	"github.com/c360studio/semengine/component"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"

	"github.com/c360studio/semsource/entityid"
)

// retrievalClient decorates the substrate client for this component: nl
// resolves are refused when the composition runs no search index, and
// file-path queries are answered by pathSeedClient.
func retrievalClient(g fusion.RetrievalClient, config Config, deps component.Dependencies) fusion.RetrievalClient {
	if !config.semanticEnabled() {
		g = nlDisabledClient{g}
	}
	return pathSeedClient{
		RetrievalClient: g,
		lens:            config.Lens,
		authority:       entityid.Authority{Org: deps.Platform.Org, Platform: deps.Platform.Platform},
		systems:         append([]string(nil), config.SourceSystems...),
	}
}

// errNLNotEnabled is the permanent refusal for a natural-language resolve in a
// composition that runs no search index (graph.lexical_search false).
var errNLNotEnabled = errs.ClassifiedCode(errs.ErrorInvalid, semgraph.ErrorCodeNotEnabled,
	errors.New("natural-language search is not enabled in this composition"))

// nlDisabledClient refuses every natural-language resolve as not_enabled and
// passes symbol and prefix resolves through (#291). Without a search index an
// nl resolve goes to graph.lexical.query.search, which nothing answers, and the
// substrate reports that as a transient outage: a caller would retry a request
// this composition can never answer. Refusing at the resolve covers every verb
// and both lenses, including doc_context, which resolves its query in nl mode.
type nlDisabledClient struct {
	fusion.RetrievalClient
}

// Resolve refuses nl mode and delegates every other mode.
func (c nlDisabledClient) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	if q.Mode == fusion.ResolveModeNL {
		return fusion.Resolution{}, errNLNotEnabled
	}
	return c.RetrievalClient.Resolve(ctx, q)
}
