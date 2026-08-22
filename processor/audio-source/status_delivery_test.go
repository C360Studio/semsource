package audiosource

import (
	"testing"

	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/statustest"
)

// TestBuildStatusReport_DeliveryFiguresComeFromThePublisher pins the wiring
// this change exists to fix: delivered and lost must be read from the
// publisher's confirmed counters, never from the source's own hand-off
// counter — Send() returns after a buffer write, before any delivery outcome
// exists. Offered includes entities the publisher refused on overflow, since
// those are counted as lost. The assertion is driven under induced loss, where
// the figures disagree; with no loss they would agree for the wrong reason.
func TestBuildStatusReport_DeliveryFiguresComeFromThePublisher(t *testing.T) {
	pub, accepted := statustest.LossyPublisher(t, 2)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}
	c.config.InstanceName = "audio-1"
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
}

// TestBuildStatusReport_SeedLossIsPerPassNotLifetime pins the distinction that
// makes a loss-degraded readiness phase clearable. The publisher's counters are
// monotonic, so a source that lost entities once reports non-zero LostTotal for
// the rest of the process. SeedLost answers "has this pass lost anything",
// which a clean re-seed resets.
func TestBuildStatusReport_SeedLossIsPerPassNotLifetime(t *testing.T) {
	pub, _ := statustest.LossyPublisher(t, 2)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}

	// A pass whose baseline predates the loss carries it.
	c.seedLoss.Begin(0)
	if lossy := c.buildStatusReport("watching"); lossy.SeedLost != pub.Lost() {
		t.Errorf("lossy pass: SeedLost = %d, want %d", lossy.SeedLost, pub.Lost())
	}

	// A re-seed beginning after it does not. Lifetime loss is unchanged and
	// still non-zero, which is the point: it can never fall back on its own.
	c.seedLoss.Begin(pub.Lost())
	clean := c.buildStatusReport("watching")
	if clean.LostTotal == 0 {
		t.Fatal("lifetime loss cleared; the assertion below would prove nothing")
	}
	if clean.SeedLost != 0 {
		t.Errorf("clean pass: SeedLost = %d, want 0 (lifetime loss is still %d)",
			clean.SeedLost, clean.LostTotal)
	}
}
