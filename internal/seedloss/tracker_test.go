package seedloss

import "testing"

func TestTracker_AttributesLossToThePassNotTheLifetime(t *testing.T) {
	var tr Tracker

	// A first pass that loses 3 of a lifetime total of 3.
	tr.Begin(0)
	tr.End(3)
	if got := tr.PassLost(); got != 3 {
		t.Fatalf("first pass: PassLost() = %d, want 3", got)
	}

	// A second, clean pass. The cumulative counter is still 3 — monotonic, never
	// reset — so a naive Lost() > 0 test would still read as lossy here. This is
	// the case that makes degraded clearable.
	tr.Begin(3)
	tr.End(3)
	if got := tr.PassLost(); got != 0 {
		t.Fatalf("clean second pass: PassLost() = %d, want 0 (cumulative loss is still 3)", got)
	}

	// A third pass that loses 2 more.
	tr.Begin(3)
	tr.End(5)
	if got := tr.PassLost(); got != 2 {
		t.Fatalf("third pass: PassLost() = %d, want 2", got)
	}
}

func TestTracker_InFlightPassKeepsThePreviousResult(t *testing.T) {
	var tr Tracker
	tr.Begin(0)
	tr.End(4)
	tr.Begin(4) // re-seed starts; previous result must stand until it completes
	if got := tr.PassLost(); got != 4 {
		t.Fatalf("PassLost() = %d during re-seed, want 4 (previous pass stands)", got)
	}
}

func TestTracker_ZeroValueReportsNoLoss(t *testing.T) {
	var tr Tracker
	if got := tr.PassLost(); got != 0 {
		t.Fatalf("zero value PassLost() = %d, want 0", got)
	}
}
