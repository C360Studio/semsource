//go:build integration

package governance

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/miniotest"
	"github.com/c360studio/semsource/internal/sourcestatus"
)

// TestIntegration_UnboundedPrefixIngestMeasurement is a measurement, not a
// behavioral assertion, and it is here because the alternative was documenting
// a whole-bucket ingest as safe on the strength of nobody having tried it.
//
// An artifact bucket differs from a repository in the way that matters: it
// grows without anyone deciding to grow it. A prefix left empty means "the
// whole bucket", and #178 records the GRAPH stream refusing the tail of a
// large corpus. So the question is what an unbounded prefix actually does
// here, and the answer belongs in the change rather than in an assumption.
//
// What it establishes: a whole-bucket ingest of this size completes, publishes
// every document, and loses nothing. What it does NOT establish is a safe
// upper bound — it is one corpus at one size against a test stream, and the
// honest reading is in the change's evidence, not in the pass/fail of this
// test. It asserts only that nothing was lost, because a measurement that
// silently tolerated loss would be worse than none.
func TestIntegration_UnboundedPrefixIngestMeasurement(t *testing.T) {
	const (
		documents  = 1000
		bodyLength = 2048
	)

	ctx := context.Background()
	store, bucket := miniotest.NewStore(t)

	body := "# Report\n\n" + strings.Repeat("Quarterly findings and supporting detail. ", bodyLength/41) + "\n"
	seedConcurrently(t, documents, func(i int) error {
		return store.Put(ctx, fmt.Sprintf("corpus/%05d.md", i), []byte(body))
	})

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(10*time.Second, ingest.Stop) })

	reports := collectStatusReports(t, tc.Client)

	// No prefix: the whole bucket, which is the configuration this measurement
	// exists to characterize.
	started := time.Now()
	source := startObjectStoreSource(t, ctx, tc.Client, metricsRegistry, bucket, "")
	t.Cleanup(func() { _ = stopWithin(30*time.Second, source.Stop) })

	// Two figures, because they answer different questions and one of them is
	// easy to misreport. The source publishes "ready" the moment its seed pass
	// returns, so that report times the ingest itself; delivery is confirmed
	// separately, since the publisher is still draining when the pass ends.
	// Timing off the delivery report instead would measure the status
	// reporter's 30-second tick and call it ingest duration.
	awaitReport(t, reports, 5*time.Minute, func(r sourcestatus.Report) bool {
		return r.Phase == "ready"
	})
	ingestElapsed := time.Since(started)

	// Every document yields a parent and at least one passage, so the entity
	// count is the honest completion signal rather than the object count.
	report := awaitReport(t, reports, 5*time.Minute, func(r sourcestatus.Report) bool {
		return r.DeliveredTotal >= int64(documents*2)
	})

	t.Logf("unbounded-prefix ingest measurement: documents=%d body_bytes=%d "+
		"ingest_elapsed=%s entities=%d offered=%d delivered=%d lost=%d seed_lost=%d "+
		"backpressure=%t skipped=%v",
		documents, len(body), ingestElapsed.Round(time.Millisecond),
		report.EntityCount, report.OfferedTotal, report.DeliveredTotal,
		report.LostTotal, report.SeedLost, report.Backpressure, report.ObjectsSkipped)

	// The one assertion. A measurement that tolerated loss without saying so
	// would be evidence for the wrong conclusion.
	if report.LostTotal != 0 {
		t.Errorf("the ingest lost %d entities — a whole-bucket prefix hit a ceiling at %d documents; "+
			"prefix-scoping guidance belongs in the docs before this is called unbounded-safe",
			report.LostTotal, documents)
	}
}

// seedConcurrently writes n objects through a bounded pool. Sequential writes
// are the slowest part of this measurement by an order of magnitude, and
// nothing here depends on order.
func seedConcurrently(t *testing.T, n int, put func(i int) error) {
	t.Helper()

	const writers = 16
	indexes := make(chan int, n)
	for i := range n {
		indexes <- i
	}
	close(indexes)

	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indexes {
				if err := put(i); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("seed the bucket: %v", err)
	}
}
