// Package cfgfile provides a SourceHandler for configuration files:
// go.mod, package.json, and Dockerfile. It uses the shared FSWatcher
// for real-time monitoring.
package cfgfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// configFileNames is the set of file names that this handler processes.
var configFileNames = map[string]bool{
	"go.mod":       true,
	"package.json": true,
	"Dockerfile":   true,
}

// Config controls optional ConfigHandler behaviour.
type Config struct {
	// WatchConfig is forwarded to the underlying FSWatcher.
	Watch fswatcher.WatchConfig
}

// ConfigHandler implements handler.SourceHandler for go.mod, package.json,
// and Dockerfile sources.
type ConfigHandler struct {
	cfg    *Config
	logger *slog.Logger
}

// New creates a ConfigHandler. cfg may be nil; defaults are applied.
func New(cfg *Config) *ConfigHandler {
	if cfg == nil {
		cfg = &Config{}
	}
	return &ConfigHandler{cfg: cfg, logger: slog.Default()}
}

// SourceType implements handler.SourceHandler.
func (h *ConfigHandler) SourceType() string { return handler.SourceTypeConfig }

// Supports implements handler.SourceHandler.
// Returns true when cfg.GetType() == "config" and the path exists.
func (h *ConfigHandler) Supports(cfg handler.SourceConfig) bool {
	if cfg.GetType() != handler.SourceTypeConfig {
		return false
	}
	p := cfg.GetPath()
	if p == "" {
		return false
	}
	if _, err := os.Stat(p); err != nil {
		return false
	}
	return true
}

// Ingest implements handler.SourceHandler.
// It walks the path in cfg, parses recognised config files, and returns
// the resulting RawEntity slice.
func (h *ConfigHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	root := cfg.GetPath()
	if root == "" {
		return nil, fmt.Errorf("cfgfile: path is required")
	}

	var entities []handler.RawEntity

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
			return nil
		}
		base := filepath.Base(path)
		if !configFileNames[base] {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			h.logger.Warn("cfgfile: failed to read file", "path", path, "error", readErr)
			return nil
		}

		parsed := h.parseFile(base, path, content, root)
		entities = append(entities, parsed...)
		return nil
	})
	if err != nil && err != context.Canceled {
		return nil, fmt.Errorf("cfgfile: walk %s: %w", root, err)
	}

	return entities, nil
}

// Watch implements handler.SourceHandler.
// Returns nil, nil when watching is disabled.
func (h *ConfigHandler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	watchCfg := h.cfg.Watch
	watchCfg.Enabled = true
	// Watch recognised config file extensions; also watch extensionless (Dockerfile)
	watchCfg.FileExtensions = []string{".mod", ".json"}

	fsw, err := fswatcher.New(watchCfg, cfg.GetPath())
	if err != nil {
		return nil, fmt.Errorf("cfgfile: create watcher: %w", err)
	}
	if err := fsw.Start(ctx); err != nil {
		return nil, fmt.Errorf("cfgfile: start watcher: %w", err)
	}

	out := make(chan handler.ChangeEvent, 64)
	go h.fanOut(ctx, cfg.GetPath(), fsw.Events(), out)
	return out, nil
}

// fanOut reads raw FSWatcher events, enriches them with parsed entities,
// and forwards them to the output channel. It also detects Dockerfile changes
// which the FSWatcher may miss (extensionless).
func (h *ConfigHandler) fanOut(ctx context.Context, root string, in <-chan handler.ChangeEvent, out chan<- handler.ChangeEvent) {
	defer close(out)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			base := filepath.Base(ev.Path)
			if !configFileNames[base] {
				continue
			}
			if ev.Operation == handler.OperationDelete {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
				continue
			}
			content, err := os.ReadFile(ev.Path)
			if err != nil {
				h.logger.Warn("cfgfile watcher: failed to read file",
					"path", ev.Path, "error", err)
				continue
			}
			entities := h.parseFile(base, ev.Path, content, root)
			enriched := handler.ChangeEvent{
				Path:      ev.Path,
				Operation: ev.Operation,
				Timestamp: time.Now(),
				Entities:  entities,
			}
			select {
			case out <- enriched:
			case <-ctx.Done():
				return
			}
		}
	}
}

// parseFile dispatches to the right parser based on the base filename and
// converts the result to []handler.RawEntity.
func (h *ConfigHandler) parseFile(base, path string, content []byte, root string) []handler.RawEntity {
	system := systemSlug(root)
	switch base {
	case "go.mod":
		return h.goModEntities(content, path, system)
	case "package.json":
		return h.packageJSONEntities(content, path, system)
	case "Dockerfile":
		return h.dockerfileEntities(content, path, system)
	}
	return nil
}

// goModEntities converts a ParseGoMod result to RawEntity values.
func (h *ConfigHandler) goModEntities(content []byte, path, system string) []handler.RawEntity {
	result, err := ParseGoMod(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse go.mod failed", "path", path, "error", err)
		return nil
	}

	var entities []handler.RawEntity

	// Module entity — instance is the module path slug (dots/slashes → dashes)
	modInstance := slugify(result.Module)
	entities = append(entities, handler.RawEntity{
		SourceType: handler.SourceTypeConfig,
		Domain:     handler.DomainConfig,
		System:     system,
		EntityType: "module",
		Instance:   modInstance,
		Properties: map[string]any{
			"module":     result.Module,
			"go_version": result.GoVersion,
			"file_path":  path,
		},
	})

	// Dependency entities — instance is content hash of (module+version)
	for _, dep := range result.Deps {
		instance := contentHashShort(dep.Path + "@" + dep.Version)
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "dependency",
			Instance:   instance,
			Properties: map[string]any{
				"name":     dep.Path,
				"version":  dep.Version,
				"indirect": dep.Indirect,
				"kind":     "go",
			},
			Edges: []handler.RawEdge{
				{
					FromHint: modInstance,
					ToHint:   instance,
					EdgeType: "requires",
				},
			},
		})
	}

	return entities
}

// packageJSONEntities converts a ParsePackageJSON result to RawEntity values.
func (h *ConfigHandler) packageJSONEntities(content []byte, path, system string) []handler.RawEntity {
	result, err := ParsePackageJSON(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse package.json failed", "path", path, "error", err)
		return nil
	}

	var entities []handler.RawEntity

	// Package entity — instance is the npm package name slug
	pkgInstance := slugify(result.Name)
	if pkgInstance == "" {
		pkgInstance = contentHashShort(path)
	}
	entities = append(entities, handler.RawEntity{
		SourceType: handler.SourceTypeConfig,
		Domain:     handler.DomainConfig,
		System:     system,
		EntityType: "package",
		Instance:   pkgInstance,
		Properties: map[string]any{
			"name":      result.Name,
			"version":   result.Version,
			"file_path": path,
		},
	})

	// Dependency entities
	for _, dep := range result.Deps {
		instance := contentHashShort(dep.Name + "@" + dep.Version)
		depKind := "prod"
		if dep.Dev {
			depKind = "dev"
		}
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "dependency",
			Instance:   instance,
			Properties: map[string]any{
				"name":    dep.Name,
				"version": dep.Version,
				"kind":    "npm-" + depKind,
			},
			Edges: []handler.RawEdge{
				{
					FromHint: pkgInstance,
					ToHint:   instance,
					EdgeType: "depends",
				},
			},
		})
	}

	return entities
}

// dockerfileEntities converts a ParseDockerfile result to RawEntity values.
func (h *ConfigHandler) dockerfileEntities(content []byte, path, system string) []handler.RawEntity {
	result, err := ParseDockerfile(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse Dockerfile failed", "path", path, "error", err)
		return nil
	}

	var entities []handler.RawEntity

	for _, img := range result.BaseImages {
		instance := slugify(img)
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "image",
			Instance:   instance,
			Properties: map[string]any{
				"image":        img,
				"exposed_ports": result.ExposedPorts,
				"file_path":    path,
			},
		})
	}

	return entities
}

// systemSlug returns a NATS-safe system slug derived from the root directory path.
// Dots and slashes are replaced with dashes.
func systemSlug(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	slug := strings.ToLower(abs)
	slug = strings.ReplaceAll(slug, string(filepath.Separator), "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// slugify converts an arbitrary string into a NATS-safe lowercase slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := b.String()
	// Collapse consecutive dashes
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

// contentHashShort returns the first 8 hex chars of the SHA-256 of s.
func contentHashShort(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}
