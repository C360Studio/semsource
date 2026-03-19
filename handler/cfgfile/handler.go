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
	"sync"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// configFileNames is the set of file names that this handler processes.
var configFileNames = map[string]bool{
	"go.mod":       true,
	"package.json": true,
	"Dockerfile":   true,
	"pom.xml":      true,
	"build.gradle": true,
}

// Config controls optional ConfigHandler behaviour.
type Config struct {
	// WatchConfig is forwarded to the underlying FSWatcher.
	Watch fswatcher.WatchConfig

	// Org is the organisation namespace used when building typed EntityState
	// values via IngestEntityStates and Watch. Required for the normalizer-free
	// processor path.
	Org string
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
// Returns true when cfg.GetType() == "config" and at least one configured
// path exists on disk.
func (h *ConfigHandler) Supports(cfg handler.SourceConfig) bool {
	if cfg.GetType() != handler.SourceTypeConfig {
		return false
	}
	for _, p := range resolvePaths(cfg) {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Ingest implements handler.SourceHandler.
// It walks all configured paths, parses recognised config files, and returns
// the resulting RawEntity slice.
func (h *ConfigHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	paths := resolvePaths(cfg)
	if len(paths) == 0 {
		return nil, fmt.Errorf("cfgfile: at least one path is required")
	}

	var entities []handler.RawEntity

	for _, root := range paths {
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
	}

	return entities, nil
}

// IngestEntityStates walks all configured paths, parses recognised config
// files, and returns fully-typed entity states that embed vocabulary-predicate
// triples directly — bypassing the normalizer entirely. The org parameter is
// the organisation namespace (e.g. "acme") used in the 6-part entity ID.
func (h *ConfigHandler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	paths := resolvePaths(cfg)
	if len(paths) == 0 {
		return nil, fmt.Errorf("cfgfile: at least one path is required")
	}

	now := time.Now().UTC()
	var states []*handler.EntityState

	for _, root := range paths {
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

			parsed := h.parseFileEntityStates(base, path, content, root, org, now)
			states = append(states, parsed...)
			return nil
		})
		if err != nil && err != context.Canceled {
			return nil, fmt.Errorf("cfgfile: walk %s: %w", root, err)
		}
	}

	return states, nil
}

// Watch implements handler.SourceHandler.
// Returns nil, nil when watching is disabled.
// One FSWatcher is created per configured path; all event streams are merged
// into a single output channel that is closed when ctx is cancelled or all
// per-root watchers have stopped.
func (h *ConfigHandler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	paths := resolvePaths(cfg)
	if len(paths) == 0 {
		return nil, fmt.Errorf("cfgfile: at least one path is required for watching")
	}

	watchCfg := h.cfg.Watch
	watchCfg.Enabled = true
	// Watch recognised config file extensions; Dockerfile is extensionless but
	// the fanOut filter catches it by base name regardless.
	watchCfg.FileExtensions = []string{".mod", ".json", ".xml", ".gradle"}
	if cp, ok := cfg.(handler.CoalesceProvider); ok {
		watchCfg = watchCfg.WithCoalesceMs(cp.GetCoalesceMs())
	}

	out := make(chan handler.ChangeEvent, 64*len(paths))

	var watchers []*fswatcher.FSWatcher
	for _, root := range paths {
		fsw, err := fswatcher.New(watchCfg, root)
		if err != nil {
			// Stop any watchers already started before returning the error.
			for _, w := range watchers {
				_ = w.Stop()
			}
			return nil, fmt.Errorf("cfgfile: create watcher for %s: %w", root, err)
		}
		if err := fsw.Start(ctx); err != nil {
			for _, w := range watchers {
				_ = w.Stop()
			}
			return nil, fmt.Errorf("cfgfile: start watcher for %s: %w", root, err)
		}
		watchers = append(watchers, fsw)
	}

	// Fan all per-root event streams into out; close out when every stream ends.
	// Each goroutine is responsible for stopping its own watcher on exit.
	var wg sync.WaitGroup
	for i, fsw := range watchers {
		wg.Add(1)
		go func(w *fswatcher.FSWatcher, root string, events <-chan handler.ChangeEvent) {
			defer wg.Done()
			defer func() { _ = w.Stop() }()
			h.fanOut(ctx, root, events, out)
		}(fsw, paths[i], fsw.Events())
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// fanOut reads raw FSWatcher events for a single root, enriches them with
// parsed entities, and forwards them to out. The caller is responsible for
// closing out after all fanOut goroutines have returned.
func (h *ConfigHandler) fanOut(ctx context.Context, root string, in <-chan handler.ChangeEvent, out chan<- handler.ChangeEvent) {
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
			now := time.Now()
			entities := h.parseFile(base, ev.Path, content, root)
			enriched := handler.ChangeEvent{
				Path:      ev.Path,
				Operation: ev.Operation,
				Timestamp: now,
				Entities:  entities,
			}
			if h.cfg.Org != "" {
				enriched.EntityStates = h.parseFileEntityStates(base, ev.Path, content, root, h.cfg.Org, now.UTC())
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
	case "pom.xml":
		return h.pomEntities(content, path, system)
	case "build.gradle":
		return h.gradleEntities(content, path, system)
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

	// Dependency entities — instance is content hash of (module+version).
	// Edges live on the module entity so the normalizer uses the module's ID as FromID.
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
		})
		// Add requires edge on the module entity (first entity in slice).
		entities[0].Edges = append(entities[0].Edges, handler.RawEdge{
			FromHint: modInstance,
			ToHint:   instance,
			EdgeType: "requires",
			ToType:   "dependency",
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

	// Dependency entities.
	// Edges live on the package entity so the normalizer uses the package's ID as FromID.
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
		})
		entities[0].Edges = append(entities[0].Edges, handler.RawEdge{
			FromHint: pkgInstance,
			ToHint:   instance,
			EdgeType: "depends",
			ToType:   "dependency",
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
				"image":         img,
				"exposed_ports": result.ExposedPorts,
				"file_path":     path,
			},
		})
	}

	return entities
}

// pomEntities converts a ParsePOM result to RawEntity values.
// A project entity is always emitted; dependency and module child entities
// follow with appropriate edges.
func (h *ConfigHandler) pomEntities(content []byte, path, system string) []handler.RawEntity {
	result, err := ParsePOM(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse pom.xml failed", "path", path, "error", err)
		return nil
	}

	var entities []handler.RawEntity

	// Project entity — instance is the slugified "groupId:artifactId" coordinate.
	projectInstance := slugify(result.GroupID + ":" + result.ArtifactID)
	if projectInstance == "" {
		projectInstance = contentHashShort(path)
	}
	entities = append(entities, handler.RawEntity{
		SourceType: handler.SourceTypeConfig,
		Domain:     handler.DomainConfig,
		System:     system,
		EntityType: "project",
		Instance:   projectInstance,
		Properties: map[string]any{
			"group_id":    result.GroupID,
			"artifact_id": result.ArtifactID,
			"version":     result.Version,
			"packaging":   result.Packaging,
			"file_path":   path,
		},
	})

	// Dependency entities — edges live on the project entity.
	for _, dep := range result.Deps {
		instance := contentHashShort(dep.GroupID + ":" + dep.ArtifactID + "@" + dep.Version)
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "dependency",
			Instance:   instance,
			Properties: map[string]any{
				"name":    dep.GroupID + ":" + dep.ArtifactID,
				"version": dep.Version,
				"scope":   dep.Scope,
				"kind":    "maven",
			},
		})
		entities[0].Edges = append(entities[0].Edges, handler.RawEdge{
			FromHint: projectInstance, ToHint: instance, EdgeType: "requires", ToType: "dependency",
		})
	}

	// Module entities for multi-module POMs — edges live on the project entity.
	for _, mod := range result.Modules {
		modInstance := slugify(mod)
		if modInstance == "" {
			modInstance = contentHashShort(mod)
		}
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "module",
			Instance:   modInstance,
			Properties: map[string]any{
				"name":      mod,
				"file_path": path,
			},
		})
		entities[0].Edges = append(entities[0].Edges, handler.RawEdge{
			FromHint: projectInstance, ToHint: modInstance, EdgeType: "contains", ToType: "module",
		})
	}

	return entities
}

// gradleEntities converts a ParseGradle result to RawEntity values.
// A project entity is derived from the directory name since build.gradle files
// do not always declare a project name inline.
func (h *ConfigHandler) gradleEntities(content []byte, path, system string) []handler.RawEntity {
	result, err := ParseGradle(content)
	if err != nil {
		h.logger.Warn("cfgfile: parse build.gradle failed", "path", path, "error", err)
		return nil
	}

	var entities []handler.RawEntity

	// Project entity — instance derived from the containing directory name.
	dirName := filepath.Base(filepath.Dir(path))
	projectInstance := slugify(dirName)
	if projectInstance == "" {
		projectInstance = contentHashShort(path)
	}
	entities = append(entities, handler.RawEntity{
		SourceType: handler.SourceTypeConfig,
		Domain:     handler.DomainConfig,
		System:     system,
		EntityType: "project",
		Instance:   projectInstance,
		Properties: map[string]any{
			"name":      dirName,
			"file_path": path,
			"build":     "gradle",
		},
	})

	// Dependency entities — edges live on the project entity.
	for _, dep := range result.Deps {
		instance := contentHashShort(dep.Group + ":" + dep.Name + "@" + dep.Version)
		entities = append(entities, handler.RawEntity{
			SourceType: handler.SourceTypeConfig,
			Domain:     handler.DomainConfig,
			System:     system,
			EntityType: "dependency",
			Instance:   instance,
			Properties: map[string]any{
				"name":          dep.Group + ":" + dep.Name,
				"version":       dep.Version,
				"configuration": dep.Configuration,
				"kind":          "gradle",
			},
		})
		entities[0].Edges = append(entities[0].Edges, handler.RawEdge{
			FromHint: projectInstance, ToHint: instance, EdgeType: "requires", ToType: "dependency",
		})
	}

	return entities
}

// resolvePaths returns the list of filesystem paths to operate on for cfg.
// If cfg.GetPaths() is non-empty it is returned directly; otherwise the single
// path from cfg.GetPath() is wrapped in a slice so callers only deal with one
// code path. Returns nil when both sources are empty.
func resolvePaths(cfg handler.SourceConfig) []string {
	if paths := cfg.GetPaths(); len(paths) > 0 {
		return paths
	}
	if p := cfg.GetPath(); p != "" {
		return []string{p}
	}
	return nil
}

// systemSlug returns a NATS-safe system slug derived from the root directory path.
// Delegates to entityid.SystemSlug for consistent base-name extraction on
// absolute paths, then replaces dots (entity-ID segment separators) with hyphens.
func systemSlug(root string) string {
	slug := entityid.SystemSlug(root)
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, ".", "-")
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
