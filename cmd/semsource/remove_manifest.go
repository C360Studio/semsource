package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// removeManifestCmd removes instance from the persisted desired manifest while
// the service is stopped: the way out when boot refuses a persisted source
// (#349, design D4 option b). The manifest lives in the deployment's config
// bucket, so it needs the NATS server, not the service. It opens the bucket
// the way boot does (file arbitration and prune included) and stops before
// discovery or any component.
func removeManifestCmd(ctx context.Context, configPath, natsFlag, instance string) (err error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", configPath, err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	nc, ssCfg, err := setupNATS(ctx, natsFlag, cfg, logger, 0, nil)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = errors.Join(err, nc.Close(closeCtx))
	}()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, configMgr.Stop(stopCtx))
	}()
	opts := sourcespawn.Options{Org: cfg.Namespace, WorkspaceDir: cfg.WorkspaceDir, MediaStoreDir: cfg.MediaStoreDir}
	if err := sourcemanifest.RemoveDesiredSource(ctx, configMgr, opts, instance); err != nil {
		return fmt.Errorf("remove %s from the desired manifest: %w", instance, err)
	}
	fmt.Printf("Removed %s from the desired manifest; it stays removed at the next boot\n", instance)
	return nil
}
