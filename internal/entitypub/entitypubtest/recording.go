// Package entitypubtest holds test doubles for the entity publisher.
package entitypubtest

import (
	"context"
	"errors"
	"sync"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/projection"
)

// RecordingReconciler records which entities the publisher reconciled. With
// no graph behind it, every reconcile ends at its first read, so it suits
// tests that check which publishes reconcile, not what a reconcile writes.
type RecordingReconciler struct {
	mu  sync.Mutex
	ids []string
}

// ReadAuthoritative is the first call of every reconcile the publisher runs,
// so recording it records which entities were reconciled.
func (r *RecordingReconciler) ReadAuthoritative(_ context.Context, id string) (*gtypes.ExactEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
	return nil, errors.New("recording reconciler: no graph")
}

// Reconcile is never reached: ReadAuthoritative always fails.
func (r *RecordingReconciler) Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error) {
	return projection.MutationReceipt{}, errors.New("recording reconciler: no graph")
}

// IDs returns the reconciled entity IDs in order.
func (r *RecordingReconciler) IDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}
