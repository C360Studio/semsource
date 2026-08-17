package sourcemanifest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/seedsup"
	"github.com/c360studio/semsource/internal/sourcestatus"
)

// TestHandleStatusReport_UnknownFieldRejectedLoudly pins the strict half of
// the shared-contract rule (#188): semsource is one process, so an unknown
// field in a report can only mean code bypassed internal/sourcestatus. Such
// a report is dropped — never leniently decoded with fields silently lost,
// which is how backpressure vanished for two releases.
func TestHandleStatusReport_UnknownFieldRejectedLoudly(t *testing.T) {
	c := slowSeedComponent(t)
	ctx := context.Background()

	// A conforming report first, so the served status is non-empty and the
	// rogue one's absence is a decision, not an artifact of an empty payload.
	c.handleStatusReport(ctx, seedReport("ast-source-a", SourcePhaseWatching))

	bypassing := []byte(`{
		"instance_name": "rogue-source",
		"source_type": "ast",
		"phase": "watching",
		"entity_count": 5,
		"error_count": 0,
		"not_on_the_contract": true,
		"timestamp": "2026-08-17T12:00:00Z"
	}`)
	c.handleStatusReport(ctx, bypassing)

	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()
	var payload StatusPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if len(payload.Sources) != 1 || payload.Sources[0].InstanceName != "ast-source-a" {
		t.Fatalf("want only the conforming source in status, got: %+v", payload.Sources)
	}
}

// TestStatusPassthrough_FullReportReachesTheSurface pins the passthrough
// half: every populated field of the shared report — including the
// previously dropped backpressure and boundaries_skipped — lands on the
// served per-source status.
func TestStatusPassthrough_FullReportReachesTheSurface(t *testing.T) {
	c := slowSeedComponent(t)
	ctx := context.Background()

	report := sourcestatus.Report{
		InstanceName:      "ast-source-a",
		SourceType:        "ast",
		Phase:             SourcePhaseWatching,
		EntityCount:       7,
		PublishTotal:      9,
		FilesParsed:       11,
		BodiesOffloaded:   4,
		BoundariesSkipped: 2,
		ErrorCount:        1,
		Backpressure:      true,
		Submodules: []sourcestatus.SubmoduleStatus{
			{Path: "vendor/lib", SHA: "b1256521ee39", State: "materialized"},
		},
		LastError: &seedsup.Error{
			Code:      "WATCH_FAILED",
			Message:   "fsnotify closed",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}
	data, err := json.Marshal(&report)
	if err != nil {
		t.Fatal(err)
	}
	c.handleStatusReport(ctx, data)

	c.statusMu.RLock()
	served := c.statusData
	c.statusMu.RUnlock()
	var payload StatusPayload
	if err := json.Unmarshal(served, &payload); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if len(payload.Sources) != 1 {
		t.Fatalf("sources = %d, want 1: %+v", len(payload.Sources), payload.Sources)
	}
	s := payload.Sources[0]
	if !s.Backpressure {
		t.Error("backpressure did not reach the served status — the #188 drop is back")
	}
	if s.BoundariesSkipped != 2 {
		t.Errorf("boundaries_skipped = %d, want 2", s.BoundariesSkipped)
	}
	if s.FilesParsed != 11 || s.BodiesOffloaded != 4 {
		t.Errorf("seed-liveness counters lost: %+v", s)
	}
	if len(s.Submodules) != 1 || s.Submodules[0].State != "materialized" {
		t.Errorf("submodules lost: %+v", s.Submodules)
	}
	if s.LastError == nil || s.LastError.Code != "WATCH_FAILED" {
		t.Errorf("last_error lost or mistyped: %+v", s.LastError)
	}

	// Recovery clears the flag: the next report without backpressure fully
	// replaces the entry — no sticky distress state on the surface.
	report.Backpressure = false
	report.LastError = nil
	data, err = json.Marshal(&report)
	if err != nil {
		t.Fatal(err)
	}
	c.handleStatusReport(ctx, data)
	c.statusMu.RLock()
	served = c.statusData
	c.statusMu.RUnlock()
	// Fresh struct: decoding into the previous payload would keep stale
	// values for omitted omitempty fields and mask a sticky flag.
	var recovered StatusPayload
	if err := json.Unmarshal(served, &recovered); err != nil {
		t.Fatalf("unmarshal recovered status: %v", err)
	}
	if recovered.Sources[0].Backpressure {
		t.Error("backpressure stuck on after a recovered report")
	}
}
