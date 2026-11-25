package sourcemanifest

import (
	"sync"
	"testing"
)

// TestComponent_ConcurrentSummariesDoNotRace covers #278: buildStatus writes
// the aggregator's sticky degraded flag, so summaries built side by side must
// not share a read lock. A lossy report makes every build take that write.
// Run under -race; the phase assertion pins that the flag still lands.
func TestComponent_ConcurrentSummariesDoNotRace(t *testing.T) {
	c := slowSeedComponent(t)
	c.aggregator.update(seeded("ast-a", 7, 7))

	const n = 8
	start := make(chan struct{})
	phases := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			phases[i] = c.buildSummary().Phase
		}()
	}
	close(start)
	wg.Wait()

	for i, got := range phases {
		if got != PhaseDegraded {
			t.Errorf("summary %d: phase = %q, want %q", i, got, PhaseDegraded)
		}
	}
}
