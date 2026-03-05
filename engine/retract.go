package engine

import (
	"time"

	"github.com/c360studio/semsource/graph"
)

// buildRetractEvent constructs a RETRACT GraphEvent for a set of entity IDs to remove.
func (e *Engine) buildRetractEvent(entityIDs []string) *graph.GraphEvent {
	return &graph.GraphEvent{
		Type:        graph.EventTypeRETRACT,
		SourceID:    "semsource",
		Namespace:   e.cfg.Namespace,
		Timestamp:   time.Now(),
		Retractions: entityIDs,
		Provenance: graph.SourceProvenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
