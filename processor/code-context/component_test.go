package codecontext

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
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

// TestHTTPGraphProjectionCompatibility proves the SemSource-owned HTTP route
// passes the beta.153 graph facet through unchanged. This is intentionally an
// end-to-end compatibility test over the real fusion engine rather than a
// locally reconstructed projection contract.
func TestHTTPGraphProjectionCompatibility(t *testing.T) {
	const (
		seed       = "acme.semsource.golang.checkout.function.Dispatch"
		target     = "acme.semsource.golang.checkout.function.Handle"
		idLikeFact = "acme.ops.robotics.gcs.drone.001"
	)
	when := time.Date(2026, 7, 19, 12, 30, 0, 0, time.UTC)
	c := newGraphProjectionCompatibilityComponent(seed, target, idLikeFact, when)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/code-context/context",
		strings.NewReader(`{"query":"Dispatch","want":["graph"]}`))
	c.handleHTTP(rec, req, "context")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	resp := decodeResp(t, rec.Body.Bytes())
	if resp.Graph == nil {
		t.Fatalf("graph projection omitted: %s", rec.Body.String())
	}
	// Both bounds are OBSERVATIONS, not a consistency claim. semstreams beta.157
	// deleted the former Coherent bool (ADR-083): the projection is assembled from
	// N independent reads with no snapshot, and two equal samples cannot establish
	// the absence of motion between them. Assert the sampled revisions and nothing
	// more — re-adding a coherence assertion here would re-assert exactly the claim
	// the substrate retracted.
	if got := resp.Graph.ViewRevision; got.Start != 153 || got.End != 153 {
		t.Fatalf("view_revision = %+v, want both bounds sampled at revision 153", got)
	}
	if len(resp.Graph.Nodes) != 1 || resp.Graph.Nodes[0].Handle != seed {
		t.Fatalf("graph nodes = %+v, want seed %q", resp.Graph.Nodes, seed)
	}

	line := findGraphFact(t, resp.Graph.Nodes[0], ast.CodeStartLine)
	if string(line.Value) != "12" || line.Datatype != "xsd:integer" {
		t.Fatalf("typed line fact = %+v, want JSON number 12 with xsd:integer", line)
	}
	if len(line.Evidence) != 1 || line.Evidence[0].Source != "go-parser" ||
		line.Evidence[0].Timestamp != when.Format(time.RFC3339) ||
		line.Evidence[0].Confidence == nil || *line.Evidence[0].Confidence != 0.99 ||
		line.Evidence[0].Context != "index-153" {
		t.Fatalf("line evidence = %+v, want verbatim parser provenance", line.Evidence)
	}
	idFact := findGraphFact(t, resp.Graph.Nodes[0], ast.CodeSignature)
	if string(idFact.Value) != `"`+idLikeFact+`"` {
		t.Fatalf("ID-like literal fact = %s, want %q as a fact", idFact.Value, idLikeFact)
	}

	call := findGraphEdge(t, resp.Graph, seed, ast.CodeCalls, target)
	ref := findGraphEdge(t, resp.Graph, seed, ast.CodeReferences, target)
	reverse := findGraphEdge(t, resp.Graph, target, ast.CodeCalls, seed)
	if call.Direction != fusion.GraphDirectionOutgoing || ref.Direction != fusion.GraphDirectionOutgoing {
		t.Fatalf("outgoing directions = call:%q reference:%q", call.Direction, ref.Direction)
	}
	if reverse.Direction != fusion.GraphDirectionIncoming {
		t.Fatalf("reverse direction = %q, want incoming relative to seed", reverse.Direction)
	}
	if call.ID == ref.ID {
		t.Fatalf("parallel predicates collapsed to edge id %q", call.ID)
	}
	if len(call.Evidence) != 1 || call.Evidence[0].Source != "go-parser" {
		t.Fatalf("call evidence = %+v, want go-parser", call.Evidence)
	}
	if len(ref.Evidence) != 0 {
		t.Fatalf("missing reference evidence was fabricated: %+v", ref.Evidence)
	}
	if len(reverse.Evidence) != 1 || reverse.Evidence[0].Source != "call-resolver" {
		t.Fatalf("reverse evidence = %+v, want call-resolver", reverse.Evidence)
	}
}

func newGraphProjectionCompatibilityComponent(seed, target, idLikeFact string, when time.Time) *Component {
	g := fusiontest.NewMemGraph()
	g.SetStatus(fusion.IndexStatus{
		Ready:           true,
		State:           fusion.StateReady,
		IndexedRevision: 153,
		TargetRevision:  153,
		// Required since beta.157: the readiness gate defers without it (ADR-084
		// D2, fail-closed), and a deferred response carries no graph facet at all.
		// This fixture is a fully-built graph, so the flag states that.
		BootstrapComplete: true,
	})
	g.AddEntity(seed, map[string]any{
		ast.DcTitle:  "Dispatch",
		ast.CodeType: "function",
		ast.CodePath: "checkout/dispatch.go",
	})
	g.AddEntity(target, map[string]any{
		ast.DcTitle:  "Handle",
		ast.CodeType: "function",
		ast.CodePath: "checkout/handle.go",
	})
	g.AddTriple(message.Triple{
		Subject: seed, Predicate: ast.CodeStartLine, Object: 12, Datatype: "xsd:integer",
		Source: "go-parser", Timestamp: when, Confidence: 0.99, Context: "index-153",
	})
	// A canonical-ID-shaped literal under an undeclared predicate remains a
	// property fact; the HTTP consumer never shape-sniffs it into an edge.
	g.AddTriple(message.Triple{Subject: seed, Predicate: ast.CodeSignature, Object: idLikeFact})
	// Parallel edge predicates and the opposite direction are all explicit and
	// retain true subject-to-object orientation. The references assertion has
	// deliberately absent evidence to prove the honesty contract.
	g.AddTriple(message.Triple{
		Subject: seed, Predicate: ast.CodeCalls, Object: target,
		Source: "go-parser", Timestamp: when, Confidence: 0.95, Context: "index-153",
	})
	g.AddEdge(seed, ast.CodeCalls, target)
	g.AddTriple(message.Triple{Subject: seed, Predicate: ast.CodeReferences, Object: target})
	g.AddEdge(seed, ast.CodeReferences, target)
	g.AddTriple(message.Triple{
		Subject: target, Predicate: ast.CodeCalls, Object: seed,
		Source: "call-resolver", Confidence: 0.8,
	})
	g.AddEdge(target, ast.CodeCalls, seed)
	g.SetResolve("Dispatch", seed)

	return newTestComponent("code", g, fusiontest.NewMemStore())
}

func findGraphFact(t *testing.T, node fusion.GraphNode, predicate string) fusion.GraphFact {
	t.Helper()
	for _, fact := range node.Facts {
		if fact.Predicate == predicate {
			return fact
		}
	}
	t.Fatalf("fact %q not found on node %+v", predicate, node)
	return fusion.GraphFact{}
}

func findGraphEdge(t *testing.T, projection *fusion.GraphProjection, source, predicate,
	target string,
) fusion.GraphEdge {
	t.Helper()
	for _, edge := range projection.Edges {
		if edge.Source == source && edge.Predicate == predicate && edge.Target == target {
			return edge
		}
	}
	t.Fatalf("edge %s -[%s]-> %s not found in %+v", source, predicate, target, projection.Edges)
	return fusion.GraphEdge{}
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
