package sourcemanifest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	semgraph "github.com/c360studio/semengine/graph"
)

// unreachableProviderStatus is graph-embedding's envelope while its provider
// does not answer: every embedding fails, so the follower publishes caught-up
// coverage as not ready, degraded, with the failures counted.
func unreachableProviderStatus(revision uint64) semgraph.IndexStatusResponse {
	return semgraph.IndexStatusResponse{
		State:           "degraded",
		IndexedRevision: revision,
		TargetRevision:  revision,
		FailedCount:     418,
	}
}

// TestSemanticReadinessWaitsOnGraphEmbedding: at tier 1 the semantic index, on
// both the capabilities document and /status, is ready only while graph-lexical
// and graph-embedding both are. A missing or failing provider must never read
// as a ready semantic index, because search then quietly answers words only.
// spec: openspec/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 acceptance needs a reachable, identified provider
func TestSemanticReadinessWaitsOnGraphEmbedding(t *testing.T) {
	building := semgraph.IndexStatusResponse{State: "building", IndexedRevision: 30, TargetRevision: 42}
	for _, tc := range []struct {
		name          string
		embedding     bool // EmbeddingEnabled: the composition runs graph-embedding
		results       map[string]capabilityStatusResult
		wantReady     bool
		wantAvailable bool
		wantState     string
	}{
		{
			name:      "tier 0 reads the word index alone",
			results:   map[string]capabilityStatusResult{"graph-lexical": {status: readyIndexStatus(42)}},
			wantReady: true, wantAvailable: true, wantState: "ready",
		},
		{
			name:      "tier 1 with both halves ready",
			embedding: true,
			results: map[string]capabilityStatusResult{
				"graph-lexical": {status: readyIndexStatus(42)}, "graph-embedding": {status: readyIndexStatus(42)},
			},
			wantReady: true, wantAvailable: true, wantState: "ready",
		},
		{
			name:      "tier 1 before graph-embedding has published",
			embedding: true,
			results: map[string]capabilityStatusResult{
				"graph-lexical": {status: readyIndexStatus(42)}, "graph-embedding": {err: errors.New("key not found")},
			},
			wantState: readinessUnknown,
		},
		{
			name:      "tier 1 with the provider unreachable",
			embedding: true,
			results: map[string]capabilityStatusResult{
				"graph-lexical": {status: readyIndexStatus(42)}, "graph-embedding": {status: unreachableProviderStatus(42)},
			},
			wantAvailable: true, wantState: "degraded",
		},
		{
			name:      "tier 1 with graph-embedding still embedding",
			embedding: true,
			results: map[string]capabilityStatusResult{
				"graph-lexical": {status: readyIndexStatus(42)}, "graph-embedding": {status: building},
			},
			wantAvailable: true, wantState: "building",
		},
		{
			name:      "tier 1 with the word index behind",
			embedding: true,
			results: map[string]capabilityStatusResult{
				"graph-lexical": {status: building}, "graph-embedding": {status: readyIndexStatus(42)},
			},
			wantAvailable: true, wantState: "building",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := map[string]capabilityStatusResult{"graph-index": {status: readyIndexStatus(42)}}
			for k, v := range tc.results {
				results[k] = v
			}
			c := newCapabilityTestComponent(t, PhaseReady, true, results)
			c.config.EmbeddingEnabled = tc.embedding

			check := func(surface string, got workbenchIndexReadiness) {
				t.Helper()
				if got.Ready != tc.wantReady || got.Available != tc.wantAvailable || got.State != tc.wantState {
					t.Errorf("%s semantic index = ready %v available %v state %q, want ready %v available %v state %q",
						surface, got.Ready, got.Available, got.State, tc.wantReady, tc.wantAvailable, tc.wantState)
				}
				if !got.Ready && got.Reason == nil && got.State == readinessUnknown {
					t.Errorf("%s: an unavailable semantic index carries no reason", surface)
				}
			}

			rec := httptest.NewRecorder()
			c.handleWorkbenchCapabilities(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
			var caps workbenchCapabilitiesResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
				t.Fatalf("decode capabilities: %v: %s", err, rec.Body.String())
			}
			check("capabilities", caps.Readiness.SemanticIndex)
			wantSearch := capabilityNotReady
			if tc.wantReady {
				wantSearch = capabilityReady
			}
			if got := caps.Queries["code_search"].Availability; got != wantSearch {
				t.Errorf("code_search availability = %q, want %q", got, wantSearch)
			}

			rec = httptest.NewRecorder()
			c.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/status", nil))
			var status struct {
				Embedding workbenchIndexReadiness `json:"embedding"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatalf("decode status: %v: %s", err, rec.Body.String())
			}
			check("status", status.Embedding)
		})
	}
}

// TestSemanticReadinessJSONFoldsBothHalves: the MCP gateway's form of the fold
// gives the same verdict from raw envelopes.
// spec: openspec/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 acceptance needs a reachable, identified provider
func TestSemanticReadinessJSONFoldsBothHalves(t *testing.T) {
	ready, err := json.Marshal(readyIndexStatus(42))
	if err != nil {
		t.Fatal(err)
	}
	failing, err := json.Marshal(unreachableProviderStatus(42))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		embedding []byte
		embErr    error
		wantReady bool
	}{
		{name: "both ready", embedding: ready, wantReady: true},
		{name: "provider unreachable", embedding: failing},
		{name: "graph-embedding never published", embErr: errors.New("key not found")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got workbenchIndexReadiness
			if err := json.Unmarshal(SemanticReadinessJSON(ready, nil, tc.embedding, tc.embErr, "semantic index"), &got); err != nil {
				t.Fatal(err)
			}
			if got.Ready != tc.wantReady {
				t.Fatalf("ready = %v, want %v: %+v", got.Ready, tc.wantReady, got)
			}
		})
	}
}

// TestSemanticReadinessFoldsTheLowerWatermark: when both halves are ready but
// cover different revisions, every surface reports the lower indexed revision,
// so a caller's read-your-write comparison never trusts a write graph-embedding
// has not covered. A half without a revision makes the folded one unknown.
// spec: openspec/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 acceptance needs a reachable, identified provider
func TestSemanticReadinessFoldsTheLowerWatermark(t *testing.T) {
	rev := func(v uint64) *uint64 { return &v }
	noRevision := []byte(`{"ready":true,"state":"ready"}`)
	for _, tc := range []struct {
		name                    string
		lexical, embedding      capabilityStatusResult
		wantIndexed, wantTarget *uint64
		wantLag                 *uint64
	}{
		{
			name:    "embedding covers less than the word index",
			lexical: capabilityStatusResult{status: readyIndexStatus(100)}, embedding: capabilityStatusResult{status: readyIndexStatus(90)},
			wantIndexed: rev(90), wantTarget: rev(100), wantLag: rev(10),
		},
		{
			name:    "word index covers less than embedding",
			lexical: capabilityStatusResult{status: readyIndexStatus(80)}, embedding: capabilityStatusResult{status: readyIndexStatus(95)},
			wantIndexed: rev(80), wantTarget: rev(95), wantLag: rev(15),
		},
		{
			name:    "same revision",
			lexical: capabilityStatusResult{status: readyIndexStatus(42)}, embedding: capabilityStatusResult{status: readyIndexStatus(42)},
			wantIndexed: rev(42), wantTarget: rev(42), wantLag: rev(0),
		},
		{
			name:    "embedding revision unknown",
			lexical: capabilityStatusResult{status: readyIndexStatus(100)}, embedding: capabilityStatusResult{raw: noRevision},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check := func(surface string, got workbenchIndexReadiness) {
				t.Helper()
				if !got.Ready || !got.Available {
					t.Fatalf("%s: both halves are ready, got %+v", surface, got)
				}
				for _, f := range []struct {
					field     string
					got, want *uint64
				}{
					{"indexed_revision", got.IndexedRevision, tc.wantIndexed},
					{"target_revision", got.TargetRevision, tc.wantTarget},
					{"lag", got.Lag, tc.wantLag},
				} {
					if (f.got == nil) != (f.want == nil) || (f.got != nil && *f.got != *f.want) {
						t.Errorf("%s %s = %v, want %v", surface, f.field, deref(f.got), deref(f.want))
					}
				}
			}

			c := newCapabilityTestComponent(t, PhaseReady, true, map[string]capabilityStatusResult{
				"graph-index": {status: readyIndexStatus(100)}, "graph-lexical": tc.lexical, "graph-embedding": tc.embedding,
			})
			c.config.EmbeddingEnabled = true
			rec := httptest.NewRecorder()
			c.handleWorkbenchCapabilities(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
			var caps workbenchCapabilitiesResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
				t.Fatalf("decode capabilities: %v: %s", err, rec.Body.String())
			}
			check("capabilities", caps.Readiness.SemanticIndex)

			rec = httptest.NewRecorder()
			c.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/status", nil))
			var status struct {
				Embedding workbenchIndexReadiness `json:"embedding"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatalf("decode status: %v: %s", err, rec.Body.String())
			}
			check("status", status.Embedding)

			// The MCP gateway's source_status folds raw envelopes the same way.
			var mcp workbenchIndexReadiness
			if err := json.Unmarshal(SemanticReadinessJSON(rawStatus(t, tc.lexical), nil, rawStatus(t, tc.embedding), nil,
				"semantic index"), &mcp); err != nil {
				t.Fatal(err)
			}
			check("source_status", mcp)
		})
	}
}

func rawStatus(t *testing.T, r capabilityStatusResult) []byte {
	t.Helper()
	if r.raw != nil {
		return r.raw
	}
	data, err := json.Marshal(r.status)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func deref(v *uint64) any {
	if v == nil {
		return nil
	}
	return *v
}
