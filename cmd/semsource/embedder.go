package main

import (
	"errors"
	"fmt"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/model"
	graphembedding "github.com/c360studio/semengine/processor/graph-embedding"
)

// embedderType names the tier 1 embedder. graph-embedding makes it part of the
// dedup key, so changing it re-embeds every entity.
const embedderType = "http"

// registerGraphEmbedding registers graph-embedding with the tier 1 HTTP
// embedder. It is registered whatever the tier: graph-embedding is composed
// only when graph.semantic_search is on, and the embedder is built at Start,
// never at registration, so a tier 0 boot never resolves the registry.
func registerGraphEmbedding(registry *component.Registry, models *model.Registry) error {
	return graphembedding.RegisterWithEmbedder(registry, embedderType, httpEmbedderFor(models))
}

// httpEmbedderFor returns graph-embedding's embedder constructor: the model
// registry's embedding endpoint mapped to httpembedder.Config, per
// processor/graph-embedding/MIGRATION.md. The constructor sends no request, so
// a provider that is down at start degrades graph-embedding rather than failing
// boot. The endpoint's query_prefix passes through as configured: on semembed
// it is left empty, because httpembedder sends input_type and semembed applies
// the model's own query instruction.
func httpEmbedderFor(models *model.Registry) func() (embedding.Embedder, error) {
	return func() (embedding.Embedder, error) {
		// A nil *model.Registry inside the RegistryReader interface is not a
		// nil interface, so ResolveEndpoint would not catch it.
		if models == nil {
			return nil, errors.New("graph-embedding: no model_registry: tier 1 needs an embedding endpoint")
		}
		resolved, endpoint, err := model.ResolveEndpointWithConfig(models, model.CapabilityEmbedding)
		if err != nil {
			return nil, fmt.Errorf("graph-embedding: resolve embedding endpoint: %w", err)
		}
		embedder, err := httpembedder.New(httpembedder.Config{
			BaseURL:     resolved.URL,
			Model:       resolved.Model,
			APIKey:      resolved.APIKey,
			QueryPrefix: endpoint.QueryPrefix,
			// A declared width lets a restart keep the stored vectors instead
			// of re-embedding what replays before the first reply (semengine#722).
			Dimensions: endpoint.Dimensions,
		})
		if err != nil {
			return nil, fmt.Errorf("graph-embedding: build http embedder: %w", err)
		}
		return embedder, nil
	}
}
