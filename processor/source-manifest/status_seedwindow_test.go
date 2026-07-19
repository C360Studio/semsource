package sourcemanifest

import "testing"

// These tests pin the honest-readiness gate (audit 2026-07-19): ready means
// every source finished its initial seed, not merely reported. The previous
// suite only ever fed "watching" reports, which is exactly how the mid-seed
// window went untested while the gate was broken in production.

func report(instance, phase string) *SourceStatusReport {
	return &SourceStatusReport{
		InstanceName: instance,
		SourceType:   "ast",
		Phase:        phase,
		EntityCount:  1,
	}
}

// TestStatusAggregator_MidSeedWindowIsSeeding: all sources REPORTED but one is
// still ingesting — the documented consumer gate must not pass.
func TestStatusAggregator_MidSeedWindowIsSeeding(t *testing.T) {
	agg := newStatusAggregator(2)
	agg.update(report("a", SourcePhaseWatching))
	agg.update(report("b", SourcePhaseIngesting))

	if !agg.allReported() {
		t.Fatal("precondition: all sources reported")
	}
	if got := agg.buildStatus("acme").Phase; got != PhaseSeeding {
		t.Errorf("phase = %q during mid-seed window, want %q (the audit's broken gate)", got, PhaseSeeding)
	}
}

// TestStatusAggregator_ReadyAfterLastSeedCompletes: the ready transition
// happens when the LAST source finishes seeding, not when it first reports.
func TestStatusAggregator_ReadyAfterLastSeedCompletes(t *testing.T) {
	agg := newStatusAggregator(2)
	agg.update(report("a", SourcePhaseIngesting))
	agg.update(report("b", SourcePhaseIngesting))
	if got := agg.buildStatus("acme").Phase; got != PhaseSeeding {
		t.Fatalf("phase = %q with both ingesting, want %q", got, PhaseSeeding)
	}

	agg.update(report("a", SourcePhaseWatching))
	if got := agg.buildStatus("acme").Phase; got != PhaseSeeding {
		t.Errorf("phase = %q with one source still ingesting, want %q", got, PhaseSeeding)
	}

	agg.update(report("b", SourcePhaseIdle))
	if got := agg.buildStatus("acme").Phase; got != PhaseReady {
		t.Errorf("phase = %q after all seeds complete, want %q", got, PhaseReady)
	}
}

// TestStatusAggregator_ErroredSourceDegrades: an errored source degrades the
// aggregate even when everything has reported (the old else-if made the
// degraded branch unreachable once all reports were in).
func TestStatusAggregator_ErroredSourceDegrades(t *testing.T) {
	agg := newStatusAggregator(2)
	agg.update(report("a", SourcePhaseWatching))
	agg.update(report("b", SourcePhaseErrored))

	if got := agg.buildStatus("acme").Phase; got != PhaseDegraded {
		t.Errorf("phase = %q with an errored source, want %q", got, PhaseDegraded)
	}
}

// TestStatusAggregator_TimeoutDegradedClearsOnCleanSeed: a seed-timeout
// degradation is transient — once every source completes cleanly, ready is
// the truthful answer.
func TestStatusAggregator_TimeoutDegradedClearsOnCleanSeed(t *testing.T) {
	agg := newStatusAggregator(2)
	agg.update(report("a", SourcePhaseWatching))
	if got := agg.markDegraded("acme").Phase; got != PhaseDegraded {
		t.Fatalf("phase = %q after markDegraded, want %q", got, PhaseDegraded)
	}

	agg.update(report("b", SourcePhaseWatching))
	if got := agg.buildStatus("acme").Phase; got != PhaseReady {
		t.Errorf("phase = %q after clean late seed, want %q", got, PhaseReady)
	}
}
