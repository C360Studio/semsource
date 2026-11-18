package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	// Register language parsers and vocabulary in the semstreams registries.
	_ "github.com/c360studio/semsource/source/ast"

	// Register net/http/pprof handlers on http.DefaultServeMux; served only when
	// --pprof-port is set (service.MaybeStartPProf). Import does not arm it.
	_ "net/http/pprof"

	"github.com/c360studio/semengine/component"
	semconfig "github.com/c360studio/semengine/config"
	graphgateway "github.com/c360studio/semengine/gateway/graph-gateway"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/output/websocket"
	"github.com/c360studio/semengine/payloadregistry"
	graphclustering "github.com/c360studio/semengine/processor/graph-clustering"
	graphindex "github.com/c360studio/semengine/processor/graph-index"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/service"
	"github.com/c360studio/semengine/storage/objectstore"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
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
	objectstoresource "github.com/c360studio/semsource/processor/objectstore-source"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semsource/processor/supersession"
	urlsource "github.com/c360studio/semsource/processor/url-source"
	videosource "github.com/c360studio/semsource/processor/video-source"
	"github.com/c360studio/semsource/storage/filestore"
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
func runCmd(args []string) (runErr error) {
	flags, err := parseRunFlags(args)
	if err != nil {
		return err
	}

	logger := buildLogger(flags.logLevel)
	slog.SetDefault(logger)

	// Arm pprof early (before NATS) so a slow/wedged boot stays profilable. No-op
	// unless --pprof-port > 0. Handlers come from the blank net/http/pprof import.
	service.MaybeStartPProf(flags.pprofPort > 0, flags.pprofPort, logger)

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
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, nc.Close(closeCtx))
	}()

	// Fail-fast projection-intent validation; the contract itself is consumed
	// where mutations are sent (supersession's typed client), not here.
	if _, err := semgovernance.BootstrapStandalone(logger); err != nil {
		return err
	}

	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, configMgr.Stop(stopCtx))
	}()

	// Discovered branches and submodules enter this boot's composition here,
	// after retained arbitration and before the service manager snapshots it.
	watches, err := admitDiscovered(ctx, semsourceCfg, expandResult, ssCfg.Components, configMgr, logger)
	if err != nil {
		return err
	}

	registry, err := prepareComponentRegistry(ctx, semsourceCfg, configMgr, nc, watches...)
	if err != nil {
		return err
	}

	payloadReg, err := buildPayloadRegistry()
	if err != nil {
		return err
	}

	manager, err := createServiceManager(nc, registry, payloadReg, configMgr, logger)
	if err != nil {
		return err
	}

	return serveUntilSignal(signalCtx, manager, logger)
}

// serveUntilSignal starts all services, then blocks until signalCtx is
// cancelled (shutdown signal or broker-loss watchdog) and drains the manager.
// Discovery happened at boot (admitDiscovered); nothing is admitted here. The
// caller's deferred cleanups (config manager + NATS) run after this returns.
func serveUntilSignal(signalCtx context.Context, manager *service.Manager, logger *slog.Logger) error {
	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	logger.Info("semsource running — waiting for shutdown signal")
	<-signalCtx.Done()
	logger.Info("shutdown signal received")

	// The signal is cancelled; the runtime stays live while shutdown uses bounded
	// authority (semengine ADR-113 caller-owned lifecycle contract). Background is
	// allowed only here, at the process composition root.
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStop()
	if err := manager.StopAll(stopCtx); err != nil {
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

	if err := checkLegacyLifecycle(ctx, nc); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return nil, nil, errors.Join(err, nc.Close(closeCtx))
	}

	if err := checkLegacySemStreamsConfig(ctx, nc, cfg.Namespace, cfg.PlatformStem()); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return nil, nil, errors.Join(err, nc.Close(closeCtx))
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

// componentWiring carries the run-time inputs semsource's own factories close
// over.
type componentWiring struct {
	Ingest       *sourcemanifest.IngestHandlerConfig
	ScopeSystems map[string][]string
}

// registerComponentFactories registers exactly the semstreams factories that
// buildSemstreamsConfig composes, then semsource's own (semengine#3: explicit
// per-package registration at the composition root, no aggregator). A config
// naming any other factory fails boot in requireRegisteredFactories.
func registerComponentFactories(registry *component.Registry, wiring ...componentWiring) error {
	for name, register := range map[string]func(*component.Registry) error{
		"graph-ingest":     graphingest.Register,
		"graph-index":      graphindex.Register,
		"graph-lexical":    graphlexical.Register,
		"graph-query":      graphquery.Register,
		"graph-gateway":    graphgateway.Register,
		"graph-clustering": graphclustering.Register,
		"objectstore":      objectstore.Register,
		"websocket":        websocket.Register,
	} {
		if err := register(registry); err != nil {
			return fmt.Errorf("register %s factory: %w", name, err)
		}
	}
	if err := registerSemsourceFactories(registry, wiring...); err != nil {
		return err
	}
	slog.Info("component factories registered", "count", len(registry.ListFactories()))
	return nil
}

// buildPayloadRegistry constructs the payload registry with the semstreams
// payloads the composed components carry (core message types, objectstore's
// stored message, graph-ingest's hierarchy container — which graph-ingest
// requires at construction), then layers semsource's own payload types on top.
// The result is plumbed through service.Dependencies.PayloadRegistry so every
// component receives the same registry via component.Dependencies.
func buildPayloadRegistry() (*payloadregistry.Registry, error) {
	reg := payloadregistry.New()
	if err := message.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register message payloads: %w", err)
	}
	if err := objectstore.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register objectstore payloads: %w", err)
	}
	if err := inference.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register inference payloads: %w", err)
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
func registerSemsourceFactories(registry *component.Registry, wiring ...componentWiring) error {
	var ingest []sourcemanifest.IngestHandlerConfig
	var scopes map[string][]string
	if len(wiring) > 0 {
		scopes = wiring[0].ScopeSystems
		if wiring[0].Ingest != nil {
			ingest = append(ingest, *wiring[0].Ingest)
		}
	}
	for name, fn := range map[string]func() error{
		"ast-source":     func() error { return astsource.Register(registry) },
		"git-source":     func() error { return gitsource.Register(registry) },
		"doc-source":     func() error { return docsource.Register(registry) },
		"cfgfile-source": func() error { return cfgfilesource.Register(registry) },
		"url-source":     func() error { return urlsource.Register(registry) },
		// No entry in buildPayloadRegistry: object-store artifacts become
		// ordinary document entities through the doc pipeline, so this source
		// introduces no payload type of its own.
		"objectstore-source": func() error { return objectstoresource.Register(registry) },
		"image-source":       func() error { return imagesource.Register(registry) },
		"video-source":       func() error { return videosource.Register(registry) },
		"audio-source":       func() error { return audiosource.Register(registry) },
		"filestore":          func() error { return filestore.Register(registry) },
		"source-manifest":    func() error { return sourcemanifest.Register(registry, ingest...) },
		"code-context":       func() error { return codecontext.Register(registry, scopes) },
		"mcp-gateway":        func() error { return mcpgateway.Register(registry) },
		"supersession":       func() error { return supersession.Register(registry) },
	} {
		if err := fn(); err != nil {
			return fmt.Errorf("register %s component: %w", name, err)
		}
	}
	return nil
}

// createServiceManager builds the ServiceManager with all configured services.
func createServiceManager(
	nc *natsclient.Client,
	registry *component.Registry,
	payloadReg *payloadregistry.Registry,
	configMgr *semconfig.Manager,
	logger *slog.Logger,
) (*service.Manager, error) {
	metricsRegistry := metric.NewMetricsRegistry()
	effective := configMgr.GetConfig().Get()
	platform := types.PlatformMeta{Org: effective.Platform.Org, Platform: effective.Platform.ID}

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
		PayloadRegistry:   payloadReg,
	}

	// ConfigureFromServices constructs every enabled configured service
	// itself on beta.160 — the composition has ONE writer. A second
	// CreateService pass here would be a duplicate composition writer and
	// fails with DuplicateServiceError.
	if err := manager.ConfigureFromServices(effective.Services, deps); err != nil {
		return nil, fmt.Errorf("configure service manager: %w", err)
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
			"  docker run --rm -p 4222:4222 nats:2.12-alpine -js\n"+
			"or, from the semsource repo:\n"+
			"  docker compose up -d nats\n"+
			"Then re-run, or set --nats-url / NATS_URL to an existing server.", url, err)
	}
	return fmt.Errorf("NATS connection failed: %w", err)
}

// compositionGeneration is the major position of the composition version. A
// retained store keeps its configuration until the file's version is newer,
// so a release that changes what buildSemstreamsConfig generates must bump
// this, or deployments keep running the previous wiring; the fingerprint test
// fails until it does. 2 was the beta.160 foundation cutover (typed ports,
// defaults-deferral, no ownership), 3 the SETUP 03A lifecycle baseline, 4 the
// alpha.5 move from graph-embedding to graph-lexical, 5 the removal of
// git_token from git-source configurations, which rewrites retained keys
// without the token (#272).
const compositionGeneration = 5

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
		gwCfg, err := codeContextComponentConfig(cfg, lens)
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
		// KV sync control: an equal-or-older version never replaces the
		// configuration a retained store already holds. The binary and the
		// operator each own one position, so either can make the file newest:
		// the generation for the generated wiring, config_revision for edits.
		Version: fmt.Sprintf("%d.%d.0", compositionGeneration, cfg.ConfigRevision),
		Platform: semconfig.PlatformConfig{
			Org:         org,
			ID:          cfg.PlatformStem(),
			Environment: "dev",
		},
		NATS: semconfig.NATSConfig{
			JetStream: semconfig.JetStreamConfig{Enabled: true},
		},
		Services:   svcs,
		Components: components,
		// Passed straight through so the ComponentManager injects it into
		// deps.ModelRegistry. Nothing at tier 0 resolves from it; the tier 1
		// embedder that will is #234.
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
func codeContextComponentConfig(cfg *config.Config, lens string) (types.ComponentConfig, error) {
	cfgMap := map[string]any{"lens": lens}
	if !cfg.Graph.LexicalSearchEnabled() {
		cfgMap["semantic_enabled"] = false
	}
	raw, err := json.Marshal(cfgMap)
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
	cfgMap := map[string]any{
		"namespace":     cfg.Namespace,
		"mcp_path":      "/mcp",
		"allowed_roots": cfg.SourceRoots,
		"instance_name": "mcp-gateway",
	}
	if !cfg.Graph.LexicalSearchEnabled() {
		cfgMap["semantic_enabled"] = false
	}
	raw, err := json.Marshal(cfgMap)
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
		manifestSources = append(manifestSources, sourcemanifest.ManifestSourceFromEntry(src))
	}
	cfgMap := map[string]any{
		"ports": map[string]any{
			"outputs": []component.PortDefinition{
				{
					Name:        "graph.ingest",
					Required:    true,
					Description: "Source manifest broadcast for downstream consumers",
					Config:      component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.manifest"}},
				},
				{
					Name:        "graph.ingest.status",
					Description: "Ingestion status broadcast for downstream consumers",
					Config:      component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.status"}},
				},
				{
					Name:        "graph.ingest.predicates",
					Description: "Predicate schema broadcast for downstream consumers",
					Config:      component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.predicates"}},
				},
			},
		},
		"namespace":             org,
		"sources":               manifestSources,
		"expected_source_count": sourceCount,
	}
	if !cfg.Graph.LexicalSearchEnabled() {
		cfgMap["semantic_enabled"] = false
	}
	raw, err := json.Marshal(cfgMap)
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

// graphTextSuffixes is the predicate-suffix list graph-lexical and
// graph-clustering read an entity's text from. A non-empty list replaces
// SemEngine's defaults, so it restates them and adds the code doc predicates
// (code.doc.signature, code.doc.comment): without them a code entity's text is
// its name alone. Clustering keywords use the same list so they and word
// search select the same values.
func graphTextSuffixes() []string {
	return append(entitytext.DefaultSuffixes(), ".signature", ".comment")
}

// graphSubsystemComponents returns the built-in semengine graph components:
// ingest, index, query, and gateway, plus graph-lexical unless
// graph.lexical_search is false and graph-clustering when enabled.
func graphSubsystemComponents(cfg *config.Config) (semconfig.ComponentConfigs, error) {
	// Resolve graph config with defaults.
	coalesceMs := 200
	indexWorkers := 0 // 0 → semengine graph-index default (1)
	enableClustering := false

	if g := cfg.Graph; g != nil {
		if g.CoalesceMs > 0 {
			coalesceMs = g.CoalesceMs
		}
		if g.IndexWorkers > 0 {
			indexWorkers = g.IndexWorkers
		}
		enableClustering = g.EnableClustering
	}

	// embedding_enabled is left out: it reads false, the tier 0 answer, so
	// graph-query refuses semantic and similar as not_enabled.
	graphQueryConfig := map[string]any{
		"ports": graphquery.DefaultConfig().Ports,
	}

	graphIngestPorts := graphingest.DefaultConfig().Ports
	for i := range graphIngestPorts.Inputs {
		if graphIngestPorts.Inputs[i].Name == "entity_stream" {
			graphIngestPorts.Inputs[i].Config = component.JetStreamPort{
				StreamName:    "GRAPH",
				Subjects:      []string{"graph.ingest.entity"},
				DeliverPolicy: "all",
			}
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
				// Our entity_stream differs from graph-ingest's DefaultConfig
				// (entities arrive on the GRAPH stream's graph.ingest.*
				// subjects, not the default ENTITY stream). Starting from
				// graphingest.DefaultConfig().Ports preserves the canonical
				// mutations and entity_states port definitions without hand-copying.
				"ports": graphIngestPorts,
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
				// Ports come from graph-index's DefaultConfig; it declares
				// the entity watch and every index write itself.
			},
		},
		"graph-query": {
			name:     "graph-query",
			compType: types.ComponentTypeProcessor,
			// The ComponentManager path applies no defaults: the single
			// graph.query.* family request port is restated exactly as
			// graph-query's DefaultConfig declares it.
			configMap: graphQueryConfig,
		},
		"graph-gateway": {
			name:     "graph-gateway",
			compType: types.ComponentTypeGateway,
			configMap: map[string]any{
				// SemEngine graph-gateway serves HTTP on the shared ServiceManager
				// mux and requests only the required graph_queries output port.
				"ports": graphgateway.DefaultConfig().Ports,
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

	if cfg.Graph.LexicalSearchEnabled() {
		configs["graph-lexical"] = struct {
			name      string
			compType  types.ComponentType
			configMap map[string]any
		}{
			name:     "graph-lexical",
			compType: types.ComponentTypeProcessor,
			// Ports come from graph-lexical's DefaultConfig (the
			// ENTITY_STATES watch); it reads offloaded bodies from the
			// shared object store itself.
			configMap: map[string]any{"text_suffixes": graphTextSuffixes()},
		}
	}

	// graph-clustering (opt-in via graph.enable_clustering): entity-ID prefix
	// community grouping over ENTITY_STATES → COMMUNITY_INDEX (ADR-099).
	if enableClustering {
		clusteringConfig := map[string]any{
			"detection_interval": "30s",
			"text_suffixes":      graphTextSuffixes(),
			// Ports come from graph-clustering's DefaultConfig (entity watch).
		}
		configs["graph-clustering"] = struct {
			name      string
			compType  types.ComponentType
			configMap map[string]any
		}{
			name:      "graph-clustering",
			compType:  types.ComponentTypeProcessor,
			configMap: clusteringConfig,
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
		// semengine requires an ordinary stream to declare its
		// discard policy — it was hardcoded to "old" before, so the policy was
		// never anyone's choice.
		//
		// "new" here, deliberately. GRAPH is the ingest TRANSPORT for
		// graph.ingest.* (graph state itself lives in the ENTITY_STATES KV, so
		// ADR-0008's retention-first rule is not in play). At the ceiling,
		// "old" would silently evict published source facts before graph-ingest
		// consumed them — producer-side surfaces all report healthy while the
		// graph quietly loses entities, which is precisely the failure class
		// the no-silent-entity-loss work exists to eliminate. "new" refuses the
		// publish instead, and entitypub treats a non-circuit-breaker publish
		// error as terminal and returns it, so the pressure is visible rather
		// than silent. MaxAge still expires messages, so a stalled producer
		// recovers without intervention.
		Discard: semconfig.StreamDiscardNew,
	}

	if override, ok := cfg.Streams["GRAPH"]; ok {
		if len(override.Subjects) > 0 {
			sc.Subjects = override.Subjects
		}
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
		if override.Discard != "" {
			sc.Discard = override.Discard
		}
	}

	return semconfig.StreamConfigs{"GRAPH": sc}
}

// websocketComponentConfig builds the WebSocket output component config.
func websocketComponentConfig(cfg *config.Config) (types.ComponentConfig, error) {
	bind := cfg.WebSocketBind
	if bind == "" {
		// Mirrors config.applyDefaults for callers holding a bare Config.
		bind = "0.0.0.0:7890"
	}
	wsHost, wsPortStr, err := net.SplitHostPort(bind)
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("parse websocket bind %q: %w", bind, err)
	}
	wsPort, err := strconv.Atoi(wsPortStr)
	if err != nil {
		return types.ComponentConfig{}, fmt.Errorf("parse websocket port %q: %w", wsPortStr, err)
	}
	wsPath := cfg.WebSocketPath
	if wsPath == "" {
		// Mirrors config.applyDefaults for callers holding a bare Config.
		wsPath = "/ws"
	}
	raw, err := json.Marshal(map[string]any{
		"path": wsPath,
		"ports": map[string]any{
			"inputs": []component.PortDefinition{
				{
					Name:        "entity_watch",
					Description: "Applied entity state from ENTITY_STATES",
					Config:      component.KVWatchPort{Bucket: "ENTITY_STATES"},
				},
			},
			"outputs": []component.PortDefinition{
				{
					Name:        "websocket_server",
					Description: "WebSocket server for downstream consumers",
					Config:      component.NetworkPort{Protocol: "http", Host: wsHost, Port: wsPort},
				},
			},
		},
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
			Enabled: true,
			Config:  smCfgJSON,
		},
		"component-manager": types.ServiceConfig{
			Enabled: true,
			Config:  json.RawMessage(`{}`),
		},
		"metrics": types.ServiceConfig{
			Enabled: true,
			Config:  metricsCfgJSON,
		},
		"heartbeat": types.ServiceConfig{
			Enabled: true,
			Config:  json.RawMessage(`{}`),
		},
	}, nil
}

// ingestHandlerConfig supplies product-owned source management at factory construction.
func ingestHandlerConfig(cfg *config.Config, store *semconfig.Manager) sourcemanifest.IngestHandlerConfig {
	return sourcemanifest.IngestHandlerConfig{
		Namespace: cfg.Namespace, Store: store,
		Spawn:    sourcespawn.Options{Org: cfg.Namespace, WorkspaceDir: cfg.WorkspaceDir, MediaStoreDir: cfg.MediaStoreDir},
		APIToken: os.Getenv("SEMSOURCE_API_TOKEN"), AllowedRoots: cfg.SourceRoots,
	}
}
