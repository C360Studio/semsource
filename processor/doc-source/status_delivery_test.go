package docsource

import (
	"testing"

	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/statustest"
)

// TestBuildStatusReport_DeliveryFiguresReconcile pins the wiring this change
// exists to fix, and the arithmetic that makes it legible.
//
// Delivered and lost must be read from the publisher's confirmed counters,
// never from the source's own hand-off counter — Send() returns after a buffer
// write, before any delivery outcome exists. Offered must include entities the
// publisher refused on overflow: a drop is in LostTotal, so leaving it out of
// OfferedTotal would put it on one side of the identity only.
//
// The assertion is driven under induced loss, where the figures disagree.
func TestBuildStatusReport_DeliveryFiguresReconcile(t *testing.T) {
	pub, accepted := statustest.LossyPublisher(t, 2)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}
	c.config.InstanceName = "doc-1"
	c.entitiesPublished.Store(accepted)

	r := c.buildStatusReport("watching")

	if r.LostTotal == 0 {
		t.Fatal("harness produced no loss; the assertions below would prove nothing")
	}
	if r.DeliveredTotal != pub.Published() {
		t.Errorf("DeliveredTotal = %d, want %d (publisher confirmed)", r.DeliveredTotal, pub.Published())
	}
	if r.LostTotal != pub.Lost() {
		t.Errorf("LostTotal = %d, want %d (publisher lost)", r.LostTotal, pub.Lost())
	}
	if want := accepted + pub.Dropped(); r.OfferedTotal != want {
		t.Errorf("OfferedTotal = %d, want %d (hand-offs the publisher took, plus those it refused)",
			r.OfferedTotal, want)
	}
	if got, want := r.OfferedTotal, r.DeliveredTotal+r.LostTotal+int64(pub.Pending()); got != want {
		t.Errorf("identity broken: offered = %d, but delivered (%d) + lost (%d) + in-flight (%d) = %d",
			got, r.DeliveredTotal, r.LostTotal, pub.Pending(), want)
	}
}

// TestBuildStatusReport_SeedLossIsPerPassNotLifetime pins the distinction that
// makes a loss-degraded readiness phase clearable. The publisher's loss
// counters are monotonic, so a source that lost entities once reports non-zero
// LostTotal for the rest of the process. SeedLost must instead answer "did the
// most recently completed pass lose anything", which a clean re-seed can reset.
func TestBuildStatusReport_SeedLossIsPerPassNotLifetime(t *testing.T) {
	pub, _ := statustest.LossyPublisher(t, 2)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}

	// A lossy pass: the baseline is taken before the loss.
	c.seedLoss.Begin(0)
	c.seedLoss.End(pub.Lost())
	lossy := c.buildStatusReport("watching")
	if lossy.SeedLost != pub.Lost() {
		t.Errorf("lossy pass: SeedLost = %d, want %d", lossy.SeedLost, pub.Lost())
	}

	// A clean pass that follows it: lifetime loss is unchanged and still
	// non-zero, but nothing was lost during this pass.
	c.seedLoss.Begin(pub.Lost())
	c.seedLoss.End(pub.Lost())
	clean := c.buildStatusReport("watching")
	if clean.LostTotal == 0 {
		t.Fatal("lifetime loss cleared; the assertion below would prove nothing")
	}
	if clean.SeedLost != 0 {
		t.Errorf("clean pass: SeedLost = %d, want 0 (lifetime loss is still %d)",
			clean.SeedLost, clean.LostTotal)
	}
}
