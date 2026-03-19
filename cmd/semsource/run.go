package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// Register payloads and vocabulary in the semstreams registries.
	_ "github.com/c360studio/semsource/graph"
	_ "github.com/c360studio/semsource/processor/source-manifest"
	_ "github.com/c360studio/semsource/source/ast"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	audiosource "github.com/c360studio/semsource/processor/audio-source"
	cfgfilesource "github.com/c360studio/semsource/processor/cfgfile-source"
	docsource "github.com/c360studio/semsource/processor/doc-source"
	gitsource "github.com/c360studio/semsource/processor/git-source"
	imagesource "github.com/c360studio/semsource/processor/image-source"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	urlsource "github.com/c360studio/semsource/processor/url-source"
	videosource "github.com/c360studio/semsource/processor/video-source"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semsource/workspace"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
)

// runCmd is the semstreams-native entry point for semsource.
//
// Wire-up sequence:
//  1. Parse flags, load semsource config
//  2. Connect to NATS, ensure streams exist
//  3. Build semstreams config.Config (platform identity + component map)
//  4. Create component registry, register all factories
//  5. Create ServiceManager, configure, start
//  6. Block on SIGINT/SIGTERM, then shut down cleanly
func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	natsURL := fs.String("nats-url", "", "NATS server URL (overrides NATS_URL env and config)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := buildLogger(*logLevel)
	slog.SetDefault(logger)

	ctx := context.Background()

	semsourceCfg, expandResult, err := loadAndExpandConfig(ctx, *configPath)
	if err != nil {
		return err
	}

	logger.Info("semsource starting",
		"version", version,
		"mode", semsourceCfg.Mode,
		"namespace", semsourceCfg.Namespace,
		"sources", len(semsourceCfg.Sources),
		"branch_watchers", len(expandResult.Watchers),
	)

	nc, ssCfg, err := setupNATS(ctx, *natsURL, semsourceCfg, logger)
	if err != nil {
		return err
	}
	defer nc.Close(context.Background())

	configMgr, err := semconfig.NewConfigManager(ssCfg, nc, logger)
	if err != nil {
		return fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return fmt.Errorf("start config manager: %w", err)
	}
	defer configMgr.Stop(5 * time.Second)

	registry := component.NewRegistry()
	if err := registerComponentFactories(registry, semsourceCfg.IsHeadless()); err != nil {
		return err
	}

	manager, err := createServiceManager(semsourceCfg, ssCfg, nc, registry, configMgr, logger)
	if err != nil {
		configMgr.Stop(5 * time.Second)
		return err
	}

	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	startBranchWatchers(signalCtx, expandResult.Watchers, semsourceCfg, configMgr, logger)

	logger.Info("semsource running — waiting for shutdown signal")
	<-signalCtx.Done()
	logger.Info("shutdown signal received")

	if err := manager.StopAll(30 * time.Second); err != nil {
		logger.Error("error stopping services", "error", err)
		return fmt.Errorf("stop services: %w", err)
	}

	logger.Info("semsource stopped")
	return nil
}

// setupNATS connects to NATS and ensures JetStream streams exist.
func setupNATS(ctx context.Context, natsFlag string, cfg *config.Config, logger *slog.Logger) (*natsclient.Client, *semconfig.Config, error) {
	natsAddr := resolveNATSURL(natsFlag, cfg)
	nc, err := connectNATS(ctx, natsAddr, logger)
	if err != nil {
		return nil, nil, err
	}

	ssCfg, err := buildSemstreamsConfig(cfg, cfg.Namespace)
	if err != nil {
		nc.Close(context.Background())
		return nil, nil, fmt.Errorf("build semstreams config: %w", err)
	}

	// In headless mode the host app owns JetStream infrastructure —
	// skip stream creation/updates to avoid conflicts.
	if !cfg.IsHeadless() {
		streamsManager := semconfig.NewStreamsManager(nc, logger)
		if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {
			nc.Close(context.Background())
			return nil, nil, fmt.Errorf("ensure JetStream streams: %w", err)
		}
		logger.Info("JetStream streams ready")
	} else {
		logger.Info("headless mode — skipping stream provisioning (host app owns infrastructure)")
	}

	return nc, ssCfg, nil
}

// loadAndExpandConfig loads the semsource config and expands "repo" meta-sources
// into their component sources (git, ast, docs, config).
func loadAndExpandConfig(ctx context.Context, path string) (*config.Config, *config.ExpandResult, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, nil, fmt.Errorf("load config %q: %w", path, err)
	}
	result, err := config.ExpandRepoSources(ctx, cfg.Sources, cfg.WorkspaceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("expand repo sources: %w", err)
	}
	cfg.Sources = result.Sources
	return cfg, result, nil
}

// registerComponentFactories registers component factories based on mode.
// In standalone mode, all semstreams built-in factories (graph, agentic, etc.)
// are registered alongside semsource factories. In headless mode, only
// semsource's own factories plus the graph-layer factories needed for
// standalone are registered — no agentic, protocol, or other factories that
// could collide with the host app's components on the shared KV bucket.
func registerComponentFactories(registry *component.Registry, headless bool) error {
	if !headless {
		if err := componentregistry.Register(registry); err != nil {
			return fmt.Errorf("register semstreams components: %w", err)
		}
	}
	if err := registerSemsourceFactories(registry); err != nil {
		return err
	}
	slog.Info("component factories registered", "count", len(registry.ListFactories()))
	return nil
}

// registerSemsourceFactories registers all semsource-specific component factories.
func registerSemsourceFactories(registry *component.Registry) error {
	for name, fn := range map[string]func() error{
		"ast-source":      func() error { return astsource.Register(registry) },
		"git-source":      func() error { return gitsource.Register(registry) },
		"doc-source":      func() error { return docsource.Register(registry) },
		"cfgfile-source":  func() error { return cfgfilesource.Register(registry) },
		"url-source":      func() error { return urlsource.Register(registry) },
		"image-source":    func() error { return imagesource.Register(registry) },
		"video-source":    func() error { return videosource.Register(registry) },
		"audio-source":    func() error { return audiosource.Register(registry) },
		"filestore":       func() error { return filestore.Register(registry) },
		"source-manifest": func() error { return sourcemanifest.Register(registry) },
	} {
		if err := fn(); err != nil {
			return fmt.Errorf("register %s component: %w", name, err)
		}
	}
	return nil
}

// createServiceManager builds the ServiceManager with all configured services.
func createServiceManager(
	cfg *config.Config,
	ssCfg *semconfig.Config,
	nc *natsclient.Client,
	registry *component.Registry,
	configMgr *semconfig.Manager,
	logger *slog.Logger,
) (*service.Manager, error) {
	metricsRegistry := metric.NewMetricsRegistry()
	platform := types.PlatformMeta{
		Org:      cfg.Namespace,
		Platform: "semsource",
	}

	serviceRegistry := service.NewServiceRegistry()
	if err := service.RegisterAll(serviceRegistry); err != nil {
		return nil, fmt.Errorf("register services: %w", err)
	}

	manager := service.NewServiceManager(serviceRegistry)
	deps := &service.Dependencies{
		NATSClient:        nc,
		MetricsRegistry:   metricsRegistry,
		Logger:            logger,
		Platform:          platform,
		Manager:           configMgr,
		ComponentRegistry: registry,
	}

	if err := manager.ConfigureFromServices(ssCfg.Services, deps); err != nil {
		return nil, fmt.Errorf("configure service manager: %w", err)
	}

	for name, svcConfig := range ssCfg.Services {
		if name == "service-manager" {
			continue
		}
		if !svcConfig.Enabled {
			logger.Info("service disabled, skipping", "name", name)
			continue
		}
		if _, err := manager.CreateService(name, svcConfig.Config, deps); err != nil {
			return nil, fmt.Errorf("create service %s: %w", name, err)
		}
		logger.Debug("created service", "name", name)
	}

	return manager, nil
}

// resolveNATSURL picks the NATS URL in priority order:
//  1. Explicit --nats-url flag
//  2. NATS_URL environment variable
//  3. EntityStore config (if present, reuse its URL)
//  4. Default localhost
func resolveNATSURL(flagValue string, cfg *config.Config) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	if cfg.EntityStore != nil && cfg.EntityStore.NATSUrl != "" {
		return cfg.EntityStore.NATSUrl
	}
	return "nats://localhost:4222"
}

// connectNATS creates a natsclient.Client and establishes the connection.
func connectNATS(ctx context.Context, url string, logger *slog.Logger) (*natsclient.Client, error) {
	logger.Info("connecting to NATS", "url", url)

	nc, err := natsclient.NewClient(url,
		natsclient.WithName("semsource"),
		natsclient.WithMaxReconnects(-1),
		natsclient.WithReconnectWait(2*time.Second),
		natsclient.WithHealthInterval(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("create NATS client: %w", err)
	}

	if err := nc.Connect(ctx); err != nil {
		nc.Close(context.Background())
		return nil, wrapNATSConnError(err, url)
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := nc.WaitForConnection(connCtx); err != nil {
		nc.Close(context.Background())
		return nil, wrapNATSConnError(err, url)
	}

	logger.Info("connected to NATS", "url", url)
	return nc, nil
}

// wrapNATSConnError adds actionable guidance for common NATS connection failures.
func wrapNATSConnError(err error, url string) error {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no servers available") ||
		strings.Contains(msg, "timeout") {
		return fmt.Errorf("NATS connection failed at %s: %w\n\nStart NATS with: docker compose up -d nats\nOr set NATS_URL to your server address", url, err)
	}
	return fmt.Errorf("NATS connection failed: %w", err)
}

// buildSemstreamsConfig constructs a semstreams config.Config from the semsource
// config. In standalone mode it wires the full graph subsystem and WebSocket
// output. In headless mode only source components and the manifest are included.
func buildSemstreamsConfig(cfg *config.Config, org string) (*semconfig.Config, error) {
	components, err := sourceComponents(cfg, org)
	if err != nil {
		return nil, err
	}

	manifestCfg, err := manifestComponentConfig(cfg, org, len(components))
	if err != nil {
		return nil, err
	}
	components["source-manifest"] = manifestCfg

	// Graph subsystem and WebSocket are standalone-only.
	if !cfg.IsHeadless() {
		graphComponents, err := graphSubsystemComponents(cfg)
		if err != nil {
			return nil, err
		}
		for k, v := range graphComponents {
			components[k] = v
		}

		wsCfg, err := websocketComponentConfig(cfg)
		if err != nil {
			return nil, err
		}
		components["websocket-output"] = wsCfg
	}

	svcs, err := serviceConfigs(cfg)
	if err != nil {
		return nil, err
	}

	ssCfg := &semconfig.Config{
		Version: "1.0.0",
		Platform: semconfig.PlatformConfig{
			Org:         org,
			ID:          "semsource",
			Environment: "dev",
		},
		NATS: semconfig.NATSConfig{
			JetStream: semconfig.JetStreamConfig{Enabled: true},
		},
		Services:   svcs,
		Components: components,
	}

	// In headless mode, no stream config — the host app owns infrastructure.
	// In standalone mode, declare the full GRAPH stream with user overrides.
	if !cfg.IsHeadless() {
		ssCfg.Streams = graphStreamConfig(cfg)
	}

	return ssCfg, nil
}

// sourceComponents translates semsource config sources into semstreams component configs.
func sourceComponents(cfg *config.Config, org string) (semconfig.ComponentConfigs, error) {
	components := make(semconfig.ComponentConfigs)

	for i, src := range cfg.Sources {
		switch src.Type {
		case "ast":
			instanceName, compCfg, err := astSourceComponentConfig(src, org, i)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (ast): %w", i, err)
			}
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (ast) marshal config: %w", i, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    "ast-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		case "git":
			instanceName, compCfg, err := gitSourceComponentConfig(src, org, i, cfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (git): %w", i, err)
			}
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (git) marshal config: %w", i, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    "git-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		case "docs":
			instanceName, compCfg := docSourceComponentConfig(src, org, i)
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (docs) marshal config: %w", i, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    "doc-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		case "config":
			instanceName, compCfg := cfgfileSourceComponentConfig(src, org, i)
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (config) marshal config: %w", i, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    "cfgfile-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		case "url":
			instanceName, compCfg := urlSourceComponentConfig(src, org, i)
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (url) marshal config: %w", i, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    "url-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		case "image", "video", "audio":
			instanceName, compCfg := mediaSourceComponentConfig(src, org, i, cfg)
			rawCfg, err := json.Marshal(compCfg)
			if err != nil {
				return nil, fmt.Errorf("source[%d] (%s) marshal config: %w", i, src.Type, err)
			}
			components[instanceName] = types.ComponentConfig{
				Name:    src.Type + "-source",
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		default:
			slog.Warn("source type not yet migrated to component — skipped",
				"index", i, "type", src.Type, "path", src.Path)
		}
	}

	return components, nil
}

// manifestComponentConfig builds the source-manifest component config.
func manifestComponentConfig(cfg *config.Config, org string, sourceCount int) (types.ComponentConfig, error) {
	manifestSources := make([]sourcemanifest.ManifestSource, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		manifestSources = append(manifestSources, sourcemanifest.ManifestSource{
			Type:         src.Type,
			Path:         src.Path,
			Paths:        src.Paths,
			URL:          src.URL,
			URLs:         src.URLs,
			Language:     src.Language,
			Branch:       src.Branch,
			Watch:        src.Watch,
			PollInterval: src.PollInterval,
		})
	}
	raw, err := json.Marshal(map[string]any{
		"ports": map[string]any{
			"outputs": []map[string]any{
				{
					"name":        "graph.ingest",
					"type":        "jetstream",
					"subject":     "graph.ingest.manifest",
					"stream_name": "GRAPH",
					"required":    true,
					"description": "Source manifest broadcast for downstream consumers",
				},
				{
					"name":        "graph.ingest.status",
					"type":        "jetstream",
					"subject":     "graph.ingest.status",
					"stream_name": "GRAPH",
					"required":    false,
					"description": "Ingestion status broadcast for downstream consumers",
				},
				{
					"name":        "graph.ingest.predicates",
					"type":        "jetstream",
					"subject":     "graph.ingest.predicates",
					"stream_name": "GRAPH",
					"required":    false,
					"description": "Predicate schema broadcast for downstream consumers",
				},
			},
		},
		"namespace":             org,
		"sources":               manifestSources,
		"expected_source_count": sourceCount,
	})
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("marshal source-manifest config: %w", err)
	}
	return types.ComponentConfig{
		Name:    "source-manifest",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}, nil
}

// graphSubsystemComponents returns the built-in semstreams graph components:
// ingest, index, query, and gateway.
func graphSubsystemComponents(cfg *config.Config) (semconfig.ComponentConfigs, error) {
	// Resolve graph config with defaults.
	gatewayBind := "0.0.0.0:8082"
	enablePlayground := true
	embedderType := "bm25"
	batchSize := 50
	coalesceMs := 200

	if g := cfg.Graph; g != nil {
		if g.GatewayBind != "" {
			gatewayBind = g.GatewayBind
		}
		if g.EnablePlayground != nil {
			enablePlayground = *g.EnablePlayground
		}
		if g.EmbedderType != "" {
			embedderType = g.EmbedderType
		}
		if g.EmbeddingBatchSize > 0 {
			batchSize = g.EmbeddingBatchSize
		}
		if g.CoalesceMs > 0 {
			coalesceMs = g.CoalesceMs
		}
	}

	configs := map[string]struct {
		name      string
		compType  types.ComponentType
		configMap map[string]any
	}{
		"graph-ingest": {
			name:     "graph-ingest",
			compType: types.ComponentTypeProcessor,
			configMap: map[string]any{
				"ports": map[string]any{
					"inputs": []map[string]any{
						{
							"name":        "entity_stream",
							"type":        "jetstream",
							"subject":     "graph.ingest.entity",
							"stream_name": "GRAPH",
							"config":      map[string]any{"deliver_policy": "all"},
						},
					},
					"outputs": []map[string]any{
						{"name": "entity_states", "type": "kv-write", "subject": "ENTITY_STATES"},
					},
				},
			},
		},
		"graph-index": {
			name:     "graph-index",
			compType: types.ComponentTypeProcessor,
			configMap: map[string]any{
				"coalesce_ms": coalesceMs,
				"ports": map[string]any{
					"inputs": []map[string]any{
						{"name": "entity_watch", "type": "kv-watch", "subject": "ENTITY_STATES"},
					},
					"outputs": []map[string]any{
						{"name": "outgoing_index", "type": "kv-write", "subject": "OUTGOING_INDEX"},
						{"name": "incoming_index", "type": "kv-write", "subject": "INCOMING_INDEX"},
						{"name": "alias_index", "type": "kv-write", "subject": "ALIAS_INDEX"},
						{"name": "predicate_index", "type": "kv-write", "subject": "PREDICATE_INDEX"},
					},
				},
			},
		},
		"graph-embedding": {
			name:     "graph-embedding",
			compType: types.ComponentTypeProcessor,
			configMap: map[string]any{
				"coalesce_ms":   coalesceMs,
				"embedder_type": embedderType,
				"batch_size":    batchSize,
				"ports": map[string]any{
					"inputs": []map[string]any{
						{"name": "entity_watch", "type": "kv-watch", "subject": "ENTITY_STATES"},
					},
					"outputs": []map[string]any{
						{"name": "embeddings", "type": "kv-write", "subject": "EMBEDDINGS_CACHE"},
					},
				},
			},
		},
		"graph-query": {
			name:     "graph-query",
			compType: types.ComponentTypeProcessor,
			configMap: map[string]any{
				"ports": map[string]any{
					"inputs": []map[string]any{
						{"name": "query_entity", "type": "nats-request", "subject": "graph.query.entity"},
						{"name": "query_relationships", "type": "nats-request", "subject": "graph.query.relationships"},
						{"name": "query_path_search", "type": "nats-request", "subject": "graph.query.pathSearch"},
					},
				},
			},
		},
		"graph-gateway": {
			name:     "graph-gateway",
			compType: types.ComponentTypeGateway,
			configMap: map[string]any{
				"ports": map[string]any{
					"inputs": []map[string]any{
						{"name": "http", "type": "http", "subject": "/graphql"},
					},
					"outputs": []map[string]any{
						{"name": "mutations", "type": "nats-request", "subject": "graph.mutation.*"},
					},
				},
				"bind_address":      gatewayBind,
				"enable_playground": enablePlayground,
			},
		},
	}

	result := make(semconfig.ComponentConfigs, len(configs))
	for key, c := range configs {
		raw, err := json.Marshal(c.configMap)
		if err != nil {
			return nil, fmt.Errorf("marshal %s config: %w", key, err)
		}
		result[key] = types.ComponentConfig{
			Name:    c.name,
			Type:    c.compType,
			Enabled: true,
			Config:  raw,
		}
	}
	return result, nil
}

// graphStreamConfig builds the GRAPH stream config with user overrides applied.
func graphStreamConfig(cfg *config.Config) semconfig.StreamConfigs {
	sc := semconfig.StreamConfig{
		Subjects: []string{
			"graph.ingest.entity",
			"graph.ingest.batch",
			"graph.ingest.manifest",
			"graph.ingest.status",
			"graph.ingest.predicates",
		},
		Storage:  "memory",
		MaxBytes: 256 * 1024 * 1024,
		MaxAge:   "1h",
		Replicas: 1,
	}

	if override, ok := cfg.Streams["GRAPH"]; ok {
		if override.Storage != "" {
			sc.Storage = override.Storage
		}
		if override.MaxBytes != nil {
			sc.MaxBytes = *override.MaxBytes
		}
		if override.MaxAge != "" {
			sc.MaxAge = override.MaxAge
		}
		if override.Replicas != nil {
			sc.Replicas = *override.Replicas
		}
	}

	return semconfig.StreamConfigs{"GRAPH": sc}
}

// websocketComponentConfig builds the WebSocket output component config.
func websocketComponentConfig(cfg *config.Config) (types.ComponentConfig, error) {
	wsAddr := fmt.Sprintf("http://%s%s", cfg.WebSocketBind, cfg.WebSocketPath)
	raw, err := json.Marshal(map[string]any{
		"ports": map[string]any{
			"inputs": []map[string]any{
				{
					"name":        "graph_entities",
					"type":        "jetstream",
					"subject":     "graph.ingest.>",
					"stream_name": "GRAPH",
					"required":    true,
					"description": "Entity, status, and predicate payloads from source components",
				},
			},
			"outputs": []map[string]any{
				{
					"name":        "websocket_server",
					"type":        "network",
					"subject":     wsAddr,
					"description": "WebSocket server for downstream consumers",
				},
			},
		},
		"delivery_mode": "at-most-once",
	})
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("marshal websocket config: %w", err)
	}
	return types.ComponentConfig{
		Name:    "websocket",
		Type:    types.ComponentTypeOutput,
		Enabled: true,
		Config:  raw,
	}, nil
}

// serviceConfigs builds the semstreams service configurations.
func serviceConfigs(cfg *config.Config) (types.ServiceConfigs, error) {
	smCfgJSON, err := json.Marshal(map[string]any{"http_port": cfg.HTTPPort})
	if err != nil {
		return nil, fmt.Errorf("marshal service-manager config: %w", err)
	}

	metricsPort := 9091
	metricsPath := "/metrics"
	if m := cfg.Metrics; m != nil {
		if m.Port > 0 {
			metricsPort = m.Port
		}
		if m.Path != "" {
			metricsPath = m.Path
		}
	}
	metricsCfgJSON, err := json.Marshal(map[string]any{"port": metricsPort, "path": metricsPath})
	if err != nil {
		return nil, fmt.Errorf("marshal metrics config: %w", err)
	}

	return types.ServiceConfigs{
		"service-manager": types.ServiceConfig{
			Name:    "service-manager",
			Enabled: true,
			Config:  smCfgJSON,
		},
		"component-manager": types.ServiceConfig{
			Name:    "component-manager",
			Enabled: true,
			Config:  json.RawMessage(`{}`),
		},
		"metrics": types.ServiceConfig{
			Name:    "metrics",
			Enabled: true,
			Config:  metricsCfgJSON,
		},
		"heartbeat": types.ServiceConfig{
			Name:    "heartbeat",
			Enabled: true,
			Config:  json.RawMessage(`{}`),
		},
		"flow-builder": types.ServiceConfig{
			Name:    "flow-builder",
			Enabled: true,
			Config:  json.RawMessage(`{}`),
		},
	}, nil
}

// sourceOutputPorts returns the standard output port config shared by all source
// components. The flow engine reads ports from the config JSON to discover
// connections — without explicit ports in the config, the flow graph shows 0
// connections even though components use DefaultConfig at runtime.
func sourceOutputPorts() map[string]any {
	return map[string]any{
		"outputs": []map[string]any{
			{
				"name":        "graph.ingest",
				"type":        "jetstream",
				"subject":     "graph.ingest.entity",
				"stream_name": "GRAPH",
				"required":    true,
				"description": "Entity state updates for graph ingestion",
			},
		},
	}
}

// astSourceComponentConfig builds a component instance name and config map for
// an AST source entry. The instance name is derived from the path so multiple
// AST sources produce distinct component instances.
func astSourceComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any, error) {
	path := src.Path
	if path == "" {
		path = "."
	}

	lang := src.Language
	if lang == "" {
		lang = "go"
	}

	slug := entityid.SystemSlug(path)
	if slug == "" {
		slug = fmt.Sprintf("source-%d", index)
	}
	project := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("ast-source-%s", project)

	compCfg := map[string]any{
		"ports": sourceOutputPorts(),
		"watch_paths": []map[string]any{
			{
				"path":      path,
				"org":       org,
				"project":   project,
				"languages": []string{lang},
			},
		},
		"watch_enabled":  src.Watch,
		"index_interval": "60s",
		"instance_name":  instanceName,
	}

	return instanceName, compCfg, nil
}

// gitSourceComponentConfig builds a component instance name and config map for
// a git source entry.
func gitSourceComponentConfig(src config.SourceEntry, org string, index int, cfg *config.Config) (string, map[string]any, error) {
	identifier := src.URL
	if identifier == "" {
		identifier = src.Path
	}
	if identifier == "" {
		identifier = fmt.Sprintf("git-%d", index)
	}
	slug := entityid.SystemSlug(identifier)
	if slug == "" {
		slug = fmt.Sprintf("git-%d", index)
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("git-source-%s", scopedSlug)

	branch := src.Branch
	if branch == "" {
		branch = "main"
	}

	compCfg := map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"repo_path":     src.Path,
		"repo_url":      src.URL,
		"branch":        branch,
		"poll_interval": "60s",
		"watch_enabled": src.Watch,
		"workspace_dir": cfg.WorkspaceDir,
		"git_token":     cfg.GitToken,
		"branch_slug":   src.BranchSlug,
		"instance_name": instanceName,
	}

	return instanceName, compCfg, nil
}

// docSourceComponentConfig builds config for a docs source entry.
func docSourceComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("docs-%d", index)
	if len(paths) > 0 {
		slug = entityid.SystemSlug(paths[0])
		if slug == "" {
			slug = fmt.Sprintf("docs-%d", index)
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("doc-source-%s", scopedSlug)
	return instanceName, map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
}

// cfgfileSourceComponentConfig builds config for a config file source entry.
func cfgfileSourceComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("config-%d", index)
	if len(paths) > 0 {
		slug = entityid.SystemSlug(paths[0])
		if slug == "" {
			slug = fmt.Sprintf("config-%d", index)
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("cfgfile-source-%s", scopedSlug)
	return instanceName, map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
}

// urlSourceComponentConfig builds config for a URL source entry.
func urlSourceComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	urls := src.URLs
	if len(urls) == 0 && src.URL != "" {
		urls = []string{src.URL}
	}
	slug := fmt.Sprintf("url-%d", index)
	if len(urls) > 0 {
		slug = entityid.SystemSlug(urls[0])
		if slug == "" {
			slug = fmt.Sprintf("url-%d", index)
		}
	}
	instanceName := fmt.Sprintf("url-source-%s", slug)
	return instanceName, map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"urls":          urls,
		"poll_interval": "300s",
		"instance_name": instanceName,
	}
}

// mediaSourceComponentConfig builds config for image, video, or audio source entries.
func mediaSourceComponentConfig(src config.SourceEntry, org string, index int, cfg *config.Config) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("%s-%d", src.Type, index)
	if len(paths) > 0 {
		s := entityid.SystemSlug(paths[0])
		if s != "" {
			slug = s
		}
	}
	instanceName := fmt.Sprintf("%s-source-%s", src.Type, slug)

	compCfg := map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}

	if cfg.MediaStoreDir != "" {
		compCfg["file_store_root"] = cfg.MediaStoreDir
	}

	return instanceName, compCfg
}

// startBranchWatchers starts a poll goroutine for each multi-branch repo.
// When new branches are discovered, component configs are pushed to the
// ConfigManager's KV store, which triggers the ServiceManager to create
// and start the corresponding components reactively.
func startBranchWatchers(
	ctx context.Context,
	watchers []config.BranchWatcherRef,
	cfg *config.Config,
	configMgr *semconfig.Manager,
	logger *slog.Logger,
) {
	for _, ref := range watchers {
		bw := workspace.NewBranchWatcher(workspace.BranchWatcherConfig{
			RepoPath:     ref.RepoPath,
			Patterns:     ref.Patterns,
			WorktreeBase: ref.WorktreeBase,
			MaxBranches:  ref.MaxBranches,
			Logger:       logger,
		})

		interval := parseBranchPollInterval(ref.BranchPollInterval)
		logger.Info("starting branch watcher",
			"repo", ref.RepoPath,
			"patterns", ref.Patterns,
			"interval", interval)

		go bw.Run(ctx, interval, func(added []workspace.BranchState, removed []string) {
			for _, bs := range added {
				logger.Info("branch discovered",
					"branch", bs.Branch,
					"worktree", bs.WorktreePath)
				publishBranchComponents(ctx, bs, ref, cfg, configMgr, logger)
			}
			for _, branch := range removed {
				logger.Info("branch removed", "branch", branch)
				removeBranchComponents(ctx, branch, ref, configMgr, logger)
			}
		})
	}
}

// publishBranchComponents pushes 4 component configs (git, ast, doc, config)
// for a discovered branch to the ConfigManager KV store.
func publishBranchComponents(
	ctx context.Context,
	bs workspace.BranchState,
	ref config.BranchWatcherRef,
	cfg *config.Config,
	configMgr *semconfig.Manager,
	logger *slog.Logger,
) {
	src := config.SourceEntry{
		URL:        "", // local repo
		Path:       bs.WorktreePath,
		Branch:     bs.Branch,
		Watch:      ref.Watch,
		Language:   ref.Language,
		BranchSlug: bs.Slug,
	}

	componentConfigs := buildBranchComponentConfigs(src, cfg.Namespace, cfg)
	for name, compCfg := range componentConfigs {
		if err := configMgr.PutComponentToKV(ctx, name, compCfg); err != nil {
			logger.Warn("failed to publish branch component config",
				"component", name,
				"branch", bs.Branch,
				"error", err)
		}
	}
}

// removeBranchComponents removes 4 component configs for a deleted branch
// from the ConfigManager KV store.
func removeBranchComponents(
	ctx context.Context,
	branch string,
	ref config.BranchWatcherRef,
	configMgr *semconfig.Manager,
	logger *slog.Logger,
) {
	slug := workspace.BranchSlug(branch)
	repoSlug := entityid.SystemSlug(ref.RepoPath)

	prefixes := []string{"git-source-", "ast-source-", "doc-source-", "cfgfile-source-"}
	for _, prefix := range prefixes {
		name := prefix + entityid.BranchScopedSlug(repoSlug, slug)
		if err := configMgr.DeleteComponentFromKV(ctx, name); err != nil {
			logger.Warn("failed to remove branch component config",
				"component", name,
				"branch", branch,
				"error", err)
		}
	}
}

// buildBranchComponentConfigs creates the 4 component configs for a single branch.
func buildBranchComponentConfigs(src config.SourceEntry, org string, cfg *config.Config) map[string]types.ComponentConfig {
	configs := make(map[string]types.ComponentConfig)

	// Git source
	gitName, gitCfg, err := gitSourceComponentConfig(src, org, 0, cfg)
	if err == nil {
		raw, _ := json.Marshal(gitCfg)
		configs[gitName] = types.ComponentConfig{
			Name:    "git-source",
			Type:    types.ComponentTypeProcessor,
			Enabled: true,
			Config:  raw,
		}
	}

	// AST source
	astName, astCfg, err := astSourceComponentConfig(src, org, 0)
	if err == nil {
		raw, _ := json.Marshal(astCfg)
		configs[astName] = types.ComponentConfig{
			Name:    "ast-source",
			Type:    types.ComponentTypeProcessor,
			Enabled: true,
			Config:  raw,
		}
	}

	// Doc source
	docName, docCfg := docSourceComponentConfig(src, org, 0)
	raw, _ := json.Marshal(docCfg)
	configs[docName] = types.ComponentConfig{
		Name:    "doc-source",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}

	// Config file source
	cfgName, cfgfileCfg := cfgfileSourceComponentConfig(src, org, 0)
	raw, _ = json.Marshal(cfgfileCfg)
	configs[cfgName] = types.ComponentConfig{
		Name:    "cfgfile-source",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}

	return configs
}

// parseBranchPollInterval parses the poll interval string with a default of 15s.
func parseBranchPollInterval(s string) time.Duration {
	if s == "" {
		return 15 * time.Second
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 15 * time.Second
	}
	return d
}
