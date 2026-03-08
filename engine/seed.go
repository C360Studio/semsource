package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildSeedEvents constructs SEED events from the current store snapshot.
// One event per entity. If the store is empty, a single empty SEED event is
// emitted to signal that the initial ingest completed (liveness).
func (e *Engine) buildSeedEvents() []*federation.Event {
	entities := e.store.Snapshot()
	now := time.Now()

	if len(entities) == 0 {
		return []*federation.Event{{
			Type:      federation.EventTypeSEED,
			SourceID:  "semsource",
			Namespace: e.cfg.Namespace,
			Timestamp: now,
		}}
	}

	events := make([]*federation.Event, len(entities))
	for i, ent := range entities {
		events[i] = &federation.Event{
			Type:      federation.EventTypeSEED,
			SourceID:  "semsource",
			Namespace: e.cfg.Namespace,
			Timestamp: now,
			Entity:    *ent,
		}
	}
	return events
}
