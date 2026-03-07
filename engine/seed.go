package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildSeedEvent constructs a SEED event from the current store snapshot.
func (e *Engine) buildSeedEvent() *federation.Event {
	entities := e.store.Snapshot()
	return &federation.Event{
		Type:      federation.EventTypeSEED,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Entities:  entities,
		Provenance: federation.Provenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
