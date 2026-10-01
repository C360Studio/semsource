package main

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
)

// prepareSourceLifecycle repairs desired facts and freezes boot receipt eligibility
// before query scopes, registry factories, or admitted producer construction.
func prepareSourceLifecycle(ctx context.Context, cfg *config.Config, manager *semconfig.Manager, nc *natsclient.Client, projector *sourcelifecycle.LocalProjector) (*sourcelifecycle.Coordinator, map[string]sourceintent.Binding, error) {
	effective := manager.GetConfig().Get()
	authority := entityid.Authority{Org: effective.Platform.Org, Platform: effective.Platform.ID}
	inputs, err := sourcelifecycle.GraphInputs(effective.Components)
	if err != nil {
		return nil, nil, err
	}
	replicas := effective.Streams[inputs[0].Stream].Replicas
	if replicas < 1 {
		replicas = 1
	}
	journal, err := sourcelifecycle.OpenJournal(ctx, nc, authority, cfg.Namespace, replicas)
	if err != nil {
		return nil, nil, err
	}
	readDesired, err := sourcelifecycle.ConfigReader(ctx, nc, cfg.Namespace, cfg.PlatformStem())
	if err != nil {
		return nil, nil, err
	}
	spawn := ingestHandlerConfig(cfg, manager).Spawn
	owner, err := sourcelifecycle.NewCoordinator(sourcelifecycle.CoordinatorConfig{Authority: authority, Namespace: cfg.Namespace, Journal: journal, Store: manager, Tail: sourcelifecycle.NATSTail{Client: nc}, Projector: projector, Inputs: inputs, ReadDesired: readDesired, RepairManifest: func(ctx context.Context, handle string) error {
		return sourcemanifest.RepairRemovalManifest(ctx, manager, spawn, handle)
	}})
	if err != nil {
		return nil, nil, err
	}
	release, err := owner.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	if err := owner.RepairDesired(ctx); err != nil {
		return nil, nil, fmt.Errorf("repair source lifecycle desired state: %w", err)
	}
	bindings, err := owner.BindBoot(ctx)
	if err != nil {
		return nil, nil, err
	}
	return owner, bindings, nil
}

func prepareComponentRegistry(ctx context.Context, cfg *config.Config, manager *semconfig.Manager, nc *natsclient.Client) (*component.Registry, error) {
	lifecycleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	projector := &sourcelifecycle.LocalProjector{}
	lifecycle, bindings, err := prepareSourceLifecycle(lifecycleCtx, cfg, manager, nc, projector)
	if err != nil {
		return nil, fmt.Errorf("prepare source lifecycle: %w", err)
	}
	registry := component.NewRegistry()
	sources, err := newLifecycleRegistry(registry, manager.GetConfig().Get().Components, bindings, lifecycle.PublisherObserver())
	if err != nil {
		return nil, fmt.Errorf("bind source lifecycle: %w", err)
	}
	scopes, err := sourceScopeSystems(manager.GetConfig().Get().Components)
	if err != nil {
		return nil, fmt.Errorf("derive source query scopes: %w", err)
	}
	ingest := ingestHandlerConfig(cfg, manager)
	ingest.Lifecycle = lifecycle
	if err := registerComponentFactories(registry, componentWiring{Ingest: &ingest, ScopeSystems: scopes, SourceRegistry: sources, ProjectionRegistry: &projectionRegistry{registry: registry, projector: projector}}); err != nil {
		return nil, err
	}
	return registry, nil
}
