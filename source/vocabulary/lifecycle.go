package source

import "github.com/c360studio/semstreams/vocabulary"

// EntityLifecycleStale marks a retained entity whose backing source artifact
// no longer exists — the graph's staleness/retraction marker (ADR-0008's
// deferred deletion exception, entity-staleness spec). Presence-based (one
// value per entity); the value names why:
//
//   - LifecycleReasonFileDeleted   — a watched/reindexed file was deleted or renamed
//   - LifecycleReasonSourceRemoved — the owning source was deregistered (remove_source)
//   - LifecycleReasonPathMissing   — a periodic sweep found the backing path absent
//
// Registered WithWeight(-3.0), strictly below code.lineage.superseded-by's
// -2.0: a stale fact ranks under a historical-but-alive version, never above
// it. Cleared via the update lane's RemoveTriples when the artifact
// reappears — never left to coexist with a live fact.
const EntityLifecycleStale = "entity.lifecycle.stale"

// Reason values for EntityLifecycleStale.
const (
	LifecycleReasonFileDeleted   = "file_deleted"
	LifecycleReasonSourceRemoved = "source_removed"
	LifecycleReasonPathMissing   = "path_missing"
)

func init() {
	registerLifecyclePredicates()
}

func registerLifecyclePredicates() {
	vocabulary.Register(EntityLifecycleStale,
		vocabulary.WithDescription("Marks a retained entity whose source artifact no longer exists: file_deleted, source_removed, or path_missing"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"lifecycleStale"),
		vocabulary.WithWeight(-3.0))
}
