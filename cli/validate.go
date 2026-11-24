package cli

import (
	"context"
	"fmt"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

// Validate loads the config at configPath and prints a validation result. It
// also refuses sources that would share an entity-ID system (#349); that check
// lives here, not in config.Validate, because sourcespawn imports config.
func Validate(ctx context.Context, term *Term, configPath string) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	opts := sourcespawn.Options{Org: cfg.Namespace, WorkspaceDir: cfg.WorkspaceDir, MediaStoreDir: cfg.MediaStoreDir}
	if err := sourcespawn.CheckSourceCollisions(ctx, cfg.Sources, opts); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	term.Success(fmt.Sprintf("Config at %s is valid", configPath))
	term.Info(fmt.Sprintf("  Namespace : %s", cfg.Namespace))
	term.Info(fmt.Sprintf("  Sources   : %d", len(cfg.Sources)))
	for _, s := range cfg.Sources {
		term.Info(fmt.Sprintf("    - %s", s.Type))
	}
	return nil
}
