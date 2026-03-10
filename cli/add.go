package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/c360studio/semsource/config"
)

// Add either runs the interactive wizard to add a source (no extra args),
// or parses non-interactive flags for the given typeKey.
//
// args are the arguments after "semsource add", e.g. ["ast", "--path", "./src"].
func Add(term *Term, configPath string, args []string) error {
	if configPath == "" {
		configPath = defaultConfigPath
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var entry *config.SourceEntry

	if len(args) > 0 {
		// Non-interactive: semsource add <type> [flags]
		typeKey := args[0]
		entry, err = addNonInteractive(typeKey, args[1:])
		if err != nil {
			return err
		}
	} else {
		// Interactive: show source type menu (single select).
		wizards := Wizards()
		var available []SourceWizard
		var labels []string
		for _, w := range wizards {
			ok, reason := w.Available()
			if !ok {
				labels = append(labels, fmt.Sprintf("%s — %s (%s)", w.Name(), w.Description(), reason))
			} else {
				labels = append(labels, fmt.Sprintf("%s — %s", w.Name(), w.Description()))
			}
			available = append(available, w)
		}

		idx := term.Select("Choose source type to add", labels)
		chosen := available[idx]

		ok, reason := chosen.Available()
		if !ok {
			return fmt.Errorf("%s is %s", chosen.Name(), reason)
		}

		entry, err = chosen.Prompts(term)
		if err != nil {
			return err
		}
	}

	cfg.Sources = append(cfg.Sources, *entry)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	term.Success(fmt.Sprintf("Added %s source to %s", entry.Type, configPath))
	return nil
}

// addNonInteractive dispatches to a type-specific flag parser.
func addNonInteractive(typeKey string, args []string) (*config.SourceEntry, error) {
	switch typeKey {
	case "ast":
		return parseASTFlags(args)
	case "git":
		return parseGitFlags(args)
	case "repo":
		return parseRepoFlags(args)
	case "docs":
		return parseDocsFlags(args)
	case "config":
		return parseConfigFlags(args)
	case "url":
		return parseURLFlags(args)
	case "image":
		return parseImageFlags(args)
	case "video":
		return parseVideoFlags(args)
	case "audio":
		return parseAudioFlags(args)
	default:
		return nil, fmt.Errorf("unknown source type %q (valid: ast, git, repo, docs, config, url, image, video, audio)", typeKey)
	}
}

func parseASTFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add ast", flag.ContinueOnError)
	path := fs.String("path", ".", "root path to scan")
	language := fs.String("language", "", "language (go, typescript, python, java, svelte)")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &config.SourceEntry{
		Type:     "ast",
		Path:     *path,
		Language: *language,
		Watch:    *watch,
	}, nil
}

func parseGitFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add git", flag.ContinueOnError)
	url := fs.String("url", "", "repository path or URL")
	branch := fs.String("branch", "main", "branch to track")
	watch := fs.Bool("watch", true, "watch for new commits")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *url == "" {
		return nil, fmt.Errorf("git source requires --url")
	}
	return &config.SourceEntry{
		Type:   "git",
		URL:    *url,
		Branch: *branch,
		Watch:  *watch,
	}, nil
}

func parseRepoFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add repo", flag.ContinueOnError)
	url := fs.String("url", "", "remote repository URL to clone and analyze")
	branch := fs.String("branch", "", "branch (default: remote default)")
	language := fs.String("language", "", "primary language (go, java, python, typescript, or leave blank to auto-detect)")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *url == "" {
		return nil, fmt.Errorf("repo source requires --url")
	}
	return &config.SourceEntry{
		Type:     "repo",
		URL:      *url,
		Branch:   *branch,
		Language: *language,
		Watch:    *watch,
	}, nil
}

func parseDocsFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add docs", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("docs source requires --paths")
	}
	return &config.SourceEntry{
		Type:  "docs",
		Paths: splitComma(*paths),
		Watch: *watch,
	}, nil
}

func parseConfigFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add config", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of config file paths")
	watch := fs.Bool("watch", true, "watch for changes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("config source requires --paths")
	}
	return &config.SourceEntry{
		Type:  "config",
		Paths: splitComma(*paths),
		Watch: *watch,
	}, nil
}

func parseURLFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add url", flag.ContinueOnError)
	urls := fs.String("urls", "", "comma-separated list of URLs")
	poll := fs.String("poll-interval", "5m", "poll interval")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *urls == "" {
		return nil, fmt.Errorf("url source requires --urls")
	}
	return &config.SourceEntry{
		Type:         "url",
		URLs:         splitComma(*urls),
		PollInterval: *poll,
	}, nil
}

func parseImageFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add image", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	maxSize := fs.String("max-file-size", "50MB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("image source requires --paths")
	}
	return &config.SourceEntry{
		Type:        "image",
		Paths:       splitComma(*paths),
		Watch:       *watch,
		MaxFileSize: *maxSize,
	}, nil
}

func parseVideoFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add video", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	keyframeMode := fs.String("keyframe-mode", "interval", "keyframe extraction mode: interval, scene, or iframes")
	keyframeInterval := fs.String("keyframe-interval", "30s", "interval between keyframe extractions (e.g. 30s)")
	maxSize := fs.String("max-file-size", "2GB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("video source requires --paths")
	}
	return &config.SourceEntry{
		Type:             "video",
		Paths:            splitComma(*paths),
		Watch:            *watch,
		KeyframeMode:     *keyframeMode,
		KeyframeInterval: *keyframeInterval,
		MaxFileSize:      *maxSize,
	}, nil
}

func parseAudioFlags(args []string) (*config.SourceEntry, error) {
	fs := flag.NewFlagSet("add audio", flag.ContinueOnError)
	paths := fs.String("paths", "", "comma-separated list of paths to scan")
	watch := fs.Bool("watch", true, "watch for changes")
	maxSize := fs.String("max-file-size", "500MB", "maximum file size")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *paths == "" {
		return nil, fmt.Errorf("audio source requires --paths")
	}
	return &config.SourceEntry{
		Type:        "audio",
		Paths:       splitComma(*paths),
		Watch:       *watch,
		MaxFileSize: *maxSize,
	}, nil
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
