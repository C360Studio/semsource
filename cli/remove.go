package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/c360studio/semsource/config"
)

// Remove lets the user remove a source from the config.
// If index is >= 0, it removes that source directly (non-interactive).
// Otherwise it shows a numbered list and prompts for selection.
func Remove(term *Term, configPath string, index int) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Sources) == 0 {
		term.Info("No sources configured.")
		return nil
	}

	if index < 0 {
		// Interactive: show list and prompt.
		var labels []string
		for i, s := range cfg.Sources {
			loc := sourceLocation(s)
			if loc == "" {
				loc = strings.Join(s.Paths, ", ")
			}
			labels = append(labels, fmt.Sprintf("[%d] %s  %s", i+1, s.Type, truncate(loc, 40)))
		}

		term.Header("Remove a source")
		index = term.Select("Which source to remove?", labels)
	}

	if index < 0 || index >= len(cfg.Sources) {
		return fmt.Errorf("invalid source index %d (have %d sources)", index+1, len(cfg.Sources))
	}

	removed := cfg.Sources[index]
	cfg.Sources = append(cfg.Sources[:index], cfg.Sources[index+1:]...)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	term.Success(fmt.Sprintf("Removed %s source from %s", removed.Type, configPath))
	return nil
}
