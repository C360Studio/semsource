package engine

import (
	"time"

	"github.com/c360studio/semsource/graph"
)

// buildHeartbeatEvent constructs a HEARTBEAT liveness event.
func (e *Engine) buildHeartbeatEvent() *graph.GraphEvent {
	return &graph.GraphEvent{
		Type:      graph.EventTypeHEARTBEAT,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Provenance: graph.SourceProvenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
