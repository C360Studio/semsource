package graph_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/graph"
)

// TestLifecycleRunRequest_AbsentSurvivesTheWire guards the distinction the
// Absent field's missing omitempty exists to preserve.
//
// An empty non-nil Absent asserts "my enumeration completed and nothing is
// gone". A nil one alongside an empty RootPath is the remove_source shape,
// which marks every entity in scope. With omitempty the first would marshal
// away into the second, and a pass that found nothing wrong would retract a
// whole corpus. The gap between those two outcomes is the entire reason this
// test exists.
func TestLifecycleRunRequest_AbsentSurvivesTheWire(t *testing.T) {
	cases := []struct {
		name    string
		absent  []string
		wantNil bool
	}{
		{"nothing is gone", []string{}, false},
		{"one object is gone", []string{"reports/q4.md"}, false},
		{"no claim about liveness", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(graph.LifecycleRunRequest{
				Org:     "acme",
				Systems: []string{"artifacts"},
				Reason:  graph.LifecycleReasonFileDeleted,
				Absent:  tc.absent,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var decoded graph.LifecycleRunRequest
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if gotNil := decoded.Absent == nil; gotNil != tc.wantNil {
				t.Errorf("absent came back nil=%v, want nil=%v — wire form was %s",
					gotNil, tc.wantNil, encoded)
			}
			if len(decoded.Absent) != len(tc.absent) {
				t.Errorf("absent = %v, want %v", decoded.Absent, tc.absent)
			}
		})
	}
}
