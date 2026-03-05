// Package ast implements the SourceHandler for AST-based code entity extraction.
// It delegates parsing to the semspec ast package and its registered language parsers.
//
// Language parsers register themselves via init() through blank imports:
//
//	import _ "github.com/c360studio/semspec/processor/ast/golang"
//	import _ "github.com/c360studio/semspec/processor/ast/ts"
package ast

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	semspecast "github.com/c360studio/semspec/processor/ast"
	// Register Go and TypeScript/JavaScript language parsers.
	_ "github.com/c360studio/semspec/processor/ast/golang"
	_ "github.com/c360studio/semspec/processor/ast/ts"

	"github.com/c360studio/semsource/handler"
)

// ASTConfig is an optional extended interface that SourceConfig implementations
// may satisfy to provide AST-specific configuration. When not implemented,
// sensible defaults are used.
type ASTConfig interface {
	handler.SourceConfig
	// GetLanguage returns the source language (e.g. "go", "ts"). Defaults to "go".
	GetLanguage() string
	// GetOrg returns the org namespace for entity IDs. Defaults to "public".
	GetOrg() string
	// GetProject returns the project slug for entity IDs.
	// Defaults to a slug derived from the path.
	GetProject() string
}

// Handler implements handler.SourceHandler for AST-based code indexing.
// It is safe for concurrent use.
type Handler struct {
	logger *slog.Logger
}

// New creates an ASTHandler.
func New(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}

// SourceType returns "ast".
func (h *Handler) SourceType() string { return handler.SourceTypeAST }

// Supports returns true only for sources with type "ast".
func (h *Handler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == handler.SourceTypeAST
}

// Ingest parses the directory at cfg.GetPath() using the configured language
// parser and returns all extracted code entities as RawEntity values.
// Respects context cancellation.
func (h *Handler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	lang, org, project, root := resolveConfig(cfg)

	parser, err := semspecast.DefaultRegistry.CreateParser(lang, org, project, root)
	if err != nil {
		return nil, fmt.Errorf("asthandler: create %s parser: %w", lang, err)
	}

	results, err := parseDirectory(ctx, parser, root)
	if err != nil {
		return nil, fmt.Errorf("asthandler: parse %s: %w", root, err)
	}

	system := pathToSystemSlug(root)
	var entities []handler.RawEntity
	for _, result := range results {
		entities = append(entities, mapParseResult(result, lang, system)...)
	}
	return entities, nil
}

// Watch starts an fsnotify-based watcher on cfg.GetPath() and translates
// ast.WatchEvent values into handler.ChangeEvent values on the returned channel.
// Returns nil, nil if cfg.IsWatchEnabled() is false.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	lang, org, project, root := resolveConfig(cfg)

	parser, err := semspecast.DefaultRegistry.CreateParser(lang, org, project, root)
	if err != nil {
		return nil, fmt.Errorf("asthandler: create %s parser for watch: %w", lang, err)
	}

	wcfg := semspecast.WatcherConfig{
		RepoRoot:      root,
		Org:           org,
		Project:       project,
		DebounceDelay: 100 * time.Millisecond,
		Logger:        h.logger,
		FileExtensions: langToExtensions(lang),
		ExcludeDirs:   []string{"vendor", "node_modules", ".git"},
	}

	watcher, err := semspecast.NewWatcherWithParser(wcfg, parser)
	if err != nil {
		return nil, fmt.Errorf("asthandler: create watcher: %w", err)
	}

	if err := watcher.Start(ctx); err != nil {
		return nil, fmt.Errorf("asthandler: start watcher: %w", err)
	}

	system := pathToSystemSlug(root)
	out := make(chan handler.ChangeEvent, 64)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				watcher.Stop() //nolint:errcheck
				return
			case ev, ok := <-watcher.Events():
				if !ok {
					return
				}
				ce := translateWatchEvent(ev, lang, system)
				select {
				case out <- ce:
				case <-ctx.Done():
					watcher.Stop() //nolint:errcheck
					return
				}
			}
		}
	}()

	return out, nil
}

// parseDirectory walks root and calls ParseFile on every file the parser handles.
func parseDirectory(ctx context.Context, parser semspecast.FileParser, root string) ([]*semspecast.ParseResult, error) {
	// Prefer the optional ParseDirectory method if available (Go parser supports it).
	type directoryParser interface {
		ParseDirectory(ctx context.Context, dirPath string) ([]*semspecast.ParseResult, error)
	}
	if dp, ok := parser.(directoryParser); ok {
		return dp.ParseDirectory(ctx, root)
	}

	// Fall back to walking the directory and calling ParseFile on each file.
	var results []*semspecast.ParseResult
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		result, parseErr := parser.ParseFile(ctx, path)
		if parseErr != nil {
			// Log and continue — a single bad file must not abort the whole ingest.
			return nil
		}
		if result != nil {
			results = append(results, result)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// resolveConfig extracts language, org, project, and root path from a SourceConfig,
// applying defaults when the optional ASTConfig interface is not implemented.
func resolveConfig(cfg handler.SourceConfig) (lang, org, project, root string) {
	root = cfg.GetPath()
	lang = "go"
	org = "public"
	project = pathToSystemSlug(root)

	if ac, ok := cfg.(ASTConfig); ok {
		if l := ac.GetLanguage(); l != "" {
			lang = l
		}
		if o := ac.GetOrg(); o != "" {
			org = o
		}
		if p := ac.GetProject(); p != "" {
			project = p
		}
	}
	return
}

// langToExtensions returns the file extensions for a given language name.
func langToExtensions(lang string) []string {
	switch lang {
	case "ts", "typescript", "javascript":
		return []string{".ts", ".tsx", ".js", ".jsx"}
	default: // "go"
		return []string{".go"}
	}
}

// pathToSystemSlug converts a filesystem path to a NATS-safe system slug.
// "/Users/coby/Code/acme/gcs" → "Users-coby-Code-acme-gcs"
func pathToSystemSlug(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "-")
	path = strings.ReplaceAll(path, ".", "-")
	return path
}
