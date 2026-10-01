package codecontext

import (
	"context"
	"reflect"
	"testing"
)

func TestMultipleSourceScopesReachResolveBeforeRanking(t *testing.T) {
	c, graph := newScopeComponent("docs", "acme")
	c.sourceSystems = []string{"first-source", "second-source"}
	if _, err := c.serve(context.Background(), "context", []byte(`{"query":"shared phrase"}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"acme.test-a1b2c3.first-source.web",
		"acme.test-a1b2c3.first-source.config",
		"acme.test-a1b2c3.second-source.web",
		"acme.test-a1b2c3.second-source.config",
	}
	if !reflect.DeepEqual(graph.lastScope, want) {
		t.Fatalf("resolver scope=%v, want every source/taxonomy prefix %v", graph.lastScope, want)
	}
}

func TestNoBootSourcesFailsBeforeUnscopedResolution(t *testing.T) {
	c, graph := newScopeComponent("docs", "acme")
	c.sourceSystems = nil
	if _, err := c.serve(context.Background(), "context", []byte(`{"query":"shared phrase"}`)); err == nil {
		t.Fatal("empty boot source scope was treated as an unrestricted query")
	}
	if graph.lastScope != nil {
		t.Fatalf("resolver was reached with scope %v", graph.lastScope)
	}
}
