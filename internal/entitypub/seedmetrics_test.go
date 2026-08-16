package entitypub

import "testing"

// TestSeedMetricsNilSafe — a nil *SeedMetrics is the documented no-registry
// mode (tests, standalone); every method must be a safe no-op so callers stay
// branch-free, same contract as pubMetrics.
func TestSeedMetricsNilSafe(t *testing.T) {
	var m *SeedMetrics
	m.IncFilesParsed()
	m.IncBodiesOffloaded()

	if got := NewSeedMetrics(nil, "ast-source-workspace"); got != nil {
		t.Errorf("NewSeedMetrics(nil registry) = %v, want nil", got)
	}
	if got := NewSeedMetrics(nil, ""); got != nil {
		t.Errorf("NewSeedMetrics(empty instance) = %v, want nil", got)
	}
}
