package sourcestatus

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/seedsup"
)

// fullReport populates every field, so the round-trip proves no field is
// lost on the wire — the exact failure mode of the nine-copies era (#188).
func fullReport() Report {
	return Report{
		InstanceName:      "ast-source-1",
		SourceType:        "ast",
		Phase:             "watching",
		EntityCount:       42,
		PublishTotal:      99,
		OfferedTotal:      99,
		DeliveredTotal:    90,
		LostTotal:         9,
		SeedLost:          4,
		FilesParsed:       120,
		BodiesOffloaded:   80,
		BoundariesSkipped: 2,
		ErrorCount:        1,
		TypeCounts:        map[string]int64{"function": 30, "file": 12},
		Backpressure:      true,
		Submodules: []SubmoduleStatus{
			{Path: "vendor/lib", SHA: "b1256521ee39", State: "materialized"},
			{Path: "stale/entry", State: "declared_but_absent"},
		},
		LastError: &seedsup.Error{
			Code:      "SOURCE_UNREACHABLE",
			Message:   "clone failed",
			Timestamp: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		},
		Timestamp: time.Date(2026, 8, 17, 12, 0, 1, 0, time.UTC),
	}
}

func TestReport_RoundTripEveryField(t *testing.T) {
	original := fullReport()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round-trip mismatch:\noriginal: %+v\ndecoded:  %+v", original, decoded)
	}
}

// TestReport_EveryStructFieldSurvivesTheWire guards the contract mechanism
// itself: every exported field of Report must appear in the marshaled JSON
// when populated. A field added without a json tag (or tagged "-") would
// silently vanish — the same class of loss the shared type exists to kill.
func TestReport_EveryStructFieldSurvivesTheWire(t *testing.T) {
	data, err := json.Marshal(fullReport())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(data, &asMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	rt := reflect.TypeOf(Report{})
	if got, want := len(asMap), rt.NumField(); got != want {
		t.Fatalf("marshaled %d keys for %d struct fields — a field is not reaching the wire:\n%s",
			got, want, data)
	}
}
