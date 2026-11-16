package sourcemanifest

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestComponent_ServedStatusNeverOlderThanLastReport covers #268: the heartbeat
// and a source's periodic report tick together, and the heartbeat's snapshot,
// built before the report and stored after it, was served for a full interval.
// Once a report has been handled, the served status must carry its figures.
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
		c.handleStatusReport(context.Background(), report)

		c.statusMu.RLock()
		data := c.statusData
		c.statusMu.RUnlock()
		var served StatusPayload
		if err := json.Unmarshal(data, &served); err != nil {
			t.Fatalf("unmarshal served status: %v", err)
		}
		if len(served.Sources) != 1 || served.Sources[0].DeliveredTotal != delivered {
			t.Fatalf("served status is older than the last handled report: want delivered_total=%d, got %+v",
				delivered, served.Sources)
		}
	}
}
