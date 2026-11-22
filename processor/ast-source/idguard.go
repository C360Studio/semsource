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
// the other file's entity.
func (c *Component) dropDuplicateIDs(results []*semsourceast.ParseResult) {
	seen := make(map[string]bool)
	for _, result := range results {
		if result == nil {
			continue
		}
		var dropped []string
		if result.FileEntity != nil && seen[result.FileEntity.ID] {
			for _, e := range result.Entities {
				if e != nil {
					dropped = append(dropped, e.ID)
				}
			}
			result.Entities = nil
			result.FileEntity = nil
			c.recordDropped(result.Path, dropped)
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
		c.recordDropped(result.Path, dropped)
	}
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
