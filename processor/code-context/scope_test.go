package codecontext

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/fusion"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
)

// scopeCapturingGraph records the Scope that reaches Resolve so a test can assert
// what the gateway defaulted (the MemGraph fake otherwise ignores Scope).
type scopeCapturingGraph struct {
	*fusiontest.MemGraph
	lastScope []string
}

func (g *scopeCapturingGraph) Resolve(ctx context.Context, q fusion.ResolveQuery) ([]string, error) {
	g.lastScope = q.Scope
	return g.MemGraph.Resolve(ctx, q)
}

// newScopeComponent builds a running component with the given lens + org over a
// Scope-capturing graph (no NATS). The query resolves to nothing, so serve runs
// the resolve (capturing Scope) and returns an empty response without hydrating.
func newScopeComponent(lensKind, org string) (*Component, *scopeCapturingGraph) {
	g := &scopeCapturingGraph{MemGraph: fusiontest.NewMemGraph()}
	g.SetStatus(fusion.IndexStatus{Ready: true})
	resolver := fusion.NewBodyResolver(fusion.MapStoreResolver{graph.BodyStoreInstance: fusiontest.NewMemStore()})
	c := &Component{
		name:        "code-context",
		lensKind:    lensKind,
		subjectRoot: lensKind + ".v1.",
		org:         org,
		graph:       g,
		engine:      fusion.NewEngine(g, resolver),
		logger:      slog.Default(),
		running:     true,
		startTime:   time.Now(),
	}
	return c, g
}

func TestDefaultScope(t *testing.T) {
	tests := []struct {
		name     string
		lensKind string
		org      string
		want     []string
	}{
		{
			name:     "docs lens scopes to the web domain",
			lensKind: "docs",
			org:      "acme",
			want:     []string{"acme.semsource.web"},
		},
		{
			name:     "code lens scopes to the code-language domains",
			lensKind: "code",
			org:      "acme",
			want: []string{
				"acme.semsource.golang",
				"acme.semsource.python",
				"acme.semsource.typescript",
				"acme.semsource.javascript",
				"acme.semsource.java",
				"acme.semsource.svelte",
			},
		},
		{
			name:     "empty org yields no scope (unfiltered, pre-ask-#16 behavior)",
			lensKind: "docs",
			org:      "",
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Component{lensKind: tt.lensKind, org: tt.org}
			if got := c.defaultScope(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("defaultScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestServeDefaultsScopeWhenEmpty proves serve applies the per-lens default to
// the fused request when the caller sent no scope.
func TestServeDefaultsScopeWhenEmpty(t *testing.T) {
	c, g := newScopeComponent("docs", "acme")
	if _, err := c.serve(context.Background(), "context", []byte(`{"query":"how does retry work"}`)); err != nil {
		t.Fatalf("serve: %v", err)
	}
	want := []string{"acme.semsource.web"}
	if !reflect.DeepEqual(g.lastScope, want) {
		t.Errorf("resolved scope = %v, want %v", g.lastScope, want)
	}
}

// TestServeRespectsCallerScope proves a caller-provided scope is passed through
// verbatim and the per-lens default does NOT override it.
func TestServeRespectsCallerScope(t *testing.T) {
	c, g := newScopeComponent("docs", "acme")
	body := []byte(`{"query":"how does retry work","scope":["other.semsource.golang"]}`)
	if _, err := c.serve(context.Background(), "context", body); err != nil {
		t.Fatalf("serve: %v", err)
	}
	want := []string{"other.semsource.golang"}
	if !reflect.DeepEqual(g.lastScope, want) {
		t.Errorf("resolved scope = %v, want caller scope %v", g.lastScope, want)
	}
}

// TestServeNoScopeWithoutOrg proves that with no org (standalone/test platform
// identity) serve leaves the scope unset — unfiltered NL resolution as before.
func TestServeNoScopeWithoutOrg(t *testing.T) {
	c, g := newScopeComponent("docs", "")
	if _, err := c.serve(context.Background(), "context", []byte(`{"query":"how does retry work"}`)); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if g.lastScope != nil {
		t.Errorf("resolved scope = %v, want nil (no default without org)", g.lastScope)
	}
}

// TestNewComponentCapturesOrg proves the org is sourced from platform identity.
func TestNewComponentCapturesOrg(t *testing.T) {
	deps := component.Dependencies{Platform: component.PlatformMeta{Org: "acme"}}
	comp, err := NewComponent(json.RawMessage(`{"lens":"docs"}`), deps)
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	if got := comp.(*Component).org; got != "acme" {
		t.Errorf("org = %q, want %q", got, "acme")
	}
}
