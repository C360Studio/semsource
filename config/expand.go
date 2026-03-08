package config

import (
	"fmt"
	"path/filepath"

	"github.com/c360studio/semsource/workspace"
)

// ExpandRepoSources expands any "repo" meta-source entries into their
// component sources: git, ast, docs, and config. Non-repo entries are
// passed through unchanged. The git source is always first in each
// expansion group so it clones the repo before other handlers try to
// read the directory.
//
// workspaceDir is the base directory where repos will be cloned.
func ExpandRepoSources(sources []SourceEntry, workspaceDir string) ([]SourceEntry, error) {
	var expanded []SourceEntry
	for _, src := range sources {
		if src.Type != "repo" {
			expanded = append(expanded, src)
			continue
		}
		if src.URL == "" {
			return nil, fmt.Errorf("repo source: url is required")
		}

		slug := workspace.URLToSlug(src.URL)
		localPath := filepath.Join(workspaceDir, slug)

		// 1. Git source — clones the repo (auto-clone resolves URL → local path)
		expanded = append(expanded, SourceEntry{
			Type:   "git",
			URL:    src.URL,
			Branch: src.Branch,
			Watch:  src.Watch,
		})

		// 2. AST source — code structure analysis
		astEntry := SourceEntry{
			Type:  "ast",
			Path:  localPath,
			Watch: src.Watch,
		}
		if src.Language != "" {
			astEntry.Language = src.Language
		}
		expanded = append(expanded, astEntry)

		// 3. Docs source — markdown/text docs
		expanded = append(expanded, SourceEntry{
			Type:  "docs",
			Paths: []string{localPath},
			Watch: src.Watch,
		})

		// 4. Config source — go.mod, package.json, pom.xml, Dockerfile, etc.
		expanded = append(expanded, SourceEntry{
			Type:  "config",
			Paths: []string{localPath},
			Watch: src.Watch,
		})
	}
	return expanded, nil
}
