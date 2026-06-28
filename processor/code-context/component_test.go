package codecontext

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
	"github.com/c360studio/semsource/source/ontology"
	srcvocab "github.com/c360studio/semsource/source/vocabulary"
)

// newTestComponent builds a running component over the given graph + lens (no NATS).
func newTestComponent(lensKind string, g fusion.GraphQueryClient) *Component {
	return &Component{
		name:        "code-context",
		lensKind:    lensKind,
		subjectRoot: lensKind + ".v1.",
		engine:      fusion.NewEngine(g),
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
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "svc.go"),
		[]byte("package svc\n\nfunc Dispatch() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := fusiontest.NewMemGraph()
	id := "o.semsource.golang.s.function.dispatch"
	g.AddEntity(id, map[string]any{
		ast.DcTitle: "Dispatch", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 3, ast.CodeEndLine: 3, ontology.ClassPredicate: cco.Algorithm,
	})
	g.SetResolve("Dispatch", id)
	c := newTestComponent("code", g)

	body, _ := json.Marshal(fusion.Request{Query: "Dispatch", Repo: root, Want: []fusion.Want{fusion.WantBody}})
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
	c := newTestComponent("code", g)

	body, _ := json.Marshal(fusion.Request{Query: "Dispatch", Repo: "/x"})
	resp := decodeResp(t, mustServe(t, c, "context", body))
	if resp.Index.Ready || len(resp.Nodes) != 0 || len(resp.Misses) != 0 {
		t.Fatalf("not-ready must be empty + no misses, got %+v", resp)
	}
}

func TestServeCodeRequiresRepo(t *testing.T) {
	c := newTestComponent("code", fusiontest.NewMemGraph())
	if _, err := c.serve(context.Background(), "context", []byte(`{"query":"X"}`)); err == nil {
		t.Fatal("code query without repo must error")
	}
}

func TestServeDocsNoRepoNeeded(t *testing.T) {
	g := fusiontest.NewMemGraph()
	id := "o.semsource.web.s.doc.abc"
	g.AddEntity(id, map[string]any{
		srcvocab.DocType: "document", srcvocab.DcTitle: "Retry Policy",
		srcvocab.DocFilePath: "docs/retry.md", srcvocab.DocContent: "# Retry\nbackoff.",
		ontology.ClassPredicate: cco.Document,
	})
	g.SetResolve("how to retry", id)
	c := newTestComponent("docs", g)

	body, _ := json.Marshal(fusion.Request{Query: "how to retry"}) // no repo
	resp := decodeResp(t, mustServe(t, c, "context", body))
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "Retry Policy" || resp.Nodes[0].Body != "# Retry\nbackoff." {
		t.Fatalf("expected fused doc with body from graph, got %+v", resp.Nodes)
	}
}

func TestHTTPMethodAndReadiness(t *testing.T) {
	c := newTestComponent("code", fusiontest.NewMemGraph())

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

func TestHTTPCodeRequiresRepoIs400(t *testing.T) {
	c := newTestComponent("code", fusiontest.NewMemGraph())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/code-context/context", bytes.NewReader([]byte(`{"query":"X"}`)))
	c.handleHTTP(rec, req, "context")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing repo should be 400, got %d", rec.Code)
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
