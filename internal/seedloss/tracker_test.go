package seedloss

import "testing"

func TestTracker_ReportsLossSinceThePassBeganNotTheLifetime(t *testing.T) {
	var tr Tracker

	// A first pass that loses 3 of a lifetime total of 3.
	tr.Begin(0)
	if got := tr.LostSince(3); got != 3 {
		t.Fatalf("first pass: LostSince(3) = %d, want 3", got)
	}

	// A re-seed. Lifetime loss is still 3 — monotonic, never reset — so a naive
	// Lost() > 0 test would still read as lossy. This is the case that makes a
	// degraded phase clearable.
	tr.Begin(3)
	if got := tr.LostSince(3); got != 0 {
		t.Fatalf("clean re-seed: LostSince(3) = %d, want 0 (lifetime loss is still 3)", got)
	}

	// Loss arriving during the drain, after the walk finished, still counts
	// against this pass — the reason the figure is live rather than frozen.
	if got := tr.LostSince(5); got != 2 {
		t.Fatalf("drain-time loss: LostSince(5) = %d, want 2", got)
	}
}

func TestTracker_ZeroValueAndBackwardsCounter(t *testing.T) {
	var tr Tracker
	if got := tr.LostSince(0); got != 0 {
		t.Fatalf("zero value: LostSince(0) = %d, want 0", got)
	}
	tr.Begin(10)
	if got := tr.LostSince(4); got != 0 {
		t.Fatalf("backwards counter: LostSince(4) = %d, want 0, not a negative repair", got)
	}
}
