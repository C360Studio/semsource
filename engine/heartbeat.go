package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildHeartbeatEvent constructs a HEARTBEAT liveness event.
func (e *Engine) buildHeartbeatEvent() *federation.Event {
	return &federation.Event{
		Type:      federation.EventTypeHEARTBEAT,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Provenance: federation.Provenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
