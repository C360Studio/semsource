// Command semsource is the SemSource graph ingestion service.
//
// It reads a YAML config file, ingests configured sources through registered
// handlers, normalizes the results into a knowledge graph, and continuously
// emits graph events (SEED, DELTA, RETRACT, HEARTBEAT) to downstream consumers
// via the configured output (WebSocket broadcast on :7890 by default).
//
// Usage:
//
//	semsource [-config semsource.yaml]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	"github.com/c360studio/semsource/normalizer"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "semsource: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// --- CLI flags ---
	configPath := flag.String("config", "semsource.yaml", "path to semsource YAML config file")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("semsource %s\n", version)
		return nil
	}

	// --- Logger ---
	logger := buildLogger(*logLevel)
	slog.SetDefault(logger)

	// --- Config ---
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	logger.Info("configuration loaded",
		"namespace", cfg.Namespace,
		"sources", len(cfg.Sources),
	)

	// --- Normalizer ---
	norm := normalizer.New(normalizer.Config{Org: cfg.Namespace})

	// --- Emitter ---
	// TODO(M2): Replace LogEmitter with a NATSEmitter or WebSocket emitter
	// once the output transport is wired. The Emitter interface means this
	// is a one-line swap at the call site.
	emitter := engine.NewLogEmitter(logger)

	// --- Engine ---
	eng := engine.NewEngine(
		cfg,
		emitter,
		logger,
		engine.WithNormalizer(norm),
	)

	// --- Handler registration ---
	// M1: No concrete handlers yet. The engine runs in scaffold mode —
	// it will emit an empty SEED on startup and watch no sources.
	// Each handler is registered here as it is implemented in M2:
	//
	//   eng.RegisterHandler(git.NewHandler(logger))
	//   eng.RegisterHandler(ast.NewHandler(logger))
	//   eng.RegisterHandler(doc.NewHandler(logger))
	//   eng.RegisterHandler(cfg2.NewHandler(logger))
	//   eng.RegisterHandler(url.NewHandler(logger))

	// --- Signal handling ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Start ---
	logger.Info("starting semsource", "version", version)
	if err := eng.Start(ctx); err != nil {
		return fmt.Errorf("engine start: %w", err)
	}

	// Block until signal received.
	<-ctx.Done()
	logger.Info("shutdown signal received, stopping engine")

	if err := eng.Stop(); err != nil {
		return fmt.Errorf("engine stop: %w", err)
	}

	logger.Info("semsource stopped")
	return nil
}

// buildLogger constructs an slog.Logger at the given level writing JSON to stdout.
func buildLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
