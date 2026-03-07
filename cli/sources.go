package cli

import (
	"fmt"
	"strings"

	"github.com/c360studio/semsource/config"
)

// Sources loads the config at configPath and prints a formatted table of sources.
func Sources(term *Term, configPath string) error {
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

	term.Header(fmt.Sprintf("Sources in %s (%d)", configPath, len(cfg.Sources)))
	fmt.Fprintln(term.out)

	// Column headers.
	fmt.Fprintf(term.out, "  %-10s %-30s %-10s %s\n", "TYPE", "PATH / URL", "WATCH", "EXTRA")
	fmt.Fprintf(term.out, "  %s\n", strings.Repeat("-", 65))

	for _, s := range cfg.Sources {
		location := sourceLocation(s)
		watchStr := "no"
		if s.Watch {
			watchStr = "yes"
		}
		extra := sourceExtra(s)
		fmt.Fprintf(term.out, "  %-10s %-30s %-10s %s\n", s.Type, truncate(location, 30), watchStr, extra)
	}
	fmt.Fprintln(term.out)
	return nil
}

func sourceLocation(s config.SourceEntry) string {
	switch s.Type {
	case "ast":
		return s.Path
	case "git":
		return s.URL
	case "docs", "config":
		return strings.Join(s.Paths, ", ")
	case "url":
		if len(s.URLs) > 0 {
			return s.URLs[0]
		}
	}
	return ""
}

func sourceExtra(s config.SourceEntry) string {
	switch s.Type {
	case "ast":
		if s.Language != "" {
			return "lang=" + s.Language
		}
	case "git":
		if s.Branch != "" {
			return "branch=" + s.Branch
		}
	case "url":
		if s.PollInterval != "" {
			return "poll=" + s.PollInterval
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
