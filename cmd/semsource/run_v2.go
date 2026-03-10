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

	// Register payloads in the semstreams payload registry.
	_ "github.com/c360studio/semsource/graph"
	_ "github.com/c360studio/semsource/processor/source-manifest"

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

// runV2Cmd is the new semstreams-native entry point for semsource.
// It replaces the engine-based runCmd for sources that have been migrated to
// semstreams processor components. Sources that have not yet been migrated
// log a warning and are skipped.
//
// Wire-up sequence mirrors the semspec bootstrap pattern:
//  1. Parse flags, load semsource config
//  2. Connect to NATS, ensure streams exist
//  3. Build semstreams config.Config (platform identity + component map)
//  4. Create component registry, register all factories
//  5. Create ServiceManager, configure, start
//  6. Block on SIGINT/SIGTERM, then shut down cleanly
func runV2Cmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	natsURL := fs.String("nats-url", "", "NATS server URL (overrides NATS_URL env and config)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := buildLogger(*logLevel)
	slog.SetDefault(logger)

	// Load semsource config.
	semsourceCfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}

	// Expand "repo" meta-sources into their component sources (git, ast, docs, config)
	// before anything else reads the source list.
	expandedSources, err := config.ExpandRepoSources(semsourceCfg.Sources, semsourceCfg.WorkspaceDir)
	if err != nil {
		return fmt.Errorf("expand repo sources: %w", err)
	}
	semsourceCfg.Sources = expandedSources

	logger.Info("semsource v2 starting",
		"version", version,
		"namespace", semsourceCfg.Namespace,
		"sources", len(semsourceCfg.Sources),
	)

	ctx := context.Background()

	// Connect to NATS.
	natsAddr := resolveNATSURL(*natsURL, semsourceCfg)
	nc, err := connectNATS(ctx, natsAddr, logger)
	if err != nil {
		return err
	}
	defer nc.Close(context.Background())

	// Build the semstreams config.Config that ServiceManager expects.
	// We derive it from the semsource config rather than loading a separate file.
	ssCfg, err := buildSemstreamsConfig(semsourceCfg, semsourceCfg.Namespace)
	if err != nil {
		return fmt.Errorf("build semstreams config: %w", err)
	}

	// Ensure required JetStream streams exist before starting components.
	streamsManager := semconfig.NewStreamsManager(nc, logger)
	if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {
		return fmt.Errorf("ensure JetStream streams: %w", err)
	}
	logger.Info("JetStream streams ready")

	// Config manager — required by ServiceManager for dynamic config watch.
	configMgr, err := semconfig.NewConfigManager(ssCfg, nc, logger)
	if err != nil {
		return fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return fmt.Errorf("start config manager: %w", err)
	}
	defer configMgr.Stop(5 * time.Second)

	// Component registry — holds all factories.
	componentRegistry := component.NewRegistry()

	logger.Debug("registering semstreams built-in component factories")
	if err := componentregistry.Register(componentRegistry); err != nil {
		return fmt.Errorf("register semstreams components: %w", err)
	}

	logger.Debug("registering semsource component factories")
	if err := astsource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register ast-source component: %w", err)
	}
	if err := gitsource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register git-source component: %w", err)
	}
	if err := docsource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register doc-source component: %w", err)
	}
	if err := cfgfilesource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register cfgfile-source component: %w", err)
	}
	if err := urlsource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register url-source component: %w", err)
	}
	if err := imagesource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register image-source component: %w", err)
	}
	if err := videosource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register video-source component: %w", err)
	}
	if err := audiosource.Register(componentRegistry); err != nil {
		return fmt.Errorf("register audio-source component: %w", err)
	}
	if err := filestore.Register(componentRegistry); err != nil {
		return fmt.Errorf("register filestore component: %w", err)
	}
	if err := sourcemanifest.Register(componentRegistry); err != nil {
		return fmt.Errorf("register source-manifest component: %w", err)
	}

	factories := componentRegistry.ListFactories()
	logger.Info("component factories registered", "count", len(factories))

	// Service infrastructure.
	metricsRegistry := metric.NewMetricsRegistry()
	platform := types.PlatformMeta{
		Org:      semsourceCfg.Namespace,
		Platform: "semsource",
	}

	serviceRegistry := service.NewServiceRegistry()
	if err := service.RegisterAll(serviceRegistry); err != nil {
		configMgr.Stop(5 * time.Second)
		return fmt.Errorf("register services: %w", err)
	}

	manager := service.NewServiceManager(serviceRegistry)
	svcDeps := &service.Dependencies{
		NATSClient:        nc,
		MetricsRegistry:   metricsRegistry,
		Logger:            logger,
		Platform:          platform,
		Manager:           configMgr,
		ComponentRegistry: componentRegistry,
	}

	if err := manager.ConfigureFromServices(ssCfg.Services, svcDeps); err != nil {
		return fmt.Errorf("configure service manager: %w", err)
	}

	// Create each configured service. ConfigureFromServices only configures the
	// service manager itself — individual services must be explicitly created.
	for name, svcConfig := range ssCfg.Services {
		if name == "service-manager" {
			continue
		}
		if !svcConfig.Enabled {
			logger.Info("service disabled, skipping", "name", name)
			continue
		}
		if _, err := manager.CreateService(name, svcConfig.Config, svcDeps); err != nil {
			return fmt.Errorf("create service %s: %w", name, err)
		}
		logger.Debug("created service", "name", name)
	}

	// Signal context for graceful shutdown.
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	logger.Info("semsource v2 running — waiting for shutdown signal")
	<-signalCtx.Done()
	logger.Info("shutdown signal received")

	shutdownTimeout := 30 * time.Second
	if err := manager.StopAll(shutdownTimeout); err != nil {
		logger.Error("error stopping services", "error", err)
		return fmt.Errorf("stop services: %w", err)
	}

	logger.Info("semsource v2 stopped")
	return nil
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
// config. It translates each AST source entry into an ast-source component config.
// Non-AST source types emit a warning and are skipped — they remain handled by
// the legacy engine path (runCmd) until their component migrations land.
func buildSemstreamsConfig(cfg *config.Config, org string) (*semconfig.Config, error) {
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
			slog.Debug("registered ast-source component instance",
				"instance", instanceName, "path", src.Path)

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
			slog.Debug("registered git-source component instance",
				"instance", instanceName, "url", src.URL, "path", src.Path)

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
			factoryName := src.Type + "-source"
			components[instanceName] = types.ComponentConfig{
				Name:    factoryName,
				Type:    types.ComponentTypeProcessor,
				Enabled: true,
				Config:  rawCfg,
			}

		default:
			slog.Warn("source type not yet migrated to component — skipped by v2 runner",
				"index", i, "type", src.Type, "path", src.Path)
		}
	}

	// --- Source manifest ---
	// Publishes the resolved source list to the GRAPH stream at startup and
	// serves on-demand queries via graph.query.sources.
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
	manifestCfg := map[string]any{
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
			},
		},
		"namespace": org,
		"sources":   manifestSources,
	}
	rawManifestCfg, err := json.Marshal(manifestCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal source-manifest config: %w", err)
	}
	components["source-manifest"] = types.ComponentConfig{
		Name:    "source-manifest",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  rawManifestCfg,
	}

	// --- Graph subsystem components ---
	// These are built-in semstreams components (registered by componentregistry.Register).
	// They form the read path: ingest → index → query → gateway.

	// graph-ingest: consumes entity payloads from the GRAPH stream, writes to
	// ENTITY_STATES KV bucket. Override default subject from entity.> to match
	// our publishing subject.
	graphIngestCfg := map[string]any{
		"ports": map[string]any{
			"inputs": []map[string]any{
				{
					"name":        "entity_stream",
					"type":        "jetstream",
					"subject":     "graph.ingest.entity",
					"stream_name": "GRAPH",
					"config": map[string]any{
						"deliver_policy": "all",
					},
				},
			},
			"outputs": []map[string]any{
				{
					"name":    "entity_states",
					"type":    "kv-write",
					"subject": "ENTITY_STATES",
				},
			},
		},
	}
	rawGraphIngestCfg, err := json.Marshal(graphIngestCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal graph-ingest config: %w", err)
	}
	components["graph-ingest"] = types.ComponentConfig{
		Name:    "graph-ingest",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  rawGraphIngestCfg,
	}

	// graph-index: watches ENTITY_STATES KV, maintains relationship indexes.
	graphIndexCfg := map[string]any{
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
	}
	rawGraphIndexCfg, err := json.Marshal(graphIndexCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal graph-index config: %w", err)
	}
	components["graph-index"] = types.ComponentConfig{
		Name:    "graph-index",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  rawGraphIndexCfg,
	}

	// graph-query: NATS request/reply coordinator for entity, relationship,
	// and path search queries.
	graphQueryCfg := map[string]any{
		"ports": map[string]any{
			"inputs": []map[string]any{
				{"name": "query_entity", "type": "nats-request", "subject": "graph.query.entity"},
				{"name": "query_relationships", "type": "nats-request", "subject": "graph.query.relationships"},
				{"name": "query_path_search", "type": "nats-request", "subject": "graph.query.pathSearch"},
			},
		},
	}
	rawGraphQueryCfg, err := json.Marshal(graphQueryCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal graph-query config: %w", err)
	}
	components["graph-query"] = types.ComponentConfig{
		Name:    "graph-query",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  rawGraphQueryCfg,
	}

	// graph-gateway: HTTP GraphQL endpoint for semstreams-ui.
	// Bind to 0.0.0.0 for Docker access, enable playground for dev.
	graphGatewayCfg := map[string]any{
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
	}
	rawGraphGatewayCfg, err := json.Marshal(graphGatewayCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal graph-gateway config: %w", err)
	}
	components["graph-gateway"] = types.ComponentConfig{
		Name:    "graph-gateway",
		Type:    types.ComponentTypeGateway,
		Enabled: true,
		Config:  rawGraphGatewayCfg,
	}

	// --- WebSocket output ---
	// Broadcasts graph entities to connected consumers (SemSpec, SemDragon).
	// Uses a JetStream consumer on the GRAPH stream for replay-on-reconnect
	// and ack-based backpressure. Serves on port 7890 at /graph.
	wsConfig := map[string]any{
		"ports": map[string]any{
			"inputs": []map[string]any{
				{
					"name":        "graph_entities",
					"type":        "jetstream",
					"subject":     "graph.ingest.entity",
					"stream_name": "GRAPH",
					"required":    true,
					"description": "Entity payloads from source components",
				},
			},
			"outputs": []map[string]any{
				{
					"name":        "websocket_server",
					"type":        "network",
					"subject":     "http://0.0.0.0:7890/graph",
					"description": "WebSocket server for downstream consumers",
				},
			},
		},
		"delivery_mode": "at-most-once",
	}
	rawWSCfg, err := json.Marshal(wsConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal websocket config: %w", err)
	}
	components["websocket-output"] = types.ComponentConfig{
		Name:    "websocket",
		Type:    types.ComponentTypeOutput,
		Enabled: true,
		Config:  rawWSCfg,
	}

	// Ensure the GRAPH stream is defined explicitly so EnsureStreams creates it.
	streams := semconfig.StreamConfigs{
		"GRAPH": semconfig.StreamConfig{
			Subjects: []string{"graph.ingest.entity", "graph.ingest.batch", "graph.ingest.manifest"},
			Storage:  "memory",
			MaxBytes: 256 * 1024 * 1024, // 256MB cap — prevents runaway memory if consumers lag
			MaxAge:   "1h",
			Replicas: 1,
		},
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
		Services: types.ServiceConfigs{
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
		},
		Components: components,
		Streams:    streams,
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

	// Derive a stable, NATS-safe instance name from the source path.
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
	// Derive a stable slug from URL or path.
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
		"ports":          sourceOutputPorts(),
		"org":            org,
		"repo_path":      src.Path,
		"repo_url":       src.URL,
		"branch":         branch,
		"poll_interval":  "60s",
		"watch_enabled":  src.Watch,
		"workspace_dir":  cfg.WorkspaceDir,
		"git_token":      cfg.GitToken,
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

	// Wire filestore root when configured — enables local binary content storage
	// in the handler in addition to metadata-only graph entity publication.
	if cfg.MediaStoreDir != "" {
		compCfg["file_store_root"] = cfg.MediaStoreDir
	}

	return instanceName, compCfg
}
