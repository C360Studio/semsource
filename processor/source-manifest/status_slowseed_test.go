package sourcemanifest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
)

func slowSeedComponent(t *testing.T) *Component {
	t.Helper()
	cfg := Config{
		Namespace: "acme",
		Sources: []ManifestSource{
			{Type: "ast", Path: "./a"},
			{Type: "docs", Path: "./b"},
		},
		ExpectedSourceCount: 2,
		Ports:               DefaultConfig().Ports,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := NewComponent(raw, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := comp.(*Component)
	c.aggregator = newStatusAggregator(c.config.ExpectedSourceCount)
	return c
}

func statusPhase(t *testing.T, c *Component) string {
	t.Helper()
	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()
	var payload StatusPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	return payload.Phase
}

func seedReport(instance, phase string) []byte {
	data, _ := json.Marshal(&SourceStatusReport{
		InstanceName: instance,
		SourceType:   "ast",
		Phase:        phase,
		EntityCount:  1,
		Timestamp:    time.Now(),
	})
	return data
}

// TestComponent_StatusDuringSlowSeed drives the report-ingest path end to end
// through the component: a slow seed (all sources reported, one still
// ingesting) must serve phase "seeding"; ready appears only after the last
// source finishes its initial seed. This pins the audit's broken consumer
// gate at the component level, above the aggregator unit tests.
func TestComponent_StatusDuringSlowSeed(t *testing.T) {
	c := slowSeedComponent(t)
	ctx := context.Background()

	c.handleStatusReport(ctx, seedReport("ast-source-a", SourcePhaseIngesting))
	c.handleStatusReport(ctx, seedReport("doc-source-b", SourcePhaseIngesting))
	if got := statusPhase(t, c); got != PhaseSeeding {
		t.Fatalf("phase = %q with both sources ingesting, want %q", got, PhaseSeeding)
	}
	if c.seedComplete {
		t.Fatal("seedComplete = true while sources are still ingesting (would suppress the seed timeout)")
	}

	c.handleStatusReport(ctx, seedReport("ast-source-a", SourcePhaseWatching))
	if got := statusPhase(t, c); got != PhaseSeeding {
		t.Fatalf("phase = %q with one source still ingesting, want %q", got, PhaseSeeding)
	}

	c.handleStatusReport(ctx, seedReport("doc-source-b", SourcePhaseIdle))
	if got := statusPhase(t, c); got != PhaseReady {
		t.Fatalf("phase = %q after all sources seeded, want %q", got, PhaseReady)
	}
	if !c.seedComplete {
		t.Fatal("seedComplete = false after all sources seeded")
	}
}
