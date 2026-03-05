package engine

import (
	"time"

	"github.com/c360studio/semsource/graph"
)

// buildSeedEvent constructs a SEED GraphEvent from the current store snapshot.
func (e *Engine) buildSeedEvent() *graph.GraphEvent {
	entities := e.store.Snapshot()
	return &graph.GraphEvent{
		Type:      graph.EventTypeSEED,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Entities:  entities,
		Provenance: graph.SourceProvenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
