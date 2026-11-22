package astsource

import (
	"sort"
	"strings"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// maxCollisionIDsLogged bounds the IDs one warning names; the count is exact.
const maxCollisionIDsLogged = 5

// dropDuplicateIDs enforces one entity per ID across results, in place
// (entity-identity-safety, "One entity ID per declaration within a parse
// pass"). The first declaration in result order, then entity order, is kept;
// every later one is removed and counted on id_collisions, with one aggregated
// warning per file that lost a declaration.
//
// It must run before bodiesForResult: that map is keyed by entity ID and filled
// by concurrent workers, so two declarations under one ID would publish
// whichever body a goroutine attached last. Dropping beats publishing both,
// because graph-ingest merges same-key writes per (predicate, source) set and
// would keep statements from each, a hybrid entity with fabricated edges
// (#329).
//
// Parsers emit in source order, so the survivor is deterministic. Across
// results the initial index passes them in walk order, which is lexical.
//
// When a result's file entity itself collides with an earlier one (path
// sanitizing folds a/b.ts and a-b.ts onto one segment), the whole result is
// dropped: its symbols would otherwise hang off, and its folder would contain,
// the other file's entity. Those results' paths are returned so the caller
// can refuse them on the per-file watch path, which never sees the survivor.
//
// quiet, when non-nil, marks results whose drops were already counted by an
// earlier pass: the periodic reindex re-runs this over every file, and an
// unchanged file it will not republish must not re-count on every interval.
// A quiet result is deduplicated exactly like any other; only the count and
// the warning are skipped.
func (c *Component) dropDuplicateIDs(results []*semsourceast.ParseResult, quiet func(*semsourceast.ParseResult) bool) []string {
	seen := make(map[string]bool)
	var foldedFiles []string
	for _, result := range results {
		if result == nil {
			continue
		}
		counted := quiet == nil || !quiet(result)
		var dropped []string
		if result.FileEntity != nil && seen[result.FileEntity.ID] {
			for _, e := range result.Entities {
				if e != nil {
					dropped = append(dropped, e.ID)
				}
			}
			result.Entities = nil
			result.FileEntity = nil
			foldedFiles = append(foldedFiles, result.Path)
			if counted {
				c.recordDropped(result.Path, dropped)
			}
			continue
		}
		kept := result.Entities[:0]
		for _, e := range result.Entities {
			if e == nil {
				continue
			}
			if seen[e.ID] {
				dropped = append(dropped, e.ID)
				continue
			}
			seen[e.ID] = true
			kept = append(kept, e)
		}
		// Clear the tail so dropped entities are not retained by the backing array.
		for i := len(kept); i < len(result.Entities); i++ {
			result.Entities[i] = nil
		}
		result.Entities = kept
		if counted {
			c.recordDropped(result.Path, dropped)
		}
	}
	return foldedFiles
}

// refuseFoldedFile withholds a watch-path parse of a file the last full pass
// dropped as a folded duplicate (see pathWatcher.folded). Publishing it would
// write its declarations under the survivor's IDs, a hybrid only a reseed
// clears (graph-ingest never removes predicates, #336). Counted and warned
// like any other drop: this pass would have published those declarations.
func (c *Component) refuseFoldedFile(pw *pathWatcher, result *semsourceast.ParseResult) bool {
	if result == nil || !pw.isFolded(result.Path) {
		return false
	}
	var dropped []string
	for _, e := range result.Entities {
		if e != nil {
			dropped = append(dropped, e.ID)
		}
	}
	c.recordDropped(result.Path, dropped)
	return true
}

// recordDropped counts and logs, once per file, the declarations one pass dropped.
func (c *Component) recordDropped(path string, dropped []string) {
	if len(dropped) == 0 {
		return
	}
	c.idCollisions.Add(int64(len(dropped)))
	c.logger.Warn("dropped declarations that share an entity ID with an earlier one",
		"path", path,
		"dropped", len(dropped),
		"ids", sampleIDs(dropped))
}

func sampleIDs(ids []string) string {
	uniq := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	sort.Strings(uniq)
	if len(uniq) > maxCollisionIDsLogged {
		return strings.Join(uniq[:maxCollisionIDsLogged], ",") + ",..."
	}
	return strings.Join(uniq, ",")
}

// setFolded replaces the folded-path set with the latest full pass's verdict
// and returns the paths that left it. Every full pass (seed, periodic
// reindex) recomputes the set from scratch, so a fold whose survivor was
// deleted is un-dropped by the next reindex, which then promotes that file.
func (pw *pathWatcher) setFolded(paths []string) (released map[string]bool) {
	next := make(map[string]bool, len(paths))
	for _, p := range paths {
		next[p] = true
	}
	pw.foldedMu.Lock()
	defer pw.foldedMu.Unlock()
	for p := range pw.folded {
		if !next[p] {
			if released == nil {
				released = make(map[string]bool)
			}
			released[p] = true
		}
	}
	pw.folded = next
	return released
}

// isFolded reports whether the last full pass dropped path as a folded duplicate.
func (pw *pathWatcher) isFolded(path string) bool {
	pw.foldedMu.Lock()
	defer pw.foldedMu.Unlock()
	return pw.folded[path]
}
