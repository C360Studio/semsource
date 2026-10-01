package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/weburl"
	semconfig "github.com/c360studio/semstreams/config"
)

// sourceScopeConfig reads only identity inputs from each producer's persisted
// component config. In particular, instance names and the launcher's original
// file config are not the authority for sources registered after the first boot.
type sourceScopeConfig struct {
	WatchPaths []struct {
		Project string `json:"project"`
		Version string `json:"version"`
	} `json:"watch_paths"`
	Paths   []string `json:"paths"`
	URLs    []string `json:"urls"`
	Project string   `json:"project"`
	Version string   `json:"version"`
	Bucket  string   `json:"bucket"`
}

// sourceScopeSystems supplies the system segments admitted by each fusion lens.
// The caller passes the effective persisted component snapshot after config
// startup, then combines these systems with the effective platform authority
// and each lens's domains. This keeps ADR-102's taxonomy order from widening
// domain queries to a whole platform, or hiding runtime-registered sources.
func sourceScopeSystems(components semconfig.ComponentConfigs) (map[string][]string, error) {
	sets := map[string]map[string]bool{"code": {}, "docs": {}}
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		component := components[name]
		if !component.Enabled {
			continue
		}
		switch component.Name {
		case "ast-source", "doc-source", "cfgfile-source", "url-source", "objectstore-source":
		default:
			continue
		}
		var config sourceScopeConfig
		if err := json.Unmarshal(component.Config, &config); err != nil {
			return nil, fmt.Errorf("source scope for %s (%s): %w", name, component.Name, err)
		}
		lens := "docs"
		var systems []string
		switch component.Name {
		case "ast-source":
			lens = "code"
			for _, path := range config.WatchPaths {
				if path.Project == "" {
					return nil, fmt.Errorf("source scope for %s: watch path has no project", name)
				}
				systems = append(systems, entityid.ScopedSystemSlug(path.Project, path.Version))
			}
		case "doc-source", "cfgfile-source":
			roots := config.Paths
			if config.Project != "" {
				roots = []string{config.Project}
			}
			for _, root := range roots {
				system := entityid.SystemSlug(root)
				// ConfigHandler.systemSlug deliberately folds case; DocHandler.system
				// preserves it. Match the respective producer instead of normalizing both.
				if component.Name == "cfgfile-source" {
					system = strings.ToLower(system)
				}
				systems = append(systems, system)
			}
		case "url-source":
			for _, rawURL := range config.URLs {
				// URLHandler.domainSlug keys by hostname, ignoring scheme/path/port.
				domain := weburl.ExtractDomain(rawURL)
				system := "unknown"
				if domain != "" {
					system = entityid.SystemSlug(strings.ToLower(domain))
				}
				systems = append(systems, system)
			}
		case "objectstore-source":
			project := config.Project
			if project == "" {
				project = config.Bucket
			}
			if project == "" {
				return nil, fmt.Errorf("source scope for %s: object store has no project or bucket", name)
			}
			systems = append(systems, entityid.ScopedSystemSlug(project, config.Version))
		}
		for _, system := range systems {
			if system == "" {
				return nil, fmt.Errorf("source scope for %s: empty system", name)
			}
			sets[lens][system] = true
		}
	}
	return sortedScopeSystems(sets), nil
}

func sortedScopeSystems(sets map[string]map[string]bool) map[string][]string {
	result := map[string][]string{"code": {}, "docs": {}}
	for lens, systems := range sets {
		for system := range systems {
			result[lens] = append(result[lens], system)
		}
		sort.Strings(result[lens])
	}
	return result
}
