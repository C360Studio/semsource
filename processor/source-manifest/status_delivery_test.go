package sourcemanifest

import "testing"

// TestBuildStatus_DeliveryFiguresReachTheAggregate pins that the delivery
// figures survive the producer -> aggregator -> payload hop. The shared report
// type makes losing them structurally hard, but the aggregate payload is a
// separate struct with its own mapping, which is exactly where the nine
// duck-typed copies used to drop fields (#188).
func TestBuildStatus_DeliveryFiguresReachTheAggregate(t *testing.T) {
	agg := newStatusAggregator(1)
	agg.update(&SourceStatusReport{
		InstanceName:   "ast-source-main",
		SourceType:     "ast",
		Phase:          SourcePhaseWatching,
		EntityCount:    120,
		OfferedTotal:   200,
		DeliveredTotal: 170,
		LostTotal:      30,
	})

	status := agg.buildStatus("ns")
	if len(status.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(status.Sources))
	}
	got := status.Sources[0]

	if got.OfferedTotal != 200 {
		t.Errorf("OfferedTotal = %d, want 200", got.OfferedTotal)
	}
	if got.DeliveredTotal != 170 {
		t.Errorf("DeliveredTotal = %d, want 170", got.DeliveredTotal)
	}
	if got.LostTotal != 30 {
		t.Errorf("LostTotal = %d, want 30", got.LostTotal)
	}
	// The identity the figures exist to make checkable, with nothing in flight.
	if got.OfferedTotal != got.DeliveredTotal+got.LostTotal {
		t.Errorf("accepted (%d) != delivered (%d) + lost (%d)",
			got.OfferedTotal, got.DeliveredTotal, got.LostTotal)
	}
}
