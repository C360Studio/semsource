package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/c360studio/semsource/config"
)

const defaultConfigPath = "semsource.json"

// Init runs the interactive setup wizard and writes semsource.json.
// It reads from term's input and writes prompts to term's output.
func Init(term *Term, configPath string) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	// Warn if config already exists.
	if _, err := os.Stat(configPath); err == nil {
		term.Println(fmt.Sprintf("  %s already exists.", configPath))
		if !term.Confirm("Overwrite it?", false) {
			term.Info("Aborted.")
			return nil
		}
	}

	term.Header("semsource setup wizard")
	term.Info("Answer the prompts to generate your semsource.json config.")

	// Step 1: namespace.
	namespace := term.Prompt("Organization name (namespace)", "")
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	// Step 2: graph stream address.
	graphAddr := term.Prompt("Graph stream address", "localhost:7890")

	// Step 3: source type multi-select.
	sources, err := runSourceMenu(term)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}

	// Build config.
	cfg := config.Config{
		Namespace: namespace,
		Flow: config.FlowConfig{
			Outputs: []config.OutputConfig{
				{
					Name:    "graph-stream",
					Type:    "network",
					Subject: "http://" + graphAddr + "/graph",
				},
			},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: sources,
	}

	// Marshal and write.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	term.Success(fmt.Sprintf("Config written to %s", configPath))
	printSummary(term, &cfg)
	return nil
}

// runSourceMenu shows the numbered toggle menu and returns completed SourceEntry list.
func runSourceMenu(term *Term) ([]config.SourceEntry, error) {
	wizards := Wizards()
	selected := make([]bool, len(wizards))

	// Default all available sources to selected.
	for i, w := range wizards {
		ok, _ := w.Available()
		selected[i] = ok
	}

	for {
		term.Header("Select sources to ingest (enter numbers to toggle, done to finish):")
		fmt.Fprintln(term.out)
		for i, w := range wizards {
			ok, reason := w.Available()
			mark := " "
			if selected[i] {
				mark = "x"
			}
			label := fmt.Sprintf("%s — %s", w.Name(), w.Description())
			if !ok {
				label = fmt.Sprintf("%s (%s)", label, reason)
			}
			fmt.Fprintf(term.out, "  [%s] %d. %s\n", mark, i+1, label)
		}
		fmt.Fprintln(term.out)
		fmt.Fprintf(term.out, "  Toggle> ")
		line, ok := term.readLine()
		if !ok {
			break
		}
		line = strings.TrimSpace(line)

		if strings.EqualFold(line, "done") || line == "" {
			break
		}

		var n int
		if _, err := fmt.Sscanf(line, "%d", &n); err != nil || n < 1 || n > len(wizards) {
			term.Info(fmt.Sprintf("  Enter a number 1-%d or 'done'.", len(wizards)))
			continue
		}
		idx := n - 1
		ok, reason := wizards[idx].Available()
		if !ok {
			term.Info(fmt.Sprintf("  %s is %s.", wizards[idx].Name(), reason))
			continue
		}
		selected[idx] = !selected[idx]
	}

	// Run prompts for each selected wizard.
	var sources []config.SourceEntry
	for i, w := range wizards {
		if !selected[i] {
			continue
		}
		entry, err := w.Prompts(term)
		if err != nil {
			return nil, fmt.Errorf("wizard %s: %w", w.Name(), err)
		}
		sources = append(sources, *entry)
	}
	return sources, nil
}

func printSummary(term *Term, cfg *config.Config) {
	term.Header("Summary")
	term.Info(fmt.Sprintf("  Namespace : %s", cfg.Namespace))
	if len(cfg.Flow.Outputs) > 0 {
		term.Info(fmt.Sprintf("  Output    : %s", cfg.Flow.Outputs[0].Subject))
	}
	term.Info(fmt.Sprintf("  Sources   : %d configured", len(cfg.Sources)))
	for _, s := range cfg.Sources {
		term.Info(fmt.Sprintf("    - %s", s.Type))
	}
	fmt.Fprintln(term.out)
	term.Info("Run 'semsource run' to start.")
}
