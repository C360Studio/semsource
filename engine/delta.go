package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildDeltaEvents constructs one DELTA event per changed entity.
func (e *Engine) buildDeltaEvents(entities []*federation.Entity) []*federation.Event {
	now := time.Now()
	events := make([]*federation.Event, len(entities))
	for i, ent := range entities {
		events[i] = &federation.Event{
			Type:      federation.EventTypeDELTA,
			SourceID:  "semsource",
			Namespace: e.cfg.Namespace,
			Timestamp: now,
			Entity:    *ent,
		}
	}
	return events
}
