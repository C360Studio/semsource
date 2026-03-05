package engine

import (
	"time"

	"github.com/c360studio/semsource/graph"
)

// buildDeltaEvent constructs a DELTA GraphEvent for a set of changed entities.
func (e *Engine) buildDeltaEvent(entities []*graph.GraphEntity) *graph.GraphEvent {
	flat := make([]graph.GraphEntity, len(entities))
	for i, ptr := range entities {
		flat[i] = *ptr
	}
	return &graph.GraphEvent{
		Type:      graph.EventTypeDELTA,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Entities:  flat,
		Provenance: graph.SourceProvenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
