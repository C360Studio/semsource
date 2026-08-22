package docsource

import (
	"testing"

	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/statustest"
)

// TestBuildStatusReport_DeliveryFiguresComeFromThePublisher pins the wiring
// this change exists to fix. Delivered and lost must be read from the
// publisher's confirmed counters, never from the source's own accept-side
// counter — Send() returns after a buffer write, before any delivery outcome
// exists. The assertion is only meaningful under loss, where the two disagree.
func TestBuildStatusReport_DeliveryFiguresComeFromThePublisher(t *testing.T) {
	pub := statustest.LossyPublisher(t, 1)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}
	c.config.InstanceName = "doc-1"
	const accepted = 7
	c.entitiesPublished.Store(accepted)

	r := c.buildStatusReport("watching")

	if r.LostTotal == 0 {
		t.Fatal("harness produced no loss; the assertion below proves nothing")
	}
	if r.AcceptedTotal != accepted {
		t.Errorf("AcceptedTotal = %d, want %d (the source's own counter)", r.AcceptedTotal, accepted)
	}
	if r.DeliveredTotal != pub.Published() {
		t.Errorf("DeliveredTotal = %d, want %d (publisher confirmed)", r.DeliveredTotal, pub.Published())
	}
	if r.LostTotal != pub.Lost() {
		t.Errorf("LostTotal = %d, want %d (publisher lost)", r.LostTotal, pub.Lost())
	}
	if r.AcceptedTotal < r.DeliveredTotal {
		t.Errorf("AcceptedTotal (%d) < DeliveredTotal (%d): acceptance must bound delivery",
			r.AcceptedTotal, r.DeliveredTotal)
	}
}
