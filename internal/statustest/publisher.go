// Package statustest builds entity publishers in known, settled delivery
// states, so a test can assert what a source's status report says about
// delivery without re-implementing the harness in every source package.
//
// It exists because the delivery figures are only meaningful when they
// disagree: a publisher that lost nothing cannot prove a report reads the
// publisher's counters rather than the source's own.
package statustest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
)

// refusing is a NATS publisher that rejects every publish, so entities leave
// the buffer and fail terminally rather than sitting in flight.
type refusing struct{}

func (refusing) PublishToStream(context.Context, string, []byte) error {
	return errors.New("statustest: upstream refuses")
}

func (refusing) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return refusing{}.PublishToStream(ctx, subject, data)
}

// LossyPublisher returns a publisher that has offered n entities, delivered
// none, lost all of them, and then SETTLED — nothing left in flight — along
// with the number of sends the publisher accepted.
//
// Settling matters: `offered = delivered + lost + in-flight` only holds
// exactly at rest. Mid-flight, the drain loop holds an entity that has left
// the buffer without yet being resolved, so it counts in neither Pending() nor
// the terminal counters. Waiting for quiescence lets a caller assert the
// identity as equality rather than as a bound.
//
// The accepted count is what a real source's own hand-off counter would hold,
// so a caller can reconstruct the source side faithfully instead of inventing
// a number. Cleanup is registered on t.
func LossyPublisher(t *testing.T, n int) (*entitypub.Publisher, int64) {
	t.Helper()
	pub, err := entitypub.New(refusing{}, discardLogger(),
		entitypub.WithCapacity(max(n, 1)*2),
		entitypub.WithSendTimeout(time.Second),
		// Microsecond backoffs so the terminal-failure sequence completes in
		// milliseconds instead of running out the delivery budget.
		entitypub.WithRetryBackoff(time.Microsecond, 10*time.Microsecond))
	if err != nil {
		t.Fatalf("statustest: new publisher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	pub.Start(ctx)
	t.Cleanup(func() {
		cancel()
		pub.Stop()
	})

	var accepted int64
	for i := 0; i < n; i++ {
		if pub.Send(&graph.EntityPayload{
			ID:                  "org.semsource.golang.sys.function.statustest",
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileContent,
		}) == nil {
			accepted++
		}
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if pub.Pending() == 0 && pub.Published()+pub.Lost() >= accepted+pub.Dropped() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pub.Pending() != 0 {
		t.Fatalf("statustest: publisher never settled: pending=%d", pub.Pending())
	}
	if pub.Lost() == 0 {
		t.Fatal("statustest: no loss produced; a delivery assertion would prove nothing")
	}
	if got := pub.Published(); got != 0 {
		t.Fatalf("statustest: Published() = %d, want 0", got)
	}
	return pub, accepted
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
