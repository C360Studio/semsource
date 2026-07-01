package codecontext

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/graph"

	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
	"github.com/c360studio/semsource/source/ontology"
	srcvocab "github.com/c360studio/semsource/source/vocabulary"
)

// newTestComponent builds a running component over the given graph + body store
// (no NATS). The engine is injected directly, bypassing Start's store attach.
func newTestComponent(lensKind string, g fusion.RetrievalClient, store *fusiontest.MemStore) *Component {
	resolver := fusion.NewBodyResolver(fusion.MapStoreResolver{graph.BodyStoreInstance: store})
	return &Component{
		name:        "code-context",
		lensKind:    lensKind,
		subjectRoot: lensKind + ".v1.",
		graph:       g,
		engine:      fusion.NewEngine(g, resolver),
		logger:      slog.Default(),
		running:     true,
		startTime:   time.Now(),
	}
}

func decodeResp(t *testing.T, raw []byte) fusion.Response {
	t.Helper()
	var resp fusion.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestServeCodeReadyHit(t *testing.T) {
	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	store.Set("body:dispatch", "func Dispatch() {}")
	id := "o.semsource.golang.s.function.dispatch"
	g.AddEntity(id, map[string]any{
		ast.DcTitle: "Dispatch", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 3, ast.CodeEndLine: 3, ontology.ClassPredicate: cco.Algorithm,
		ast.CodeBodyStore: graph.BodyStoreInstance, ast.CodeBodyKey: "body:dispatch",
	})
	g.SetResolve("Dispatch", id)
	c := newTestComponent("code", g, store)

	body, _ := json.Marshal(fusion.Request{Query: "Dispatch", Want: []fusion.Want{fusion.WantBody}})
	raw, err := c.serve(context.Background(), "context", body)
	if err != nil {
		t.Fatal(err)
	}
	resp := decodeResp(t, raw)
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "Dispatch" || resp.Nodes[0].Body != "func Dispatch() {}" {
		t.Fatalf("expected fused Dispatch with body, got %+v", resp.Nodes)
	}
}

func TestServeNotReady(t *testing.T) {
	g := fusiontest.NewMemGraph()
	g.SetStatus(fusion.IndexStatus{Ready: false, State: fusion.StateBuilding})
	c := newTestComponent("code", g, fusiontest.NewMemStore())

	body, _ := json.Marshal(fusion.Request{Query: "Dispatch"})
	resp := decodeResp(t, mustServe(t, c, "context", body))
	if resp.Index.Ready || len(resp.Nodes) != 0 || len(resp.Misses) != 0 {
		t.Fatalf("not-ready must be empty + no misses, got %+v", resp)
	}
}

func TestServeDocs(t *testing.T) {
	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	store.Set("body:retry", "# Retry\nbackoff.")
	id := "o.semsource.web.s.doc.abc"
	g.AddEntity(id, map[string]any{
		srcvocab.DocType: "document", srcvocab.DcTitle: "Retry Policy",
		srcvocab.DocFilePath:  "docs/retry.md",
		srcvocab.DocBodyStore: graph.BodyStoreInstance, srcvocab.DocBodyKey: "body:retry",
		ontology.ClassPredicate: cco.Document,
	})
	g.SetResolve("how to retry", id)
	c := newTestComponent("docs", g, store)

	body, _ := json.Marshal(fusion.Request{Query: "how to retry"})
	resp := decodeResp(t, mustServe(t, c, "context", body))
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "Retry Policy" || resp.Nodes[0].Body != "# Retry\nbackoff." {
		t.Fatalf("expected fused doc with body from store, got %+v", resp.Nodes)
	}
}

// TestServeImpact exercises the "impact" verb: the engine computes the impact
// facet (transitive reverse-relation closure) onto the response itself when the
// request Wants it (beta.123, semstreams#409).
func TestServeImpact(t *testing.T) {
	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	store.Set("body:core", "func Core() {}")
	core := "o.semsource.golang.s.function.core"
	caller := "o.semsource.golang.s.function.caller"
	g.AddEntity(core, map[string]any{
		ast.DcTitle: "Core", ast.CodeType: "function", ast.CodePath: "core.go",
		ast.CodeBodyStore: graph.BodyStoreInstance, ast.CodeBodyKey: "body:core",
	})
	g.AddEntity(caller, map[string]any{
		ast.DcTitle: "Caller", ast.CodeType: "function", ast.CodePath: "caller.go",
	})
	g.AddEdge(caller, ast.CodeCalls, core) // caller → core, so core's reverse closure = {caller}
	g.SetResolve("Core", core)
	c := newTestComponent("code", g, store)

	raw := mustServe(t, c, "impact", mustJSON(fusion.Request{Query: "Core"}))
	resp := decodeResp(t, raw)
	if resp.Impact == nil || resp.Impact.Nodes != 1 || resp.Impact.Files != 1 {
		t.Fatalf("expected impact {nodes:1, files:1}, got %+v", resp.Impact)
	}
}

func TestHTTPMethodAndReadiness(t *testing.T) {
	c := newTestComponent("code", fusiontest.NewMemGraph(), fusiontest.NewMemStore())

	rec := httptest.NewRecorder()
	c.handleHTTP(rec, httptest.NewRequest(http.MethodGet, "/code-context/context", nil), "context")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be 405, got %d", rec.Code)
	}

	c.running = false
	rec = httptest.NewRecorder()
	c.handleHTTP(rec, httptest.NewRequest(http.MethodPost, "/code-context/context", nil), "context")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-running should be 503, got %d", rec.Code)
	}
}

func TestNewComponentRejectsBadLens(t *testing.T) {
	if _, err := NewComponent(json.RawMessage(`{"lens":"nope"}`), component.Dependencies{}); err == nil {
		t.Fatal("NewComponent must reject an unknown lens")
	}
}

func mustServe(t *testing.T, c *Component, verb string, body []byte) []byte {
	t.Helper()
	raw, err := c.serve(context.Background(), verb, body)
	if err != nil {
		t.Fatalf("serve(%s): %v", verb, err)
	}
	return raw
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
