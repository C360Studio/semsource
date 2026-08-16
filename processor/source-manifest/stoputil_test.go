//go:build integration

package sourcemanifest

import (
	"context"
	"time"
)

// stopWithin bounds a component Stop with a fresh shutdown context. Tests own
// their context roots under the semstreams caller-owned lifecycle contract; a
// timeout here is a failure bound, not synchronization.
func stopWithin(d time.Duration, stop func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return stop(ctx)
}
