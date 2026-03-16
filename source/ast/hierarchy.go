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
func BuildHierarchy(results []*ParseResult, org, project string) []*CodeEntity {
	if len(results) == 0 {
		return nil
	}

	now := time.Now()
	systemSlug := entityid.SystemSlug(project)
	repoID := buildRepoID(org, systemSlug)

	repo := &CodeEntity{
		ID:        repoID,
		Type:      TypeRepo,
		Name:      project,
		IndexedAt: now,
	}

	// Collect all unique folder paths and map files to their parent folder.
	folderPaths := make(map[string]bool)
	filesByDir := make(map[string][]string) // dir → file entity IDs

	for _, r := range results {
		if r.FileEntity == nil {
			continue
		}
		dir := filepath.Dir(r.Path)
		if dir == "." {
			dir = ""
		}
		filesByDir[dir] = append(filesByDir[dir], r.FileEntity.ID)
		expandAncestors(dir, folderPaths)
	}

	// Build folder entities sorted by depth (shallowest first).
	sortedPaths := sortFolderPaths(folderPaths)
	folders := make(map[string]*CodeEntity, len(sortedPaths))

	for _, p := range sortedPaths {
		folders[p] = newFolderEntity(org, systemSlug, p, now)
	}

	// Wire containment: folder → parent, folder → children, repo → top-level.
	wireContainment(repo, folders, sortedPaths)

	// Assign files to their immediate parent (folder or repo).
	assignFilesToParents(results, repo, folders)

	// Collect files in folders to their Contains lists.
	populateFolderFiles(folders, filesByDir)
	populateRepoFiles(repo, filesByDir)

	// Sort all Contains lists for deterministic output.
	sortContains(repo, folders)

	// Assemble return slice: repo first, then folders in depth order.
	out := make([]*CodeEntity, 0, 1+len(folders))
	out = append(out, repo)
	for _, p := range sortedPaths {
		out = append(out, folders[p])
	}
	return out
}

// BuildFolderChain returns folder entities for all ancestor directories of
// filePath, with the top-level folder's ContainedBy set to the repo entity ID.
// Does not return a repo entity (assumed already published during initial index).
func BuildFolderChain(filePath, org, project string) []*CodeEntity {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return nil
	}

	now := time.Now()
	systemSlug := entityid.SystemSlug(project)
	repoID := buildRepoID(org, systemSlug)

	folderPaths := make(map[string]bool)
	expandAncestors(dir, folderPaths)

	sortedPaths := sortFolderPaths(folderPaths)
	folders := make(map[string]*CodeEntity, len(sortedPaths))

	for _, p := range sortedPaths {
		folders[p] = newFolderEntity(org, systemSlug, p, now)
	}

	// Wire parent relationships between folders, linking top-level to repo.
	for _, p := range sortedPaths {
		parent := filepath.Dir(p)
		if parent == "." || parent == "" {
			folders[p].ContainedBy = repoID
		} else if pf, ok := folders[parent]; ok {
			folders[p].ContainedBy = pf.ID
			pf.Contains = append(pf.Contains, folders[p].ID)
		}
	}

	out := make([]*CodeEntity, 0, len(sortedPaths))
	for _, p := range sortedPaths {
		out = append(out, folders[p])
	}
	return out
}

// buildRepoID constructs the deterministic entity ID for a repository.
// systemSlug must already be processed through entityid.SystemSlug().
func buildRepoID(org, systemSlug string) string {
	return entityid.Build(org, entityid.PlatformSemsource, DomainCode, systemSlug, string(TypeRepo), SanitizePathSegment(systemSlug))
}

// newFolderEntity creates a CodeEntity for a directory path.
// systemSlug must already be processed through entityid.SystemSlug().
func newFolderEntity(org, systemSlug, folderPath string, now time.Time) *CodeEntity {
	name := filepath.Base(folderPath)
	return &CodeEntity{
		ID:        entityid.Build(org, entityid.PlatformSemsource, DomainCode, systemSlug, string(TypeFolder), SanitizePathSegment(folderPath)),
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

// assignFilesToParents sets ContainedBy on each file entity.
func assignFilesToParents(results []*ParseResult, repo *CodeEntity, folders map[string]*CodeEntity) {
	for _, r := range results {
		if r.FileEntity == nil {
			continue
		}
		dir := filepath.Dir(r.Path)
		if dir == "." {
			dir = ""
		}
		if f, ok := folders[dir]; ok {
			r.FileEntity.ContainedBy = f.ID
		} else {
			r.FileEntity.ContainedBy = repo.ID
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
