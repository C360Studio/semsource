// Package sourcespawn translates config.SourceEntry values into the
// component configs the semstreams ServiceManager spawns reactively from
// the shared config KV bucket.
//
// The package is the single source of truth for "what does a source look
// like as a runtime component" — used by the startup loader, the
// branch-watcher, and the programmatic source-add API on source-manifest.
package sourcespawn

import (
	"fmt"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
)

// outputPorts returns the standard output port config shared by all source
// components. The flow engine reads ports from the config JSON to discover
// connections — without explicit ports in the config, the flow graph shows
// 0 connections even though components use DefaultConfig at runtime.
func outputPorts() map[string]any {
	return map[string]any{
		"outputs": []map[string]any{
			{
				"name":        "graph.ingest",
				"type":        "jetstream",
				"subject":     "graph.ingest.entity",
				"stream_name": "GRAPH",
				"required":    true,
				"description": "Entity state updates for graph ingestion",
			},
		},
	}
}

// astComponentConfig builds a component instance name and config map for an
// AST source entry. The instance name is derived from the path so multiple
// AST sources produce distinct component instances.
func astComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any, error) {
	path := src.Path
	if path == "" {
		path = "."
	}

	lang := src.Language
	if lang == "" {
		lang = "go"
	}

	slug := entityid.SystemSlug(path)
	if slug == "" {
		slug = fmt.Sprintf("source-%d", index)
	}
	project := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("ast-source-%s", project)

	indexInterval := src.IndexInterval
	if indexInterval == "" {
		indexInterval = "60s"
	}

	compCfg := map[string]any{
		"ports": outputPorts(),
		"watch_paths": []map[string]any{
			{
				"path":      path,
				"org":       org,
				"project":   project,
				"languages": []string{lang},
			},
		},
		"watch_enabled":  src.Watch,
		"index_interval": indexInterval,
		"instance_name":  instanceName,
	}

	return instanceName, compCfg, nil
}

// gitComponentConfig builds a component instance name and config map for a
// git source entry.
func gitComponentConfig(src config.SourceEntry, org string, index int, opts Options) (string, map[string]any, error) {
	identifier := src.URL
	if identifier == "" {
		identifier = src.Path
	}
	if identifier == "" {
		identifier = fmt.Sprintf("git-%d", index)
	}
	slug := entityid.SystemSlug(identifier)
	if slug == "" {
		slug = fmt.Sprintf("git-%d", index)
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("git-source-%s", scopedSlug)

	branch := src.Branch
	if branch == "" {
		branch = "main"
	}

	pollInterval := src.PollInterval
	if pollInterval == "" {
		pollInterval = "60s"
	}

	compCfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"repo_path":     src.Path,
		"repo_url":      src.URL,
		"branch":        branch,
		"poll_interval": pollInterval,
		"watch_enabled": src.Watch,
		"workspace_dir": opts.WorkspaceDir,
		"git_token":     opts.GitToken,
		"branch_slug":   src.BranchSlug,
		"instance_name": instanceName,
	}

	return instanceName, compCfg, nil
}

// docComponentConfig builds config for a docs source entry.
func docComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("docs-%d", index)
	if len(paths) > 0 {
		s := entityid.SystemSlug(paths[0])
		if s == "" {
			slug = fmt.Sprintf("docs-%d", index)
		} else {
			slug = s
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("doc-source-%s", scopedSlug)
	return instanceName, map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
}

// cfgfileComponentConfig builds config for a config-file source entry.
func cfgfileComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("config-%d", index)
	if len(paths) > 0 {
		s := entityid.SystemSlug(paths[0])
		if s == "" {
			slug = fmt.Sprintf("config-%d", index)
		} else {
			slug = s
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("cfgfile-source-%s", scopedSlug)
	return instanceName, map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
}

// urlComponentConfig builds config for a URL source entry.
func urlComponentConfig(src config.SourceEntry, org string, index int) (string, map[string]any) {
	urls := src.URLs
	if len(urls) == 0 && src.URL != "" {
		urls = []string{src.URL}
	}
	slug := fmt.Sprintf("url-%d", index)
	if len(urls) > 0 {
		s := entityid.SystemSlug(urls[0])
		if s == "" {
			slug = fmt.Sprintf("url-%d", index)
		} else {
			slug = s
		}
	}
	instanceName := fmt.Sprintf("url-source-%s", slug)
	pollInterval := src.PollInterval
	if pollInterval == "" {
		pollInterval = "300s"
	}
	return instanceName, map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"urls":          urls,
		"poll_interval": pollInterval,
		"instance_name": instanceName,
	}
}

// mediaComponentConfig builds config for image, video, or audio source entries.
func mediaComponentConfig(src config.SourceEntry, org string, index int, opts Options) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := fmt.Sprintf("%s-%d", src.Type, index)
	if len(paths) > 0 {
		s := entityid.SystemSlug(paths[0])
		if s != "" {
			slug = s
		}
	}
	instanceName := fmt.Sprintf("%s-source-%s", src.Type, slug)

	compCfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}

	if opts.MediaStoreDir != "" {
		compCfg["file_store_root"] = opts.MediaStoreDir
	}

	return instanceName, compCfg
}
