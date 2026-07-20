package sourcemanifest

import (
	"encoding/json"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/types"
)

// systemsForRemovedInstance looks up instanceName's stored component config
// (before it is deleted) and derives the entity-ID system slug(s) its
// entities carry, so remove_source can scope the staleness lifecycle trigger
// (source_removed marking) to exactly what the removed source produced.
// Returns nil when the instance is unknown or its factory type isn't yet
// resolvable — a soft miss: unmarked entities on removal was already the
// pre-existing behavior for those cases, so this is purely additive.
func systemsForRemovedInstance(instanceName string, store sourcespawn.ConfigStore) []string {
	if store == nil {
		return nil
	}
	cfg := store.GetConfig().Get()
	if cfg == nil {
		return nil
	}
	cc, ok := cfg.Components[instanceName]
	if !ok {
		return nil
	}
	return systemsForComponentConfig(cc)
}

// systemsForComponentConfig decodes cc's stored JSON back into the
// system-identifying fields each sourcespawn builder used to construct it,
// mirroring the slug computation the runtime component itself performs.
// Switches on the factory name (cc.Name) since the field shape differs by
// source type. Unrecognized factories return nil.
func systemsForComponentConfig(cc types.ComponentConfig) []string {
	switch cc.Name {
	case "ast-source":
		var decoded struct {
			WatchPaths []struct {
				Project string `json:"project"`
				Version string `json:"version"`
			} `json:"watch_paths"`
		}
		if err := json.Unmarshal(cc.Config, &decoded); err != nil {
			return nil
		}
		systems := make([]string, 0, len(decoded.WatchPaths))
		for _, wp := range decoded.WatchPaths {
			if wp.Project == "" {
				continue
			}
			systems = append(systems, entityid.ScopedSystemSlug(wp.Project, wp.Version))
		}
		return systems

	case "doc-source", "cfgfile-source", "image-source", "video-source", "audio-source":
		var decoded struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(cc.Config, &decoded); err != nil {
			return nil
		}
		systems := make([]string, 0, len(decoded.Paths))
		for _, p := range decoded.Paths {
			if s := entityid.SystemSlug(p); s != "" {
				systems = append(systems, s)
			}
		}
		return systems

	case "url-source":
		var decoded struct {
			URLs []string `json:"urls"`
		}
		if err := json.Unmarshal(cc.Config, &decoded); err != nil {
			return nil
		}
		systems := make([]string, 0, len(decoded.URLs))
		for _, u := range decoded.URLs {
			if s := entityid.SystemSlug(u); s != "" {
				systems = append(systems, s)
			}
		}
		return systems

	case "git-source":
		var decoded struct {
			RepoPath   string `json:"repo_path"`
			RepoURL    string `json:"repo_url"`
			BranchSlug string `json:"branch_slug"`
		}
		if err := json.Unmarshal(cc.Config, &decoded); err != nil {
			return nil
		}
		identifier := decoded.RepoURL
		if identifier == "" {
			identifier = decoded.RepoPath
		}
		slug := entityid.SystemSlug(identifier)
		if slug == "" {
			return nil
		}
		return []string{entityid.BranchScopedSlug(slug, decoded.BranchSlug)}

	default:
		return nil
	}
}
