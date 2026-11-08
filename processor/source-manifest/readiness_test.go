package sourcemanifest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestIndexReadinessFrom_DownedResponderIsExplicit pins the explicit-
// unavailability contract: a failed readiness sub-query yields
// {available:false, reason}, never key omission (the audit found the exact
// signal agents gate on vanished silently when its responder was down).
func TestIndexReadinessFrom_DownedResponderIsExplicit(t *testing.T) {
	got := indexReadinessFrom(nil, errors.New("no responders"), "structural index")
	if got.Available {
		t.Error("Available = true for a downed responder")
	}
	if got.Reason == nil || !strings.Contains(got.Reason.Message, "structural index") {
		t.Errorf("Reason = %+v, want message naming the signal", got.Reason)
	}

	// Garbage replies are unavailable too, not misparsed as ready.
	got = indexReadinessFrom([]byte("not json"), nil, "semantic index")
	if got.Available || got.Ready {
		t.Errorf("garbage reply parsed as available/ready: %+v", got)
	}
}

// TestIndexReadinessFrom_HealthyReplyPassesThrough is the control: a real
// status reply maps onto the canonical shape with available:true.
func TestIndexReadinessFrom_HealthyReplyPassesThrough(t *testing.T) {
	raw := []byte(`{"ready":true,"state":"ready","indexed_revision":42,"target_revision":42}`)
	got := indexReadinessFrom(raw, nil, "structural index")
	if !got.Available || !got.Ready || got.State != "ready" {
		t.Errorf("healthy reply mangled: %+v", got)
	}
	data := IndexReadinessJSON(raw, nil, "structural index")
	var round map[string]any
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("IndexReadinessJSON not valid JSON: %v", err)
	}
	if round["available"] != true {
		t.Errorf("IndexReadinessJSON missing available:true: %s", data)
	}
}

// TestIndexReadinessFrom_DropsLegacyStatusFields pins the ADR-109 port prep
// (#227): a SemStreams-era reply still carrying the legacy phase / revision /
// last_synced trio yields only the revision-numeric contract, so no consumer
// can start gating on a field SemEngine's graph-index no longer emits.
func TestIndexReadinessFrom_DropsLegacyStatusFields(t *testing.T) {
	raw := []byte(`{"ready":true,"state":"ready","indexed_revision":42,"target_revision":42,` +
		`"phase":"ready","revision":"42","last_synced":"2026-07-15T15:00:00Z"}`)
	var round map[string]any
	if err := json.Unmarshal(IndexReadinessJSON(raw, nil, "structural index"), &round); err != nil {
		t.Fatalf("IndexReadinessJSON not valid JSON: %v", err)
	}
	for _, legacy := range []string{"phase", "revision", "last_synced"} {
		if _, ok := round[legacy]; ok {
			t.Errorf("readiness re-emits legacy field %q: %v", legacy, round)
		}
	}
	if round["indexed_revision"] != float64(42) || round["target_revision"] != float64(42) {
		t.Errorf("revision-numeric contract lost: %v", round)
	}
}
