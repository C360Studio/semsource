package graph_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semsource/graph"
)

func TestLifecycleRunRequest_JSONRoundTrip(t *testing.T) {
	original := graph.LifecycleRunRequest{
		Org:      "acme",
		Systems:  []string{"github-com-acme-repo"},
		RootPath: "/workspace/repo",
		Reason:   graph.LifecycleReasonFileDeleted,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var decoded graph.LifecycleRunRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("round trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestLifecycleRunRequest_RootPathOmittedWhenEmpty(t *testing.T) {
	req := graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"sys"},
		Reason:  graph.LifecycleReasonSourceRemoved,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if _, present := raw["root_path"]; present {
		t.Error("root_path should be omitted when empty (remove_source shape)")
	}
}

func TestPublishLifecycleTrigger_NilClient(t *testing.T) {
	_, err := graph.PublishLifecycleTrigger(context.Background(), nil, graph.LifecycleRunRequest{})
	if err == nil {
		t.Fatal("expected error for nil NATS client, got nil")
	}
}

func TestLifecycleReasonConstants(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  string
		want string
	}{
		{"FileDeleted", graph.LifecycleReasonFileDeleted, "file_deleted"},
		{"SourceRemoved", graph.LifecycleReasonSourceRemoved, "source_removed"},
		{"PathMissing", graph.LifecycleReasonPathMissing, "path_missing"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
