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
