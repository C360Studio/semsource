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
