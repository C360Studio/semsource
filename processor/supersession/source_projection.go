package supersession

import (
	"context"
	"encoding/json"
	"github.com/c360studio/semstreams/pkg/projection"
)

const sourceProjectionSubject = "graph.lifecycle.source"

type lifecycleMutator interface {
	Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error)
}

// handleSourceProjection retains a stateless refusal for clients of the retired API.
func (c *Component) handleSourceProjection(_ context.Context, _ []byte) ([]byte, error) {
	return json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code: "SOURCE_LIFECYCLE_UNAVAILABLE", Message: "source lifecycle projection is unavailable",
		},
	})
}
