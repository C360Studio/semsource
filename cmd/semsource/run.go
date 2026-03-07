package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	asthandler "github.com/c360studio/semsource/handler/ast"
	"github.com/c360studio/semsource/handler/cfgfile"
	dochandler "github.com/c360studio/semsource/handler/doc"
	githandler "github.com/c360studio/semsource/handler/git"
	imghandler "github.com/c360studio/semsource/handler/image"
	urlhandler "github.com/c360studio/semsource/handler/url"
	"github.com/c360studio/semsource/normalizer"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/storage/objectstore"
)

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logger := buildLogger(*logLevel)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", *configPath, err)
	}
	logger.Info("configuration loaded",
		"namespace", cfg.Namespace,
		"sources", len(cfg.Sources),
	)

	norm := normalizer.New(normalizer.Config{Org: cfg.Namespace})
	emitter := engine.NewLogEmitter(logger)

	eng := engine.NewEngine(
		cfg,
		emitter,
		logger,
		engine.WithNormalizer(norm),
	)

	eng.RegisterHandler(asthandler.New(logger))
	eng.RegisterHandler(githandler.New(githandler.Config{}))
	eng.RegisterHandler(dochandler.New())
	eng.RegisterHandler(cfgfile.New(nil))
	eng.RegisterHandler(urlhandler.New(logger))

	// Wire ObjectStore into ImageHandler when configured.
	var natsClient *natsclient.Client
	var objStore *objectstore.Store
	if cfg.ObjectStore != nil {
		client, connectErr := natsclient.NewClient(cfg.ObjectStore.NATSUrl)
		if connectErr != nil {
			return fmt.Errorf("objectstore: create NATS client: %w", connectErr)
		}
		natsClient = client

		connectCtx, connectCancel := context.WithTimeout(context.Background(), 10_000_000_000) // 10s
		if connectErr = natsClient.Connect(connectCtx); connectErr != nil {
			connectCancel()
			return fmt.Errorf("objectstore: connect to NATS: %w", connectErr)
		}

		store, storeErr := objectstore.NewStoreWithConfig(connectCtx, natsClient, objectstore.Config{
			BucketName: cfg.ObjectStore.Bucket,
		})
		connectCancel()
		if storeErr != nil {
			return fmt.Errorf("objectstore: create store: %w", storeErr)
		}
		objStore = store
		logger.Info("object store connected", "bucket", cfg.ObjectStore.Bucket)

		eng.RegisterHandler(imghandler.New(imghandler.WithStore(objStore), imghandler.WithLogger(logger)))
	} else {
		eng.RegisterHandler(imghandler.New())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting semsource", "version", version)
	if err := eng.Start(ctx); err != nil {
		return fmt.Errorf("engine start: %w", err)
	}

	<-ctx.Done()
	logger.Info("shutdown signal received, stopping engine")

	if err := eng.Stop(); err != nil {
		return fmt.Errorf("engine stop: %w", err)
	}

	// Clean up ObjectStore resources.
	if objStore != nil {
		if err := objStore.Close(); err != nil {
			logger.Warn("object store close error", "err", err)
		}
	}
	if natsClient != nil {
		if err := natsClient.Close(context.Background()); err != nil {
			logger.Warn("NATS client close error", "err", err)
		}
	}

	logger.Info("semsource stopped")
	return nil
}
