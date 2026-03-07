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
// It auto-detects the project context to provide smart defaults.
// An optional ProjectInfo can be passed to override auto-detection (for testing).
func Init(term *Term, configPath string, overrides ...*ProjectInfo) error {
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

	// Auto-detect project context (or use override).
	var info *ProjectInfo
	if len(overrides) > 0 && overrides[0] != nil {
		info = overrides[0]
	} else {
		dir, _ := os.Getwd()
		info = DetectProject(dir)
	}

	term.Header("semsource setup wizard")
	printDetectionSummary(term, info)

	// Step 1: namespace (with smart default).
	namespace := term.Prompt("Organization name (namespace)", info.Namespace)
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	// Step 2: graph stream address.
	graphAddr := term.Prompt("Graph stream address", "localhost:7890")

	// Step 3: source type multi-select (pre-selected based on detection).
	sources, err := runSourceMenu(term, info)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}

	cfg := buildConfig(namespace, graphAddr, sources)

	return writeAndSummarize(term, configPath, cfg)
}

// InitQuick runs a zero-prompt setup using auto-detected defaults.
// Falls back to interactive Init if detection finds nothing useful.
// An optional ProjectInfo can be passed to override auto-detection (for testing).
func InitQuick(term *Term, configPath string, overrides ...*ProjectInfo) error {
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

	var info *ProjectInfo
	if len(overrides) > 0 && overrides[0] != nil {
		info = overrides[0]
	} else {
		dir, _ := os.Getwd()
		info = DetectProject(dir)
	}

	if info.Namespace == "" {
		term.Info("Could not auto-detect project details. Falling back to interactive mode.")
		return Init(term, configPath)
	}

	term.Header("semsource quick setup")
	printDetectionSummary(term, info)

	// Build sources from detected context.
	sources := buildDetectedSources(info)
	if len(sources) == 0 {
		term.Info("No sources detected. Falling back to interactive mode.")
		return Init(term, configPath)
	}

	cfg := buildConfig(info.Namespace, "localhost:7890", sources)

	return writeAndSummarize(term, configPath, cfg)
}

// buildDetectedSources creates SourceEntry values from auto-detected project info.
func buildDetectedSources(info *ProjectInfo) []config.SourceEntry {
	var sources []config.SourceEntry

	if info.HasGit {
		sources = append(sources, config.SourceEntry{
			Type:   "git",
			URL:    ".",
			Branch: "main",
			Watch:  true,
		})
	}

	if info.Language != "" {
		sources = append(sources, config.SourceEntry{
			Type:     "ast",
			Path:     ".",
			Language: info.Language,
			Watch:    true,
		})
	}

	if info.HasDocs && len(info.DocPaths) > 0 {
		sources = append(sources, config.SourceEntry{
			Type:  "docs",
			Paths: info.DocPaths,
			Watch: true,
		})
	}

	if len(info.ConfigFiles) > 0 {
		sources = append(sources, config.SourceEntry{
			Type:  "config",
			Paths: info.ConfigFiles,
			Watch: true,
		})
	}

	if info.HasImages && len(info.ImagePaths) > 0 {
		sources = append(sources, config.SourceEntry{
			Type:  "image",
			Paths: info.ImagePaths,
			Watch: true,
		})
	}

	return sources
}

// runSourceMenu shows the numbered toggle menu, pre-selecting based on detection.
func runSourceMenu(term *Term, info *ProjectInfo) ([]config.SourceEntry, error) {
	wizards := Wizards()
	selected := make([]bool, len(wizards))

	// Pre-select based on detection instead of selecting everything.
	for i, w := range wizards {
		ok, _ := w.Available()
		if !ok {
			continue
		}
		selected[i] = shouldPreselect(w.TypeKey(), info)
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

	// Run prompts for each selected wizard, passing detection info for defaults.
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

// shouldPreselect returns true if a source type is relevant to the detected project.
func shouldPreselect(typeKey string, info *ProjectInfo) bool {
	switch typeKey {
	case "git":
		return info.HasGit
	case "ast":
		return info.Language != ""
	case "docs":
		return info.HasDocs
	case "config":
		return len(info.ConfigFiles) > 0
	case "url":
		return false // URLs are never auto-detected
	case "image":
		return info.HasImages
	default:
		return false
	}
}

// printDetectionSummary shows what was auto-detected.
func printDetectionSummary(term *Term, info *ProjectInfo) {
	var found []string
	if info.HasGit {
		found = append(found, "git repo")
	}
	if info.Language != "" {
		found = append(found, info.Language+" project")
	}
	if info.HasDocs {
		found = append(found, "docs")
	}
	if len(info.ConfigFiles) > 0 {
		found = append(found, fmt.Sprintf("%d config file(s)", len(info.ConfigFiles)))
	}
	if info.HasImages {
		found = append(found, fmt.Sprintf("%d image dir(s)", len(info.ImagePaths)))
	}

	if len(found) > 0 {
		term.Info(fmt.Sprintf("Detected: %s", strings.Join(found, ", ")))
	} else {
		term.Info("No project files detected — you'll configure sources manually.")
	}
	fmt.Fprintln(term.out)
}

func buildConfig(namespace, graphAddr string, sources []config.SourceEntry) *config.Config {
	return &config.Config{
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
}

func writeAndSummarize(term *Term, configPath string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	term.Success(fmt.Sprintf("Config written to %s", configPath))
	printSummary(term, cfg)
	return nil
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

	// P6: Post-init guidance.
	fmt.Fprintln(term.out)
	term.Header("Next steps")
	term.Info("  semsource run        Start ingesting and streaming your knowledge graph")
	term.Info("  semsource add        Add another source (interactive or with flags)")
	term.Info("  semsource sources    View configured sources")
	term.Info("  semsource validate   Check your config is valid")
	fmt.Fprintln(term.out)
	term.Info("Docs: https://github.com/c360studio/semsource")
}
