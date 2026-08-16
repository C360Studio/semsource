package sourcemanifest

import (
	"encoding/json"
	"testing"
)

// TestStatusAggregator_PassesSeedLivenessThrough — the manifest must carry the
// pre-publish liveness counters (5.7) onto the external status surface, not
// silently drop them the way an unmapped field would (JSON decode ignores
// nothing loudly).
func TestStatusAggregator_PassesSeedLivenessThrough(t *testing.T) {
	agg := newStatusAggregator(1)
	agg.update(&SourceStatusReport{
		InstanceName:    "ast-source-workspace",
		SourceType:      "ast",
		Phase:           SourcePhaseIngesting,
		FilesParsed:     3,
		BodiesOffloaded: 5,
	})

	status := agg.buildStatus("acme")
	if len(status.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(status.Sources))
	}
	src := status.Sources[0]
	if src.FilesParsed != 3 {
		t.Errorf("FilesParsed = %d, want 3", src.FilesParsed)
	}
	if src.BodiesOffloaded != 5 {
		t.Errorf("BodiesOffloaded = %d, want 5", src.BodiesOffloaded)
	}
}

// TestSourceStatusReport_SeedLivenessWireNames — producers and the manifest
// are coupled by JSON field names, not a shared Go type (the seedsup.Error
// precedent). This pins the wire names the ast-source report emits.
func TestSourceStatusReport_SeedLivenessWireNames(t *testing.T) {
	wire := `{"instance_name":"a","source_type":"ast","phase":"ingesting",` +
		`"entity_count":0,"files_parsed":7,"bodies_offloaded":11,"error_count":0}`
	var r SourceStatusReport
	if err := json.Unmarshal([]byte(wire), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.FilesParsed != 7 || r.BodiesOffloaded != 11 {
		t.Errorf("decoded liveness = (%d, %d), want (7, 11) — wire names files_parsed/bodies_offloaded",
			r.FilesParsed, r.BodiesOffloaded)
	}
}
