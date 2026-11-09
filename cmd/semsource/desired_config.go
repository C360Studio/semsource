package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
	"strings"
)

func prepareComponentRegistry(ctx context.Context, cfg *config.Config, manager *semconfig.Manager, nc *natsclient.Client) (*component.Registry, error) {
	registry := component.NewRegistry()
	scopes, err := sourceScopeSystems(manager.GetConfig().Get().Components)
	if err != nil {
		return nil, err
	}
	ingest := ingestHandlerConfig(cfg, manager)
	ingest.ReadDesired, err = readDesiredConfig(ctx, nc, cfg.Namespace, cfg.PlatformStem())
	if err != nil {
		return nil, err
	}
	if err := registerComponentFactories(registry, componentWiring{Ingest: &ingest, ScopeSystems: scopes}); err != nil {
		return nil, err
	}
	if err := requireRegisteredFactories(registry, manager.GetConfig().Get().Components); err != nil {
		return nil, err
	}
	return registry, nil
}

// requireRegisteredFactories fails boot when an enabled component names a
// factory nobody registered. The pinned ComponentManager logs such a component
// and skips it, so without this check a dropped registration boots a service
// that silently lacks the component.
func requireRegisteredFactories(registry *component.Registry, components semconfig.ComponentConfigs) error {
	for instance, cc := range components {
		if !cc.Enabled {
			continue
		}
		if _, ok := registry.GetFactory(cc.Name); !ok {
			return fmt.Errorf("component %q names unregistered factory %q", instance, cc.Name)
		}
	}
	return nil
}

// readDesiredConfig observes existing ConfigManager authority after ambiguous writes.
func readDesiredConfig(ctx context.Context, client *natsclient.Client, org, stem string) (func(context.Context, string) (types.ComponentConfig, bool, error), error) {
	bucket, err := semconfig.BucketName(org, stem)
	if err != nil {
		return nil, err
	}
	reader, err := semgraph.OpenCatalogReader(ctx, client, bucket)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, handle string) (types.ComponentConfig, bool, error) {
		var cc types.ComponentConfig
		entry, err := reader.Get(ctx, "components."+strings.ReplaceAll(handle, " ", "_"))
		if err != nil {
			if natsclient.IsKVNotFoundError(err) {
				return cc, false, nil
			}
			return cc, false, err
		}
		if err := json.Unmarshal(entry.Value(), &cc); err != nil {
			return cc, false, err
		}
		return cc, true, nil
	}, nil
}
