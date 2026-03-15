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

	semsourceCfg, err := loadAndExpandConfig(*configPath)
	if err != nil {
		return err
	}

	logger.Info("semsource starting",
		"version", version,
		"namespace", semsourceCfg.Namespace,
		"sources", len(semsourceCfg.Sources),
	)

	ctx := context.Background()

	natsAddr := resolveNATSURL(*natsURL, semsourceCfg)
	nc, err := connectNATS(ctx, natsAddr, logger)
	if err != nil {
		return err
	}
	defer nc.Close(context.Background())

	ssCfg, err := buildSemstreamsConfig(semsourceCfg, semsourceCfg.Namespace)
	if err != nil {
		return fmt.Errorf("build semstreams config: %w", err)
	}

	streamsManager := semconfig.NewStreamsManager(nc, logger)
	if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {
		return fmt.Errorf("ensure JetStream streams: %w", err)
	}
	logger.Info("JetStream streams ready")

	configMgr, err := semconfig.NewConfigManager(ssCfg, nc, logger)
	if err != nil {
		return fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return fmt.Errorf("start config manager: %w", err)
	}
	defer configMgr.Stop(5 * time.Second)

	registry := component.NewRegistry()
	if err := registerComponentFactories(registry); err != nil {
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

// loadAndExpandConfig loads the semsource config and expands "repo" meta-sources
// into their component sources (git, ast, docs, config).
func loadAndExpandConfig(path string) (*config.Config, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load config %q: %w", path, err)
	}
	expanded, err := config.ExpandRepoSources(cfg.Sources, cfg.WorkspaceDir)
	if err != nil {
		return nil, fmt.Errorf("expand repo sources: %w", err)
	}
	cfg.Sources = expanded
	return cfg, nil
}

// registerComponentFactories registers all semstreams built-in and semsource
// component factories in the given registry.
func registerComponentFactories(registry *component.Registry) error {
	if err := componentregistry.Register(registry); err != nil {
		return fmt.Errorf("register semstreams components: %w", err)
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
// config. It translates each source entry into a component config and wires up
// the graph subsystem, manifest, and websocket components.
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

	graphComponents, err := graphSubsystemComponents()
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

	svcs, err := serviceConfigs(cfg)
	if err != nil {
		return nil, err
	}

	return &semconfig.Config{
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
		Streams: semconfig.StreamConfigs{
			"GRAPH": semconfig.StreamConfig{
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
			},
		},
	}, nil
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
func graphSubsystemComponents() (semconfig.ComponentConfigs, error) {
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
				"bind_address":      "0.0.0.0:8082",
				"enable_playground": true,
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
			Config:  json.RawMessage(`{"port": 9091, "path": "/metrics"}`),
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
	instanceName := fmt.Sprintf("ast-source-%s", slug)

	compCfg := map[string]any{
		"ports": sourceOutputPorts(),
		"watch_paths": []map[string]any{
			{
				"path":      path,
				"org":       org,
				"project":   slug,
				"languages": []string{lang},
			},
		},
		"watch_enabled":  src.Watch,
		"index_interval": "60s",
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
	instanceName := fmt.Sprintf("git-source-%s", slug)

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
	return fmt.Sprintf("doc-source-%s", slug), map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
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
	return fmt.Sprintf("cfgfile-source-%s", slug), map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
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
	return fmt.Sprintf("url-source-%s", slug), map[string]any{
		"ports":         sourceOutputPorts(),
		"org":           org,
		"urls":          urls,
		"poll_interval": "300s",
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
	}

	if cfg.MediaStoreDir != "" {
		compCfg["file_store_root"] = cfg.MediaStoreDir
	}

	return instanceName, compCfg
}
