package cli

import (
	"fmt"

	"github.com/c360studio/semsource/config"
)

// Validate loads the config at configPath and prints a validation result.
func Validate(term *Term, configPath string) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
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
