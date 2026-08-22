// Package statustest builds entity publishers in known delivery states, so a
// test can assert what a source's status report says about delivery without
// re-implementing the gated-publisher harness in every source package.
//
// It exists because the delivery figures (accepted/delivered/lost) are only
// meaningful when they disagree: a publisher that lost nothing cannot prove a
// report reads the publisher's counters rather than the source's own.
package statustest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
)

// wedged is a NATS publisher whose sends never complete, so the drain loop
// blocks on the first item and the buffer overflows behind it.
type wedged struct{ gate chan struct{} }

func (w *wedged) PublishToStream(ctx context.Context, _ string, _ []byte) error {
	select {
	case <-w.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *wedged) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return w.PublishToStream(ctx, subject, data)
}

// LossyPublisher returns a started publisher that has lost at least minLost
// entities to bounded-backpressure overflow and delivered none. Cleanup is
// registered on t. The publisher's Lost() is non-zero and Published() is zero,
// which is the disagreement a delivery-figure assertion needs.
func LossyPublisher(t *testing.T, minLost int) *entitypub.Publisher {
	t.Helper()
	w := &wedged{gate: make(chan struct{})}
	pub, err := entitypub.New(w, discardLogger(),
		entitypub.WithCapacity(1),
		entitypub.WithSendTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatalf("statustest: new publisher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pub.Start(ctx)
	t.Cleanup(func() {
		close(w.gate)
		cancel()
		pub.Stop()
	})

	// Send until enough entities have been dropped. The wedged drain loop holds
	// one item and the capacity-1 buffer holds one more, so every send past
	// that overflows after the send timeout.
	for i := 0; i < minLost+8 && int(pub.Lost()) < minLost; i++ {
		_ = pub.Send(&graph.EntityPayload{
			ID:                  "org.semsource.golang.sys.function.statustest",
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileContent,
		})
	}
	if got := int(pub.Lost()); got < minLost {
		t.Fatalf("statustest: Lost() = %d, want >= %d", got, minLost)
	}
	if got := pub.Published(); got != 0 {
		t.Fatalf("statustest: Published() = %d, want 0", got)
	}
	return pub
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
