package sourcemanifest

import "testing"

func seeded(name string, seedLost, errCount int64) *SourceStatusReport {
	return &SourceStatusReport{
		InstanceName: name,
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  100,
		SeedLost:     seedLost,
		ErrorCount:   errCount,
	}
}

// TestBuildStatus_SeedLossDegradesTheAggregate is the core of #177: a seed that
// finished while entities it offered never arrived must not be claimable as
// ready. The source is past its seed and not errored, which is exactly the
// state that previously reported ready with tens of thousands of entities
// missing.
func TestBuildStatus_SeedLossDegradesTheAggregate(t *testing.T) {
	agg := newStatusAggregator(1)
	agg.update(seeded("ast-source-main", 34871, 34871))

	if got := agg.buildStatus("ns").Phase; got != PhaseDegraded {
		t.Errorf("phase = %q, want %q: a seed that lost entities is not ready", got, PhaseDegraded)
	}
}

// TestBuildStatus_ParseFailuresAloneDoNotDegrade is the regression guard for
// the whole rationale of keying on SeedLost rather than ErrorCount. ErrorCount
// is a sum that includes parse failures, so degrading on it would put any
// corpus containing one unparseable file into a permanent degraded state — a
// phase that is always on, carrying no information.
func TestBuildStatus_ParseFailuresAloneDoNotDegrade(t *testing.T) {
	agg := newStatusAggregator(1)
	agg.update(seeded("ast-source-main", 0, 250)) // 250 unparseable files, nothing lost

	if got := agg.buildStatus("ns").Phase; got != PhaseReady {
		t.Errorf("phase = %q, want %q: parse failures are not delivery loss", got, PhaseReady)
	}
}

// TestBuildStatus_LossDegradationIsStickyUntilACleanPass pins that continued
// healthy activity cannot launder a lossy seed, and that a clean re-pass is
// what clears it — the recovery path the spec requires.
func TestBuildStatus_LossDegradationIsStickyUntilACleanPass(t *testing.T) {
	agg := newStatusAggregator(1)

	agg.update(seeded("ast-source-main", 12, 12))
	if got := agg.buildStatus("ns").Phase; got != PhaseDegraded {
		t.Fatalf("after lossy seed: phase = %q, want %q", got, PhaseDegraded)
	}

	// More watching reports carrying the same result must not clear it.
	for i := 0; i < 3; i++ {
		agg.update(seeded("ast-source-main", 12, 12))
		if got := agg.buildStatus("ns").Phase; got != PhaseDegraded {
			t.Fatalf("watch report %d: phase = %q, want %q (activity must not launder loss)",
				i, got, PhaseDegraded)
		}
	}

	// A re-seed in flight: still degraded, not plain "seeding" — the graph is
	// missing what the lossy pass dropped for the whole of the re-seed.
	reseeding := seeded("ast-source-main", 12, 12)
	reseeding.Phase = SourcePhaseIngesting
	agg.update(reseeding)
	if got := agg.buildStatus("ns").Phase; got != PhaseDegraded {
		t.Errorf("during re-seed: phase = %q, want %q", got, PhaseDegraded)
	}

	// The re-seed completes clean. Lifetime loss is unchanged and still
	// non-zero, but this pass lost nothing, so ready is the truthful answer.
	clean := seeded("ast-source-main", 0, 12)
	clean.LostTotal = 12
	agg.update(clean)
	if got := agg.buildStatus("ns").Phase; got != PhaseReady {
		t.Errorf("after clean re-pass: phase = %q, want %q (lifetime loss is monotonic and must not block forever)",
			got, PhaseReady)
	}
}

// TestBuildStatus_ReproducesIssue177Evidence pins the concrete run that
// motivated the change. beta.161 OSH acceptance, boot A: the publisher had
// delivered 42,931 entities and terminally failed 34,871, and status reported
// publish_total 77,802 with phase ready. A consumer honouring the documented
// gate proceeded against a graph missing 45% of what the headline claimed.
func TestBuildStatus_ReproducesIssue177Evidence(t *testing.T) {
	const (
		delivered = 42931
		lost      = 34871
		offered   = delivered + lost // 77,802 — the old publish_total
	)

	agg := newStatusAggregator(1)
	agg.update(&SourceStatusReport{
		InstanceName:   "ast-source-main",
		SourceType:     "ast",
		Phase:          SourcePhaseWatching,
		EntityCount:    delivered,
		OfferedTotal:   offered,
		DeliveredTotal: delivered,
		LostTotal:      lost,
		SeedLost:       lost,
		ErrorCount:     lost,
	})

	status := agg.buildStatus("ns")
	if status.Phase != PhaseDegraded {
		t.Errorf("phase = %q, want %q (this run previously reported ready)", status.Phase, PhaseDegraded)
	}

	got := status.Sources[0]
	for _, tc := range []struct {
		name      string
		got, want int64
	}{
		{"delivered_total", got.DeliveredTotal, delivered},
		{"lost_total", got.LostTotal, lost},
		{"seed_lost", got.SeedLost, lost},
		{"offered_total", got.OfferedTotal, offered},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	// The old headline number is still derivable — it was never wrong, only
	// misnamed. What changed is that it can no longer be mistaken for delivery.
	if got.OfferedTotal != 77802 {
		t.Errorf("offered_total = %d, want 77802 (the figure that shipped as publish_total)", got.OfferedTotal)
	}
	if got.DeliveredTotal >= got.OfferedTotal {
		t.Errorf("delivered (%d) must be strictly below offered (%d) under loss",
			got.DeliveredTotal, got.OfferedTotal)
	}
}
