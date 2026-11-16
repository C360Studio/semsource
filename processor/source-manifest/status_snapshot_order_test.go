package sourcemanifest

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func deliveredReport(t *testing.T, delivered int64) []byte {
	t.Helper()
	report, err := json.Marshal(&SourceStatusReport{
		InstanceName:   "ast-a",
		SourceType:     "ast",
		Phase:          "watching",
		OfferedTotal:   delivered,
		DeliveredTotal: delivered,
		Timestamp:      time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func servedDelivered(t *testing.T, c *Component) int64 {
	t.Helper()
	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()
	var served StatusPayload
	if err := json.Unmarshal(data, &served); err != nil {
		t.Fatalf("unmarshal served status: %v", err)
	}
	if len(served.Sources) != 1 {
		t.Fatalf("served status has %d sources, want 1", len(served.Sources))
	}
	return served.Sources[0].DeliveredTotal
}

// TestComponent_HeartbeatSnapshotCannotOverwriteNewerReport covers #268: the
// heartbeat and a source's periodic report tick together, and the heartbeat's
// snapshot, built before the report and stored after it, was served for a full
// interval. The seam sits exactly between build and store. If a report can be
// handled there, it is, which is the losing order; if it cannot, because the
// lock is held across both, it is handled straight after. Either way the
// served status must carry the report's figures.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Source status reflects delivery truth
func TestComponent_HeartbeatSnapshotCannotOverwriteNewerReport(t *testing.T) {
	c := slowSeedComponent(t)
	c.handleStatusReport(context.Background(), deliveredReport(t, 1))

	const newer = 9
	handledInside := false
	c.statusBuilt = func() {
		if !c.statusMu.TryRLock() {
			return
		}
		c.statusMu.RUnlock()
		c.handleStatusReport(context.Background(), deliveredReport(t, newer))
		handledInside = true
	}
	c.refreshStatus()
	if !handledInside {
		c.handleStatusReport(context.Background(), deliveredReport(t, newer))
	}

	if got := servedDelivered(t, c); got != newer {
		t.Fatalf("served delivered_total = %d after a report of %d was handled (handled between build and store: %t)",
			got, newer, handledInside)
	}
}

// TestComponent_ServedStatusNeverOlderThanLastReport is the supplemental stress
// case for the same property: it depends on the scheduler to interleave, so a
// pass proves less than the deterministic test above.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Source status reflects delivery truth
func TestComponent_ServedStatusNeverOlderThanLastReport(t *testing.T) {
	c := slowSeedComponent(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				c.refreshStatus()
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()

	for delivered := int64(1); delivered <= 5000; delivered++ {
		c.handleStatusReport(context.Background(), deliveredReport(t, delivered))
		if got := servedDelivered(t, c); got != delivered {
			t.Fatalf("served delivered_total = %d after a report of %d was handled", got, delivered)
		}
	}
}
