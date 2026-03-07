package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildDeltaEvent constructs a DELTA event for a set of changed entities.
func (e *Engine) buildDeltaEvent(entities []*federation.Entity) *federation.Event {
	flat := make([]federation.Entity, len(entities))
	for i, ptr := range entities {
		flat[i] = *ptr
	}
	return &federation.Event{
		Type:      federation.EventTypeDELTA,
		SourceID:  "semsource",
		Namespace: e.cfg.Namespace,
		Timestamp: time.Now(),
		Entities:  flat,
		Provenance: federation.Provenance{
			SourceType: "engine",
			SourceID:   "semsource",
			Timestamp:  time.Now(),
			Handler:    "engine",
		},
	}
}
