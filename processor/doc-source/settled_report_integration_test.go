//go:build integration

package docsource

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/internal/sourcestatus"
)

// heldDelivery holds every entity publish until released, standing in for a
// publisher that has not yet drained the seed.
type heldDelivery struct{ release chan struct{} }

func (h heldDelivery) PublishToStream(ctx context.Context, _ string, _ []byte) error {
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h heldDelivery) PublishToStreamWithMsgID(ctx context.Context, s string, d []byte, _ string) error {
	return h.PublishToStream(ctx, s, d)
}

// TestReportWhenSettled_PublishesDeliveredFiguresWithoutTheTick pins #284: the
// seed-end report is built before the publisher drains, so it reads
// delivered < offered. The settled report must follow once delivery completes,
// with no periodic reporter running — before the fix the figures corrected only
// on the 30s tick, and the required e2e-local settle check rode on that tick.
func TestReportWhenSettled_PublishesDeliveredFiguresWithoutTheTick(t *testing.T) {
	srv := natstest.New(t)
	reports := make(chan sourcestatus.Report, 8)
	sub, err := srv.Client.Subscribe(t.Context(), "semsource.internal.status", func(_ context.Context, msg *nats.Msg) {
		var r sourcestatus.Report
		if err := json.Unmarshal(msg.Data, &r); err != nil {
			t.Errorf("decode status report: %v", err)
			return
		}
		reports <- r
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	held := heldDelivery{release: make(chan struct{})}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub, err := entitypub.New(held, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.Start(ctx)

	c := &Component{publisher: pub, natsClient: srv.Client, logger: logger, distinct: entitypub.NewDistinctTracker()}
	c.config.InstanceName = "doc-settled"
	const seeded = 3
	for i := range seeded {
		p := &graph.EntityPayload{
			ID:                  "acme.semsource.docs.web.doc.d" + string(rune('a'+i)),
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileContent,
		}
		if err := pub.Send(p); err != nil {
			t.Fatal(err)
		}
		c.entitiesPublished.Add(1)
	}

	c.publishStatusReport(ctx, "watching")
	c.reportWhenSettled(ctx)
	first := receive(t, reports)
	if first.OfferedTotal != seeded || first.DeliveredTotal >= seeded {
		t.Fatalf("seed-end report = offered %d delivered %d; the harness must hold delivery so it reads incomplete",
			first.OfferedTotal, first.DeliveredTotal)
	}

	close(held.release)
	settled := receive(t, reports)
	if settled.DeliveredTotal != seeded || settled.OfferedTotal != seeded {
		t.Fatalf("settled report = offered %d delivered %d, want both %d", settled.OfferedTotal, settled.DeliveredTotal, seeded)
	}
	if settled.Phase != "watching" {
		t.Errorf("settled report phase = %q, want the current phase %q", settled.Phase, "watching")
	}

	cancel()
	if err := c.workers.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = pub.Stop(context.Background())
}

// receive bounds each wait well under the 30s periodic interval, so a report
// that only the tick could produce fails the test instead of passing late.
func receive(t *testing.T, reports <-chan sourcestatus.Report) sourcestatus.Report {
	t.Helper()
	select {
	case r := <-reports:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("no status report within 10s")
		return sourcestatus.Report{}
	}
}
