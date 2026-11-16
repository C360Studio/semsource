package graph_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
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

// TestLifecycleOwnerSeparatesRootsAndInstances: the owner key is
// deterministic, differs per root and per instance at one root (repo expansion
// gives AST and docs the same path), and never carries the local path.
// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestLifecycleOwnerSeparatesRootsAndInstances(t *testing.T) {
	const root = "/home/someone/src/svc"
	a := graph.LifecycleOwner("ast-source-svc", root)
	if a != graph.LifecycleOwner("ast-source-svc", root) {
		t.Fatal("owner key is not deterministic")
	}
	if a == graph.LifecycleOwner("doc-source-svc", root) {
		t.Error("two instances at one root share an owner key")
	}
	if a == graph.LifecycleOwner("ast-source-svc", root+"/docs") {
		t.Error("two roots of one instance share an owner key")
	}
	if strings.Contains(a, "someone") || strings.Contains(a, "/home") {
		t.Errorf("owner key %q leaks the local path", a)
	}
}
