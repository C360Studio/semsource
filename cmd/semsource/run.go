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

	// Register language parsers and vocabulary in the semstreams registries.
	_ "github.com/c360studio/semsource/source/ast"

	// Register net/http/pprof handlers on http.DefaultServeMux; served only when
	// --pprof-port is set (service.MaybeStartPProf). Import does not arm it.
	_ "net/http/pprof"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	semgovernance "github.com/c360studio/semsource/internal/governance"
	"github.com/c360studio/semsource/internal/sourcespawn"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	audiosource "github.com/c360studio/semsource/processor/audio-source"
	cfgfilesource "github.com/c360studio/semsource/processor/cfgfile-source"
	codecontext "github.com/c360studio/semsource/processor/code-context"
	docsource "github.com/c360studio/semsource/processor/doc-source"
	gitsource "github.com/c360studio/semsource/processor/git-source"
	imagesource "github.com/c360studio/semsource/processor/image-source"
	mcpgateway "github.com/c360studio/semsource/processor/mcp-gateway"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semsource/processor/supersession"
	urlsource "github.com/c360studio/semsource/processor/url-source"
	videosource "github.com/c360studio/semsource/processor/video-source"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semsource/workspace"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
)

// runFlags holds the parsed `semsource run` command-line flags.
type runFlags struct {
	configPath          string
	logLevel            string
	natsURL             string
	natsDisconnectGrace time.Duration
	pprofPort           int
}

// parseRunFlags parses the `semsource run` flag set.
func parseRunFlags(args []string) (*runFlags, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	natsURL := fs.String("nats-url", "", "NATS server URL (overrides NATS_URL env and config)")
	natsDisconnectGrace := fs.Duration("nats-disconnect-grace", resolveNATSDisconnectGrace(),
		"shut down if NATS is disconnected continuously beyond this duration; 0 disables the watchdog")
	pprofPort := fs.Int("pprof-port", 0,
		"if >0, serve net/http/pprof (/debug/pprof) on this port for profiling")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &runFlags{
		configPath:          *configPath,
		logLevel:            *logLevel,
		natsURL:             *natsURL,
		natsDisconnectGrace: *natsDisconnectGrace,
		pprofPort:           *pprofPort,
	}, nil
}

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
	flags, err := parseRunFlags(args)
	if err != nil {
		return err
	}

	logger := buildLogger(flags.logLevel)
	slog.SetDefault(logger)

	// Arm pprof early (before NATS) so a slow/wedged boot stays profilable. No-op
	// unless --pprof-port > 0. Handlers come from the blank net/http/pprof import.
	service.MaybeStartPProf(flags.pprofPort > 0, flags.pprofPort)

	ctx := context.Background()

	semsourceCfg, expandResult, err := loadAndExpandConfig(ctx, flags.configPath)
	if err != nil {
		return err
	}

	logger.Info("semsource starting",
		"version", resolveVersion(),
		"namespace", semsourceCfg.Namespace,
		"sources", len(semsourceCfg.Sources),
		"branch_watchers", len(expandResult.Watchers),
	)

	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	// Hand the natsclient an "exit if the broker stays gone" policy: when
	// the connection has been continuously down for the configured grace,
	// trigger graceful shutdown so the supervisor can restart us with a
	// fresh state instead of letting downstream components hot-loop.
	onConnectionLost := func(err error) {
		logger.Error("NATS broker unreachable beyond grace period — initiating shutdown",
			"grace", flags.natsDisconnectGrace,
			"error", err,
		)
		signalCancel()
	}
	nc, ssCfg, err := setupNATS(ctx, flags.natsURL, semsourceCfg, logger,
		flags.natsDisconnectGrace, onConnectionLost)
	if err != nil {
		return err
	}
	defer nc.Close(context.Background())

	governanceBoot, err := semgovernance.BootstrapStandalone(signalCtx, nc, logger)
	if err != nil {
		return err
	}

	registry := component.NewRegistry()
	if err := registerComponentFactories(registry); err != nil {
		return err
	}

	payloadReg, err := buildPayloadRegistry()
	if err != nil {
		return err
	}

	configMgr, err := semconfig.NewConfigManager(ssCfg, nc, logger)
	if err != nil {
		return fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return fmt.Errorf("start config manager: %w", err)
	}
	defer configMgr.Stop(5 * time.Second)

	manager, err := createServiceManager(semsourceCfg, ssCfg, nc, registry, payloadReg, configMgr, governanceBoot, logger)
	if err != nil {
		configMgr.Stop(5 * time.Second)
		return err
	}

	return serveUntilSignal(signalCtx, manager, configMgr, semsourceCfg, expandResult, logger)
}

// serveUntilSignal starts all services, wires runtime ingest handlers and
// branch watchers, then blocks until signalCtx is cancelled (shutdown signal
// or broker-loss watchdog) and drains the manager. The caller's deferred
// cleanups (config manager + NATS) run after this returns.
func serveUntilSignal(
	signalCtx context.Context,
	manager *service.Manager,
	configMgr *semconfig.Manager,
	cfg *config.Config,
	expandResult *config.ExpandResult,
	logger *slog.Logger,
) error {
	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	if err := registerIngestHandlers(signalCtx, manager, configMgr, cfg, logger); err != nil {
		logger.Warn("failed to register ingest handlers", "error", err)
	}

	startBranchWatchers(signalCtx, expandResult.Watchers, cfg, configMgr, logger)

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

// setupNATS connects to NATS and ensures JetStream streams exist. The
// connection-loss watchdog (grace + callback) is wired into the natsclient
// so the caller is signalled once the broker has been continuously absent
// for the configured grace.
func setupNATS(
	ctx context.Context,
	natsFlag string,
	cfg *config.Config,
	logger *slog.Logger,
	connectionLossGrace time.Duration,
	onConnectionLost func(error),
) (*natsclient.Client, *semconfig.Config, error) {
	natsAddr := resolveNATSURL(natsFlag, cfg)
	nc, err := connectNATS(ctx, natsAddr, logger, connectionLossGrace, onConnectionLost)
	if err != nil {
		return nil, nil, err
	}

	ssCfg, err := buildSemstreamsConfig(cfg, cfg.Namespace)
	if err != nil {
		nc.Close(context.Background())
		return nil, nil, fmt.Errorf("build semstreams config: %w", err)
	}

	streamsManager := semconfig.NewStreamsManager(nc, logger)
	if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {
		nc.Close(context.Background())
		return nil, nil, fmt.Errorf("ensure JetStream streams: %w", err)
	}
	logger.Info("JetStream streams ready")

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

// registerComponentFactories registers the semstreams built-in factories
// (graph, agentic, etc.) alongside semsource's own factories. semsource runs as
// a standalone service that owns its full component set.
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

// buildPayloadRegistry constructs the payload registry, registers semstreams
// first-party builtins, and layers semsource's own payload types on top.
// The result is plumbed through service.Dependencies.PayloadRegistry so every
// component receives the same registry via component.Dependencies.
func buildPayloadRegistry() (*payloadregistry.Registry, error) {
	reg := payloadregistry.New()
	if err := payloadbuiltins.Register(reg); err != nil {
		return nil, fmt.Errorf("register builtin payloads: %w", err)
	}
	if err := graph.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register graph payloads: %w", err)
	}
	if err := sourcemanifest.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register source-manifest payloads: %w", err)
	}
	return reg, nil
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
		"code-context":    func() error { return codecontext.Register(registry) },
		"mcp-gateway":     func() error { return mcpgateway.Register(registry) },
		"supersession":    func() error { return supersession.Register(registry) },
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
	payloadReg *payloadregistry.Registry,
	configMgr *semconfig.Manager,
	governanceBoot *semgovernance.Bootstrap,
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
	if governanceBoot != nil {
		manager.RegisterInstance("ownership",
			service.NewOwnershipService(governanceBoot.Registry, governanceBoot.Heartbeater, metricsRegistry, logger))
	}

	deps := &service.Dependencies{
		NATSClient:        nc,
		MetricsRegistry:   metricsRegistry,
		Logger:            logger,
		Platform:          platform,
		Manager:           configMgr,
		ComponentRegistry: registry,
		PayloadRegistry:   payloadReg,
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

// defaultNATSDisconnectGrace is the default window we tolerate a NATS broker
// outage before initiating a graceful shutdown. Long enough to ride through
// a typical broker restart or short network blip; short enough that the
// process supervisor can take over before logs and CPU spend get out of hand.
const defaultNATSDisconnectGrace = 2 * time.Minute

// resolveNATSDisconnectGrace returns the watchdog grace, falling back to the
// SEMSOURCE_NATS_DISCONNECT_GRACE env var, then defaultNATSDisconnectGrace.
// A zero or negative value disables the watchdog.
func resolveNATSDisconnectGrace() time.Duration {
	if v := os.Getenv("SEMSOURCE_NATS_DISCONNECT_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultNATSDisconnectGrace
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
// When connectionLossGrace is positive and onConnectionLost is non-nil, the
// natsclient fires the callback once the broker has been continuously
// unreachable for at least the grace duration.
func connectNATS(
	ctx context.Context,
	url string,
	logger *slog.Logger,
	connectionLossGrace time.Duration,
	onConnectionLost func(error),
) (*natsclient.Client, error) {
	logger.Info("connecting to NATS", "url", url)

	opts := []natsclient.ClientOption{
		natsclient.WithName("semsource"),
		natsclient.WithMaxReconnects(-1),
		natsclient.WithReconnectWait(2 * time.Second),
		natsclient.WithHealthInterval(30 * time.Second),
	}
	if connectionLossGrace > 0 && onConnectionLost != nil {
		opts = append(opts,
			natsclient.WithConnectionLossTimeout(connectionLossGrace),
			natsclient.WithConnectionLostCallback(onConnectionLost),
		)
	}

	nc, err := natsclient.NewClient(url, opts...)
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
		return fmt.Errorf("NATS connection failed at %s: %w\n\n"+
			"SemSource needs a NATS server (JetStream + KV). Start one with:\n"+
			"  docker run --rm -p 4222:4222 nats:2-alpine -js\n"+
			"or, from the semsource repo:\n"+
			"  docker compose up -d nats\n"+
			"Then re-run, or set --nats-url / NATS_URL to an existing server.", url, err)
	}
	return fmt.Errorf("NATS connection failed: %w", err)
}

// buildSemstreamsConfig constructs a semstreams config.Config from the semsource
// config: source components, the manifest, the full graph subsystem, the
// WebSocket output, and the fusion gateways.
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

	// Graph subsystem + WebSocket output + the fusion gateways. The gateways
	// query graph.query.* / graph.index.query.*, which the graph subsystem here
	// serves; semsource owns the full set as a standalone external service.
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

	// code-context / doc-context fusion gateways (ADR-0004). Two instances of
	// one factory; the map key is the instance name (→ HTTP prefix), the
	// Config.Lens selects the lens, and the NATS subject root differs by lens.
	for instance, lens := range map[string]string{"code-context": "code", "doc-context": "docs"} {
		gwCfg, err := codeContextComponentConfig(lens)
		if err != nil {
			return nil, err
		}
		components[instance] = gwCfg
	}

	// MCP gateway (ADR-0007 §1): source-registration tools over Streamable HTTP.
	mcpCfg, err := mcpGatewayComponentConfig(cfg)
	if err != nil {
		return nil, err
	}
	components["mcp-gateway"] = mcpCfg

	// supersession (ADR-0008): serves the version-diff query (graph.query.versionDiff,
	// behind the code_changes MCP tool) and the correspondence/lineage pass. It must
	// be in the default set — otherwise the query is unserved and code_changes times
	// out. The lineage/demotion pass itself stays trigger-driven (graph.supersession.run
	// or an operator-set interval), so spawning it adds no periodic background load.
	supersessionCfg, err := supersessionComponentConfig()
	if err != nil {
		return nil, err
	}
	components["supersession"] = supersessionCfg

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
		// Tier 1/2 enablement: passed straight through so the ComponentManager
		// injects it into deps.ModelRegistry (verified: component_manager.go
		// buildComponentDependencies). graph-embedding's HTTP embedder resolves
		// "embedding" (→ semembed); graph-clustering's summarizer resolves
		// "community_summary" (→ seminstruct). Nil = tier 0 (BM25).
		ModelRegistry: cfg.ModelRegistry,
	}

	// Declare the full GRAPH stream with user overrides.
	ssCfg.Streams = graphStreamConfig(cfg)

	return ssCfg, nil
}

// sourceComponents translates semsource config sources into semstreams component configs.
func sourceComponents(cfg *config.Config, org string) (semconfig.ComponentConfigs, error) {
	components := make(semconfig.ComponentConfigs)
	opts := sourcespawn.Options{
		Org:           org,
		WorkspaceDir:  cfg.WorkspaceDir,
		GitToken:      cfg.GitToken,
		MediaStoreDir: cfg.MediaStoreDir,
	}

	for i, src := range cfg.Sources {
		built, err := sourcespawn.Build(src, opts)
		if err != nil {
			if sourcespawn.CodeOf(err) == sourcespawn.CodeUnsupportedType {
				slog.Warn("source type not yet migrated to component — skipped",
					"index", i, "type", src.Type, "path", src.Path)
				continue
			}
			return nil, fmt.Errorf("source[%d] (%s): %w", i, src.Type, err)
		}
		for name, compCfg := range built {
			components[name] = compCfg
		}
	}

	return components, nil
}

// codeContextComponentConfig builds a code-context fusion-gateway component
// config for the given lens ("code" or "docs").
func codeContextComponentConfig(lens string) (types.ComponentConfig, error) {
	raw, err := json.Marshal(map[string]any{"lens": lens})
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("marshal code-context config: %w", err)
	}
	return types.ComponentConfig{
		Name:    "code-context",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}, nil
}

// supersessionComponentConfig builds the supersession component config with its
// defaults (on-demand trigger, no periodic pass). The empty JSON object unmarshals
// onto DefaultConfig, so the component serves graph.query.versionDiff and the
// graph.supersession.run trigger; an operator can enable a periodic lineage pass
// via the component's `interval` config.
func supersessionComponentConfig() (types.ComponentConfig, error) {
	raw, err := json.Marshal(map[string]any{})
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("marshal supersession config: %w", err)
	}
	return types.ComponentConfig{
		Name:    "supersession",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}, nil
}

// mcpGatewayComponentConfig builds the mcp-gateway component config. The bearer
// token is NOT set here — the component reads SEMSOURCE_API_TOKEN from the
// environment so the secret never lands in the config KV.
func mcpGatewayComponentConfig(cfg *config.Config) (types.ComponentConfig, error) {
	raw, err := json.Marshal(map[string]any{
		"namespace":     cfg.Namespace,
		"mcp_path":      "/mcp",
		"allowed_roots": cfg.SourceRoots,
		"instance_name": "mcp-gateway",
	})
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("marshal mcp-gateway config: %w", err)
	}
	return types.ComponentConfig{
		Name:    "mcp-gateway",
		Type:    types.ComponentTypeProcessor,
		Enabled: true,
		Config:  raw,
	}, nil
}

// manifestComponentConfig builds the source-manifest component config.
func manifestComponentConfig(cfg *config.Config, org string, sourceCount int) (types.ComponentConfig, error) {
	manifestSources := make([]sourcemanifest.ManifestSource, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		manifestSources = append(manifestSources, sourcemanifest.ManifestSource{
			Type:          src.Type,
			Path:          src.Path,
			Paths:         src.Paths,
			URL:           src.URL,
			URLs:          src.URLs,
			Language:      src.Language,
			Branch:        src.Branch,
			Watch:         src.Watch,
			PollInterval:  src.PollInterval,
			IndexInterval: src.IndexInterval,
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

func graphQueryInputPorts() []map[string]any {
	return []map[string]any{
		{"name": "query_entity", "type": "nats-request", "subject": "graph.query.entity"},
		{"name": "query_entity_by_alias", "type": "nats-request", "subject": "graph.query.entityByAlias"},
		{"name": "query_batch", "type": "nats-request", "subject": "graph.query.batch"},
		{"name": "query_relationships", "type": "nats-request", "subject": "graph.query.relationships"},
		{"name": "query_path_search", "type": "nats-request", "subject": "graph.query.pathSearch"},
		{"name": "query_hierarchy_stats", "type": "nats-request", "subject": "graph.query.hierarchyStats"},
		{"name": "query_prefix", "type": "nats-request", "subject": "graph.query.prefix"},
		{"name": "query_spatial", "type": "nats-request", "subject": "graph.query.spatial"},
		{"name": "query_temporal", "type": "nats-request", "subject": "graph.query.temporal"},
		{"name": "query_semantic", "type": "nats-request", "subject": "graph.query.semantic"},
		{"name": "query_similar", "type": "nats-request", "subject": "graph.query.similar"},
		{"name": "local_search", "type": "nats-request", "subject": "graph.query.localSearch"},
		{"name": "global_search", "type": "nats-request", "subject": "graph.query.globalSearch"},
		{"name": "query_summary", "type": "nats-request", "subject": "graph.query.summary"},
		{"name": "query_search_graph", "type": "nats-request", "subject": "graph.query.searchGraph"},
	}
}

func graphGatewayOutputPorts() []map[string]any {
	return []map[string]any{
		{"name": "queries", "type": "nats-request", "subject": "graph.query.*"},
		{"name": "mutations", "type": "nats-request", "subject": "graph.mutation.*"},
	}
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
	indexWorkers := 0 // 0 → semstreams graph-index default (1)
	enableClustering := false
	clusteringLLM := false

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
		if g.IndexWorkers > 0 {
			indexWorkers = g.IndexWorkers
		}
		enableClustering = g.EnableClustering
		clusteringLLM = g.EnableClustering && g.ClusteringLLM
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
				"enforce_owner_lease": false,
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
				// workers=0 → graph-index applies its own default (1); raise via
				// graph.index_workers to parallelize bulk index builds.
				"workers": indexWorkers,
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
				// graph-embedding embeds a triple's value only when its predicate
				// ends with a text_suffix. A non-empty list REPLACES the built-in
				// defaults, so we restate them and add the code doc predicates
				// (code.doc.signature, code.doc.comment) — otherwise code entities
				// embed dc.terms.title (the symbol name) ALONE, and signatures /
				// docstrings never enter the semantic index.
				"text_suffixes": []string{
					".title", ".content", ".description", ".summary", ".text",
					".name", ".body", ".abstract", ".subject",
					".signature", ".comment",
				},
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
					"inputs": graphQueryInputPorts(),
				},
			},
		},
		"graph-gateway": {
			name:     "graph-gateway",
			compType: types.ComponentTypeGateway,
			configMap: map[string]any{
				"ports": map[string]any{
					"inputs": []map[string]any{
						// Subject must encode host:port: the registry parses the
						// gateway's network port from it (parsePortFromSubject), so a
						// path like "/graphql" yields port 0 and registration fails.
						// The GraphQL route is GraphQLPath (default /graphql), served on
						// ServiceManager's central mux — independent of this subject.
						{"name": "http", "type": "http", "subject": gatewayBind},
					},
					"outputs": graphGatewayOutputPorts(),
				},
				"bind_address":      gatewayBind,
				"enable_playground": enablePlayground,
			},
		},
		// objectstore backs location-independent verbatim content (code/text
		// bodies) addressed via message.StorageReference and dereferenced over
		// storage.objectstore.api (ADR-0006 / semstreams#376). Default ports
		// apply (write/api/events/stored); the producer + Lens.Hydrate→StorageRef
		// wiring lands once the upstream body/key convention is settled. Large
		// binaries (media) deliberately stay on the local filestore, not here.
		"objectstore": {
			name:     "objectstore",
			compType: types.ComponentTypeStorage,
			configMap: map[string]any{
				"bucket_name": "CONTENT",
			},
		},
	}

	// graph-clustering (tier 2, opt-in via graph.enable_clustering): LPA
	// community detection over ENTITY_STATES → COMMUNITY_INDEX, feeding the
	// already-declared local/global/summary query routes. Pure-Go LPA runs with
	// no external service; clustering_llm adds GraphRAG summaries via the
	// model_registry "community_summary" capability (→ seminstruct).
	if enableClustering {
		configs["graph-clustering"] = struct {
			name      string
			compType  types.ComponentType
			configMap map[string]any
		}{
			name:     "graph-clustering",
			compType: types.ComponentTypeProcessor,
			configMap: map[string]any{
				"detection_interval": "30s",
				"enable_llm":         clusteringLLM,
				"ports": map[string]any{
					"inputs": []map[string]any{
						{"name": "entity_watch", "type": "kv-watch", "subject": "ENTITY_STATES"},
					},
					"outputs": []map[string]any{
						{"name": "communities", "type": "kv-write", "subject": "COMMUNITY_INDEX"},
					},
				},
			},
		}
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
			// watch_config:true wires the ComponentManager to the
			// ConfigManager's KV watcher. Without it, runtime writes via
			// graph.ingest.add (sourcespawn.Add → PutComponentToKV) and the
			// branch watcher land in KV but never trigger component spawn —
			// only the boot-time snapshot is loaded.
			Config: json.RawMessage(`{"watch_config":true}`),
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

// registerIngestHandlers locates the running source-manifest component and
// wires graph.ingest.add.{namespace} and graph.ingest.remove.{namespace}
// onto it. The component owns the subscription lifecycle — Stop tears
// these down alongside its existing query subs.
func registerIngestHandlers(
	ctx context.Context,
	manager *service.Manager,
	configMgr *semconfig.Manager,
	cfg *config.Config,
	logger *slog.Logger,
) error {
	cmService, ok := manager.GetService("component-manager")
	if !ok {
		return fmt.Errorf("component-manager service not running")
	}
	cm, ok := cmService.(*service.ComponentManager)
	if !ok {
		return fmt.Errorf("component-manager has unexpected type %T", cmService)
	}

	ingestCfg := sourcemanifest.IngestHandlerConfig{
		Namespace: cfg.Namespace,
		Store:     configMgr,
		Spawn: sourcespawn.Options{
			Org:           cfg.Namespace,
			WorkspaceDir:  cfg.WorkspaceDir,
			GitToken:      cfg.GitToken,
			MediaStoreDir: cfg.MediaStoreDir,
		},
		// HTTP façade guards (ADR-0007): optional bearer token (permissive when
		// unset) and the filesystem-root allowlist for path-based HTTP adds.
		APIToken:     os.Getenv("SEMSOURCE_API_TOKEN"),
		AllowedRoots: cfg.SourceRoots,
	}

	// source-manifest's Start (which flips its running flag) can lag StartAll's
	// return, so poll until it is both managed AND started before wiring the
	// curator ingest handlers. A one-shot registration loses that race —
	// RegisterIngestHandlers returns "component not started" and the
	// graph.ingest.add/remove subjects would then never get handlers (runtime
	// source-add silently no-ops). Bounded so a genuinely-absent component
	// surfaces an error instead of blocking forever.
	const (
		readyDeadline = 10 * time.Second
		pollInterval  = 100 * time.Millisecond
	)
	deadline := time.Now().Add(readyDeadline)
	lastErr := fmt.Errorf("source-manifest not yet managed")
	for {
		if mc, ok := cm.GetManagedComponents()["source-manifest"]; ok {
			smComponent, ok := mc.Component.(*sourcemanifest.Component)
			if !ok {
				return fmt.Errorf("source-manifest has unexpected type %T", mc.Component)
			}
			err := smComponent.RegisterIngestHandlers(ctx, ingestCfg)
			if err == nil {
				logger.Debug("source-manifest ingest handlers registered")
				return nil
			}
			// "component not started" is the start-race — retry (it returns
			// before any subscription, so retrying can't double-register). Any
			// other error is terminal.
			if !strings.Contains(err.Error(), "component not started") {
				return err
			}
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("source-manifest ingest handlers not registered within %s: %w", readyDeadline, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
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
	opts := sourcespawn.Options{
		Org:           cfg.Namespace,
		WorkspaceDir:  cfg.WorkspaceDir,
		GitToken:      cfg.GitToken,
		MediaStoreDir: cfg.MediaStoreDir,
	}
	// A discovered branch expands to a "repo" entry pinned to the worktree;
	// sourcespawn handles the per-branch git/ast/docs/config split.
	repoEntry := config.SourceEntry{
		Type:       "repo",
		Path:       bs.WorktreePath,
		Branch:     bs.Branch,
		BranchSlug: bs.Slug,
		Watch:      ref.Watch,
		Language:   ref.Language,
	}
	componentConfigs, err := sourcespawn.Build(repoEntry, opts)
	if err != nil {
		logger.Warn("failed to build branch component configs",
			"branch", bs.Branch,
			"error", err)
		return
	}
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
