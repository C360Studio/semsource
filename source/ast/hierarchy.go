package ast

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semsource/entityid"
)

// DomainCode is the language-agnostic domain for structural container entities
// (repos and folders). Folders can contain files in multiple languages, so
// using a language-specific domain would break deduplication.
const DomainCode = "code"

// BuildHierarchy produces repo and folder entities with containment edges
// from a batch of ParseResults. It also mutates the file entities in each
// ParseResult to set ContainedBy to their immediate parent (folder or repo).
//
// Returns nil when results is empty.
func BuildHierarchy(results []*ParseResult, authority entityid.Authority, project string) []*CodeEntity {
	if len(results) == 0 {
		return nil
	}
	files := make(map[string]string, len(results))
	for _, r := range results {
		if r.FileEntity == nil {
			continue
		}
		files[r.Path] = r.FileEntity.ID
		r.FileEntity.ContainedBy = FileParentID(r.Path, authority, project)
	}
	return BuildHierarchyFromFiles(files, authority, project)
}

// BuildHierarchyFromFiles produces the repo entity and every folder entity
// for a set of indexed files (root-relative path → file entity ID), each with
// its complete Contains list: child folders and files, sorted. The repo is
// always first, then folders shallowest first. An empty set yields the repo
// alone, with no Contains.
func BuildHierarchyFromFiles(files map[string]string, authority entityid.Authority, project string) []*CodeEntity {
	now := time.Now()
	systemSlug := entityid.SystemSlug(project)
	repo := &CodeEntity{
		ID:        buildRepoID(authority, systemSlug),
		Type:      TypeRepo,
		Name:      project,
		IndexedAt: now,
	}

	folderPaths := make(map[string]bool)
	filesByDir := make(map[string][]string) // dir → file entity IDs
	for path, id := range files {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = ""
		}
		filesByDir[dir] = append(filesByDir[dir], id)
		expandAncestors(dir, folderPaths)
	}

	sortedPaths := sortFolderPaths(folderPaths)
	folders := make(map[string]*CodeEntity, len(sortedPaths))
	for _, p := range sortedPaths {
		folders[p] = newFolderEntity(authority, systemSlug, p, now)
	}
	wireContainment(repo, folders, sortedPaths)
	populateFolderFiles(folders, filesByDir)
	populateRepoFiles(repo, filesByDir)
	sortContains(repo, folders)

	out := make([]*CodeEntity, 0, 1+len(folders))
	out = append(out, repo)
	for _, p := range sortedPaths {
		out = append(out, folders[p])
	}
	return out
}

// EmptyFolder returns the folder entity for folderPath with its parent edge
// and no Contains: what a folder left with no indexed file publishes so its
// stale contains edges clear.
func EmptyFolder(folderPath string, authority entityid.Authority, project string) *CodeEntity {
	f := newFolderEntity(authority, entityid.SystemSlug(project), folderPath, time.Now())
	f.ContainedBy = FileParentID(folderPath, authority, project)
	return f
}

// FileParentID returns the ID of the folder or repo that directly contains the
// file at filePath (root-relative), matching what BuildHierarchy assigns. The
// watch path uses it so a single-file publish carries the same
// code.structure.belongs edge as the full index (#336 task 4.1a).
func FileParentID(filePath string, authority entityid.Authority, project string) string {
	systemSlug := entityid.SystemSlug(project)
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return buildRepoID(authority, systemSlug)
	}
	return authority.Build(systemSlug, DomainCode, string(TypeFolder), SanitizePathSegment(dir))
}

// buildRepoID constructs the deterministic entity ID for a repository.
// systemSlug must already be processed through entityid.SystemSlug().
func buildRepoID(authority entityid.Authority, systemSlug string) string {
	return authority.Build(systemSlug, DomainCode, string(TypeRepo), SanitizePathSegment(systemSlug))
}

// newFolderEntity creates a CodeEntity for a directory path.
// systemSlug must already be processed through entityid.SystemSlug().
func newFolderEntity(authority entityid.Authority, systemSlug, folderPath string, now time.Time) *CodeEntity {
	name := filepath.Base(folderPath)
	return &CodeEntity{
		ID:        authority.Build(systemSlug, DomainCode, string(TypeFolder), SanitizePathSegment(folderPath)),
		Type:      TypeFolder,
		Name:      name,
		Path:      folderPath,
		IndexedAt: now,
	}
}

// expandAncestors adds dir and all its ancestors to the set.
func expandAncestors(dir string, set map[string]bool) {
	if dir == "" || dir == "." {
		return
	}
	for d := dir; d != "." && d != ""; d = filepath.Dir(d) {
		if set[d] {
			break // Already expanded this path and ancestors.
		}
		set[d] = true
	}
}

// sortFolderPaths returns folder paths sorted by depth then lexicographically.
func sortFolderPaths(paths map[string]bool) []string {
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Slice(sorted, func(i, j int) bool {
		di := strings.Count(sorted[i], "/")
		dj := strings.Count(sorted[j], "/")
		if di != dj {
			return di < dj
		}
		return sorted[i] < sorted[j]
	})
	return sorted
}

// wireContainment sets ContainedBy and Contains between folders and the repo.
func wireContainment(repo *CodeEntity, folders map[string]*CodeEntity, sortedPaths []string) {
	for _, p := range sortedPaths {
		parent := filepath.Dir(p)
		if parent == "." || parent == "" {
			// Top-level folder belongs to repo.
			folders[p].ContainedBy = repo.ID
			repo.Contains = append(repo.Contains, folders[p].ID)
		} else if pf, ok := folders[parent]; ok {
			folders[p].ContainedBy = pf.ID
			pf.Contains = append(pf.Contains, folders[p].ID)
		}
	}
}

// populateFolderFiles adds file entity IDs to each folder's Contains list.
func populateFolderFiles(folders map[string]*CodeEntity, filesByDir map[string][]string) {
	for dir, fileIDs := range filesByDir {
		if f, ok := folders[dir]; ok {
			f.Contains = append(f.Contains, fileIDs...)
		}
	}
}

// populateRepoFiles adds root-level file entity IDs to the repo's Contains list.
func populateRepoFiles(repo *CodeEntity, filesByDir map[string][]string) {
	if rootFiles, ok := filesByDir[""]; ok {
		repo.Contains = append(repo.Contains, rootFiles...)
	}
}

// sortContains sorts all Contains lists for deterministic output.
func sortContains(repo *CodeEntity, folders map[string]*CodeEntity) {
	sort.Strings(repo.Contains)
	for _, f := range folders {
		sort.Strings(f.Contains)
	}
}
