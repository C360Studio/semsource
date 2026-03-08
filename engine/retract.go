package engine

import (
	"time"

	"github.com/c360studio/semstreams/federation"
)

// buildRetractEvent constructs a RETRACT event for a set of entity IDs to remove.
func (e *Engine) buildRetractEvent(entityIDs []string) *federation.Event {
	return &federation.Event{
		Type:        federation.EventTypeRETRACT,
		SourceID:    "semsource",
		Namespace:   e.cfg.Namespace,
		Timestamp:   time.Now(),
		Retractions: entityIDs,
	}
}
