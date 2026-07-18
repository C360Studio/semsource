package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
)

func TestWorkbenchCapabilities_ReadyHeadlessContract(t *testing.T) {
	c := newCapabilityTestComponent(t, PhaseReady, true, map[string]capabilityStatusResult{
		"graph.index.query.status":     {status: readyIndexStatus(42)},
		"graph.embedding.query.status": {status: readyIndexStatus(42)},
	})
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("/source-manifest", mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var got workbenchCapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode capabilities: %v: %s", err, rec.Body.String())
	}
	if got.ContractVersion != 1 {
		t.Fatalf("contract_version = %d, want 1", got.ContractVersion)
	}
	if got.Product.Key != "semsource" || got.Product.Name != "SemSource" {
		t.Fatalf("product = %+v", got.Product)
	}
	if got.Project.Key != "acme" || got.Project.IdentityKind != "deployment_namespace" {
		t.Fatalf("project = %+v", got.Project)
	}
	if strings.Contains(rec.Body.String(), "project_id") || strings.Count(got.Project.Key, ".") == 5 {
		t.Fatalf("project scope was represented as an entity id: %s", rec.Body.String())
	}
	if got.Readiness.Overall != capabilityReady || !got.Readiness.Source.Ready ||
		!got.Readiness.StructuralIndex.Ready || !got.Readiness.SemanticIndex.Ready {
		t.Fatalf("readiness = %+v", got.Readiness)
	}
	if got.Readiness.Source.SourceCount == nil || *got.Readiness.Source.SourceCount != 2 ||
		got.Readiness.Source.TotalEntities == nil || *got.Readiness.Source.TotalEntities != 418 {
		t.Fatalf("source readiness = %+v", got.Readiness.Source)
	}

	assertCapability(t, got.Queries, "source_inventory", capabilityReady, http.MethodGet,
		"/source-manifest/sources")
	assertCapability(t, got.Queries, "source_status", capabilityReady, http.MethodGet,
		"/source-manifest/status")
	assertCapability(t, got.Queries, "project_summary", capabilityReady, http.MethodGet,
		"/source-manifest/summary")
	assertCapability(t, got.Queries, "predicate_schema", capabilityReady, http.MethodGet,
		"/source-manifest/predicates")
	assertCapability(t, got.Queries, "code_context", capabilityReady, http.MethodPost,
		"/code-context/context")
	assertCapability(t, got.Queries, "code_impact", capabilityReady, http.MethodPost,
		"/code-context/impact")
	assertCapability(t, got.Queries, "code_search", capabilityReady, http.MethodPost,
		"/code-context/search")
	assertCapability(t, got.Queries, "doc_context", capabilityReady, http.MethodPost,
		"/doc-context/context")
	assertCapability(t, got.Actions, "source_add", capabilityReady, http.MethodPost,
		"/source-manifest/sources")
	assertCapability(t, got.Actions, "source_remove", capabilityReady, http.MethodDelete,
		"/source-manifest/sources/{id}")
	assertUnsupported(t, got.Queries["graph_projection"], "upstream_contract_pending")
	assertUnsupported(t, got.Actions["okf_import"], "not_implemented")
	assertUnsupported(t, got.Actions["okf_export"], "not_implemented")
	assertUnsupported(t, got.ProjectViews, "not_implemented")
	if got.Contracts["fusion_http_error"] != "1" {
		t.Fatalf("contracts = %+v", got.Contracts)
	}
	for _, absent := range []string{"flow-builder", "trajectory", "semteams", "semops", "graphql"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), absent) {
			t.Fatalf("unrelated or unverified capability %q advertised: %s", absent, rec.Body.String())
		}
	}
}

func TestWorkbenchCapabilities_PartialAndUnavailableSignals(t *testing.T) {
	secret := "private.graph.subject.storage-key"
	tests := []struct {
		name            string
		phase           string
		structural      capabilityStatusResult
		semantic        capabilityStatusResult
		overall         string
		structuralQuery string
		semanticQuery   string
	}{
		{
			name:            "structural building",
			phase:           PhaseReady,
			structural:      capabilityStatusResult{status: semgraph.IndexStatusResponse{State: "building", IndexedRevision: 31, TargetRevision: 42, Lag: 11}},
			semantic:        capabilityStatusResult{status: readyIndexStatus(42)},
			overall:         capabilityPartial,
			structuralQuery: capabilityNotReady,
			semanticQuery:   capabilityReady,
		},
		{
			name:            "semantic unavailable does not downgrade structural",
			phase:           PhaseReady,
			structural:      capabilityStatusResult{status: readyIndexStatus(42)},
			semantic:        capabilityStatusResult{err: errors.New(secret)},
			overall:         capabilityReady,
			structuralQuery: capabilityReady,
			semanticQuery:   capabilityNotReady,
		},
		{
			name:            "source still seeding",
			phase:           PhaseSeeding,
			structural:      capabilityStatusResult{status: readyIndexStatus(42)},
			semantic:        capabilityStatusResult{status: readyIndexStatus(42)},
			overall:         capabilityPartial,
			structuralQuery: capabilityReady,
			semanticQuery:   capabilityReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCapabilityTestComponent(t, tt.phase, true, map[string]capabilityStatusResult{
				"graph.index.query.status":     tt.structural,
				"graph.embedding.query.status": tt.semantic,
			})
			rec := httptest.NewRecorder()
			c.handleWorkbenchCapabilities(rec,
				httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			var got workbenchCapabilitiesResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Readiness.Overall != tt.overall {
				t.Fatalf("overall = %q, want %q", got.Readiness.Overall, tt.overall)
			}
			if got.Queries["code_context"].Availability != tt.structuralQuery ||
				got.Queries["code_impact"].Availability != tt.structuralQuery ||
				got.Queries["doc_context"].Availability != tt.structuralQuery {
				t.Fatalf("structural query states = %+v", got.Queries)
			}
			if got.Queries["code_search"].Availability != tt.semanticQuery {
				t.Fatalf("code_search = %+v", got.Queries["code_search"])
			}
			if tt.semantic.err != nil {
				if got.Readiness.SemanticIndex.State != readinessUnknown ||
					got.Readiness.SemanticIndex.Reason == nil ||
					got.Readiness.SemanticIndex.Reason.Code != "status_unavailable" {
					t.Fatalf("semantic readiness = %+v", got.Readiness.SemanticIndex)
				}
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("private readiness error leaked: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestWorkbenchCapabilities_MalformedStatusAndUnavailableActions(t *testing.T) {
	c := newCapabilityTestComponent(t, PhaseReady, false, map[string]capabilityStatusResult{
		"graph.index.query.status":     {status: readyIndexStatus(7)},
		"graph.embedding.query.status": {raw: []byte(`{"ready":`)},
	})
	rec := httptest.NewRecorder()
	c.handleWorkbenchCapabilities(rec,
		httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))

	var got workbenchCapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Readiness.SemanticIndex.State != readinessUnknown ||
		got.Queries["code_search"].Availability != capabilityNotReady {
		t.Fatalf("semantic state = %+v query=%+v", got.Readiness.SemanticIndex, got.Queries["code_search"])
	}
	for _, action := range []string{"source_add", "source_remove"} {
		if got.Actions[action].Availability != capabilityNotReady || got.Actions[action].Reason == nil {
			t.Fatalf("%s = %+v", action, got.Actions[action])
		}
	}
}

func TestWorkbenchCapabilities_SourceErrorsOverrideAggregateReady(t *testing.T) {
	secret := "private source credential failure"
	c := newCapabilityTestComponent(t, PhaseReady, true, map[string]capabilityStatusResult{
		structuralStatusSubject: {status: readyIndexStatus(7)},
		semanticStatusSubject:   {status: readyIndexStatus(7)},
	})
	c.statusData = mustMarshal(t, &StatusPayload{
		Namespace: "acme",
		Phase:     PhaseReady,
		Sources: []SourceStatus{{
			InstanceName: "ast-source",
			SourceType:   "ast",
			Phase:        SourcePhaseErrored,
			ErrorCount:   1,
			LastError: &SourceError{
				Code:      SourceAuthFailed,
				Message:   secret,
				Timestamp: time.Date(2026, 7, 15, 14, 59, 0, 0, time.UTC),
			},
		}},
		Timestamp: time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC),
	})

	rec := httptest.NewRecorder()
	c.handleWorkbenchCapabilities(rec,
		httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
	var got workbenchCapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Readiness.Overall != capabilityPartial || got.Readiness.Source.Ready ||
		got.Readiness.Source.State != PhaseDegraded || got.Readiness.Source.Reason == nil ||
		got.Readiness.Source.Reason.Code != "source_errors_present" ||
		!got.Readiness.Source.Reason.Retryable {
		t.Fatalf("source errors were advertised ready: %+v", got.Readiness)
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("source error detail leaked: %s", rec.Body.String())
	}
	for _, id := range []string{"source_inventory", "source_status", "project_summary", "predicate_schema"} {
		if got.Queries[id].Availability != capabilityNotReady {
			t.Fatalf("%s = %+v, want not_ready", id, got.Queries[id])
		}
	}
}

func TestWorkbenchCapabilities_AbsentIndexMetadataRemainsAbsent(t *testing.T) {
	c := newCapabilityTestComponent(t, PhaseReady, true, map[string]capabilityStatusResult{
		structuralStatusSubject: {raw: []byte(`{"ready":false,"state":"building"}`)},
		semanticStatusSubject:   {raw: []byte(`{"ready":false,"state":"building"}`)},
	})
	rec := httptest.NewRecorder()
	c.handleWorkbenchCapabilities(rec,
		httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))

	var got workbenchCapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Readiness.StructuralIndex.IndexedRevision != nil ||
		got.Readiness.StructuralIndex.TargetRevision != nil || got.Readiness.StructuralIndex.Lag != nil {
		t.Fatalf("absent index metadata was manufactured: %+v", got.Readiness.StructuralIndex)
	}
	for _, field := range []string{"indexed_revision", "target_revision", "lag"} {
		if strings.Contains(rec.Body.String(), `"`+field+`"`) {
			t.Fatalf("absent %s emitted: %s", field, rec.Body.String())
		}
	}
}

func TestWorkbenchCapabilities_StatusQueriesAreConcurrent(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	c := newCapabilityTestComponent(t, PhaseReady, true, nil)
	c.readinessRequest = func(ctx context.Context, subject string) ([]byte, error) {
		started <- subject
		select {
		case <-release:
			return json.Marshal(readyIndexStatus(9))
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		c.handleWorkbenchCapabilities(rec,
			httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
		done <- rec
	}()

	seen := map[string]bool{}
	for range 2 {
		select {
		case subject := <-started:
			seen[subject] = true
		case <-time.After(time.Second):
			t.Fatal("readiness requests did not start concurrently")
		}
	}
	close(release)
	rec := <-done
	if rec.Code != http.StatusOK || len(seen) != 2 {
		t.Fatalf("status=%d subjects=%v body=%s", rec.Code, seen, rec.Body.String())
	}
}

func TestWorkbenchCapabilities_StatusTimeoutIsPartialAndSanitized(t *testing.T) {
	secret := "private.timeout.subject"
	c := newCapabilityTestComponent(t, PhaseReady, true, nil)
	c.readinessTimeout = 20 * time.Millisecond
	c.readinessRequest = func(ctx context.Context, subject string) ([]byte, error) {
		if subject == structuralStatusSubject {
			return json.Marshal(readyIndexStatus(12))
		}
		<-ctx.Done()
		return nil, errors.New(secret)
	}

	rec := httptest.NewRecorder()
	c.handleWorkbenchCapabilities(rec,
		httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got workbenchCapabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Readiness.Overall != capabilityReady || got.Readiness.SemanticIndex.State != readinessUnknown ||
		got.Queries["code_search"].Availability != capabilityNotReady {
		t.Fatalf("timeout response = %+v", got)
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("timeout cause leaked: %s", rec.Body.String())
	}
}

func TestWorkbenchCapabilities_MethodAndCancellation(t *testing.T) {
	c := newCapabilityTestComponent(t, PhaseReady, true, nil)
	c.readinessRequest = func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	rec := httptest.NewRecorder()
	c.handleWorkbenchCapabilities(rec,
		httptest.NewRequest(http.MethodPost, "/source-manifest/capabilities", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status=%d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil).WithContext(ctx)
	w := &capabilityTrackingWriter{}
	c.handleWorkbenchCapabilities(w, req)
	if w.writes != 0 {
		t.Fatalf("writes after cancellation = %d, want 0", w.writes)
	}
}

func TestWorkbenchCapabilities_V1ConsumerIgnoresAdditions(t *testing.T) {
	raw := []byte(`{
		"contract_version":1,
		"product":{"key":"semsource","name":"SemSource","future":"ok"},
		"queries":{"source_inventory":{"availability":"ready"},"future_query":{"availability":"ready"}},
		"future_top_level":true
	}`)
	var consumer struct {
		ContractVersion int `json:"contract_version"`
		Product         struct {
			Key string `json:"key"`
		} `json:"product"`
		Queries map[string]struct {
			Availability string `json:"availability"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(raw, &consumer); err != nil {
		t.Fatalf("v1 additive decode failed: %v", err)
	}
	if consumer.ContractVersion != 1 || consumer.Product.Key != "semsource" ||
		consumer.Queries["source_inventory"].Availability != capabilityReady {
		t.Fatalf("known v1 fields lost: %+v", consumer)
	}
}

type capabilityStatusResult struct {
	status semgraph.IndexStatusResponse
	raw    []byte
	err    error
}

func newCapabilityTestComponent(t *testing.T, phase string, ingestReady bool,
	results map[string]capabilityStatusResult,
) *Component {
	t.Helper()
	status := &StatusPayload{
		Namespace: "acme",
		Phase:     phase,
		Sources: []SourceStatus{
			{InstanceName: "ast-source", SourceType: "ast", Phase: SourcePhaseWatching, EntityCount: 400},
			{InstanceName: "doc-source", SourceType: "docs", Phase: SourcePhaseWatching, EntityCount: 18},
		},
		TotalEntities: 418,
		Timestamp:     time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC),
	}
	c := &Component{
		config:     Config{Namespace: "acme"},
		running:    true,
		statusData: mustMarshal(t, status),
		logger:     newTestLogger(),
	}
	if ingestReady {
		c.ingestCfg = &IngestHandlerConfig{}
	}
	c.readinessRequest = func(_ context.Context, subject string) ([]byte, error) {
		result, ok := results[subject]
		if !ok {
			return nil, errors.New("unexpected readiness subject")
		}
		if result.err != nil {
			return nil, result.err
		}
		if result.raw != nil {
			return result.raw, nil
		}
		return json.Marshal(result.status)
	}
	return c
}

func readyIndexStatus(revision uint64) semgraph.IndexStatusResponse {
	return semgraph.IndexStatusResponse{
		Ready:           true,
		State:           "ready",
		IndexedRevision: revision,
		TargetRevision:  revision,
		Revision:        "ready-revision",
		LastSynced:      "2026-07-15T15:00:00Z",
	}
}

func assertCapability(t *testing.T, capabilities map[string]workbenchCapability, id, availability,
	method, href string,
) {
	t.Helper()
	got, ok := capabilities[id]
	if !ok {
		t.Fatalf("capability %q missing", id)
	}
	if got.Availability != availability || got.Method != method || got.Href != href {
		t.Fatalf("%s = %+v", id, got)
	}
}

func assertUnsupported(t *testing.T, got workbenchCapability, reason string) {
	t.Helper()
	if got.Availability != capabilityUnsupported || got.Reason == nil || got.Reason.Code != reason {
		t.Fatalf("unsupported capability = %+v", got)
	}
}

type capabilityTrackingWriter struct {
	header http.Header
	writes int
}

func (w *capabilityTrackingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *capabilityTrackingWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func (w *capabilityTrackingWriter) WriteHeader(int) { w.writes++ }
