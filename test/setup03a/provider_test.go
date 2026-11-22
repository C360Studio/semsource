//go:build qualification

package setup03a

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// faultProxy sits between SemSource and the frozen, real embedding provider.
// In mode 0 every request reaches the provider; mode 1 answers 503; mode 2
// stalls until the caller gives up. It counts the texts each request carried
// by input_type, so the test can tell document embedding (the index) from
// query embedding (search).
type faultProxy struct {
	mode                      atomic.Int32
	documents, queries        atomic.Int64 // texts forwarded to the provider
	failedDocs, failedQueries atomic.Int64 // texts answered 503
	stalled                   atomic.Int64 // requests stalled
	reverse                   *httputil.ReverseProxy
}

func (p *faultProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var req struct {
		Input     []string `json:"input"`
		InputType string   `json:"input_type"`
	}
	_ = json.Unmarshal(body, &req)
	texts := int64(len(req.Input))
	query := req.InputType == "query"
	switch p.mode.Load() {
	case 1:
		if query {
			p.failedQueries.Add(texts)
		} else {
			p.failedDocs.Add(texts)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"qualification provider unavailable"}}`))
	case 2:
		p.stalled.Add(1)
		// This delay is the injected fault, not a readiness synchronizer.
		// Bound it independently even if a client does not propagate cancellation.
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
	default:
		if query {
			p.queries.Add(texts)
		} else {
			p.documents.Add(texts)
		}
		p.reverse.ServeHTTP(w, r)
	}
}

// searchReply is the part of graph.query.searchGraph's reply this test reads.
type searchReply struct {
	Data struct {
		Text *struct {
			Matches []struct {
				ID         string `json:"id"`
				VectorRank int    `json:"vector_rank"`
			} `json:"matches"`
		} `json:"text"`
		Merge *struct {
			Detail   string `json:"detail"`
			Honoured bool   `json:"honoured"`
			Reason   string `json:"reason"`
		} `json:"merge"`
	} `json:"data"`
}

func (h *harness) searchGraph(query string) (searchReply, json.RawMessage, error) {
	var reply searchReply
	raw, err := h.rpc("graph.query.searchGraph", map[string]any{"query": query})
	if err != nil {
		return reply, raw, err
	}
	err = json.Unmarshal(raw, &reply)
	return reply, raw, err
}

// embeddingStatus reads graph-embedding's GRAPH_STATUS readiness record.
func (h *harness) embeddingStatus() (map[string]any, error) {
	kv, err := h.js.KeyValue(h.ctx, "GRAPH_STATUS")
	if err != nil {
		return nil, err
	}
	entry, err := kv.Get(h.ctx, "graph-embedding")
	if err != nil {
		return nil, err
	}
	var status map[string]any
	err = json.Unmarshal(entry.Value(), &status)
	return status, err
}

// TestProviderFailureAndRecovery injects transport faults through an owned
// proxy and asserts what SemEngine alpha.7 does with them. Successful calls
// still go to exactly the frozen, real embedding provider.
//
// A provider outage is not a failed answer: graph-query merges the word list
// with whatever the embedding search returns, so the answer is the word list
// and says so (merge.honoured false on searchGraph, provenance lexical on
// fusion). Readiness is index coverage, not provider liveness (#234 ruling).
// Two upstream limits are asserted as they stand, so a fix upstream fails
// this test and gets the assertion updated: an entity whose embedding failed
// is not retried until a restart (semengine#724), and a restart embeds stored
// entities again, not only the failed ones (semengine#722).
func TestProviderFailureAndRecovery(t *testing.T) {
	if os.Getenv("SETUP03A_PROFILE") != "neural" {
		t.Skip("provider fault qualification is only selected by the neural profile")
	}
	binary, out, endpoint := os.Getenv("SETUP03A_BINARY"), os.Getenv("SETUP03A_OUT"), os.Getenv("SETUP03A_PROVIDER")
	if binary == "" || out == "" || endpoint == "" {
		t.Fatal("neural provider qualification requires SETUP03A_BINARY, SETUP03A_OUT and SETUP03A_PROVIDER")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	h := &harness{t: t, ctx: ctx, out: filepath.Join(out, "provider-faults"), binary: binary, ids: map[string]string{}}
	if err := os.MkdirAll(h.out, 0755); err != nil {
		t.Fatal(err)
	}
	h.result = report{CorpusVersion: "1", Profile: "neural-provider-faults", SemEngineSlices: []int{2}, Started: time.Now().UTC()}
	defer h.finish()
	raw, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	h.result.BinarySHA256 = hex.EncodeToString(sum[:])
	h.fixture()
	h.broker()
	h.configure("neural")
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	target.Path = ""
	proxy := &faultProxy{reverse: httputil.NewSingleHostReverseProxy(target)}
	server := httptest.NewServer(proxy)
	defer func() { server.CloseClientConnections(); server.Close() }()
	h.result.Configuration["model_registry"].(map[string]any)["endpoints"].(map[string]any)["semembed"].(map[string]any)["url"] = server.URL + "/v1"
	raw, err = json.MarshalIndent(h.result.Configuration, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	h.save("semsource.json", raw)
	h.providerMetrics("before")
	h.start()

	const paraphrase = "where should a blue package be sent"
	if !h.semanticReady("initial_semantic_index_ready", 120*time.Second) {
		return
	}
	seeded := proxy.documents.Load()
	h.check("real_provider_initial_merge", 30*time.Second, func() (any, error) {
		reply, raw, err := h.searchGraph(paraphrase)
		if err != nil {
			return raw, err
		}
		if reply.Data.Merge == nil || !reply.Data.Merge.Honoured {
			return raw, errors.New("searchGraph did not merge the embedding list")
		}
		f, err := h.fuse("doc", "context", paraphrase, 10)
		if err != nil || f.Provenance != "embedding" {
			return f, fmt.Errorf("fusion provenance=%q err=%v, want embedding", f.Provenance, err)
		}
		return map[string]any{"search": raw, "fusion": f, "documents_embedded": seeded}, nil
	})

	proxy.mode.Store(1)
	h.check("outage_search_answers_words_and_says_so", 15*time.Second, func() (any, error) {
		reply, raw, err := h.searchGraph("cobalt lantern routing")
		detail := map[string]any{"reply": raw, "injected_query_texts": proxy.failedQueries.Load()}
		if err != nil {
			return detail, fmt.Errorf("an outage must not fail searchGraph: %w", err)
		}
		if proxy.failedQueries.Load() == 0 {
			return detail, errors.New("fault did not reach the provider")
		}
		if reply.Data.Merge == nil || reply.Data.Merge.Honoured || reply.Data.Merge.Reason == "" {
			return detail, errors.New("outage answer did not report merge.honoured false with a reason")
		}
		if reply.Data.Text == nil || len(reply.Data.Text.Matches) == 0 {
			return detail, errors.New("outage answer lost the word list")
		}
		return detail, nil
	})
	h.check("outage_fusion_answers_words_as_lexical", 15*time.Second, func() (any, error) {
		f, err := h.fuse("doc", "context", "cobalt lantern routing", 10)
		if err != nil {
			return f, fmt.Errorf("an outage must not fail fusion: %w", err)
		}
		if f.Provenance != "lexical" {
			return f, fmt.Errorf("provenance=%q, want lexical: no seed came from the embedding list", f.Provenance)
		}
		for _, n := range f.Nodes {
			if strings.Contains(n.Body, "north depot") {
				return f, nil
			}
		}
		return f, errors.New("word answer lost the matching passage")
	})
	// A document written during the outage fails to embed.
	outageDoc := filepath.Join(h.source, "docs/outage.md")
	if err = os.WriteFile(outageDoc, []byte("# Amber beacon schedule\n\nThe amber beacon schedule ships green crates to the south quay.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h.check("outage_embedding_failure_degrades_index", 90*time.Second, func() (any, error) {
		status, err := h.embeddingStatus()
		detail := map[string]any{"graph_embedding": status, "injected_document_texts": proxy.failedDocs.Load()}
		if err != nil {
			return detail, err
		}
		failed, _ := status["failed_count"].(float64)
		if proxy.failedDocs.Load() == 0 || failed == 0 || status["ready"] == true {
			return detail, errors.New("graph-embedding has not recorded the failed embedding")
		}
		semantic, err := h.semanticStatus()
		detail["semantic_index"] = semantic
		if err != nil || semantic["ready"] == true {
			return detail, errors.New("semantic index reads ready while an embedding is failed")
		}
		return detail, nil
	})

	proxy.mode.Store(2)
	// fusion's nl resolve asks mergedSearch, which waits at most 4 s (or
	// query_timeout_ms when shorter) for the embedding search, then answers
	// from words. searchGraph waits up to query_timeout_ms (15 s by default).
	h.check("stalled_provider_is_bounded_by_the_merge_wait", time.Second, func() (any, error) {
		started := time.Now()
		f, err := h.fuseWithin("doc", "context", "cobalt lantern routing stalled", 10, 12*time.Second)
		elapsed := time.Since(started)
		detail := map[string]any{"response": f, "stalled_requests": proxy.stalled.Load(), "elapsed_ms": elapsed.Milliseconds()}
		if err != nil {
			return detail, fmt.Errorf("a stalled provider must not fail fusion: %w", err)
		}
		if proxy.stalled.Load() == 0 {
			return detail, errors.New("the stall did not reach the provider")
		}
		if elapsed > 6500*time.Millisecond || f.Provenance != "lexical" {
			return detail, errors.New("stalled provider was not bounded by the merge wait or not answered from words")
		}
		return detail, nil
	})

	proxy.mode.Store(0)
	h.check("recovery_merges_again", 30*time.Second, func() (any, error) {
		reply, raw, err := h.searchGraph(paraphrase)
		if err != nil {
			return raw, err
		}
		if reply.Data.Merge == nil || !reply.Data.Merge.Honoured {
			return raw, errors.New("search has not merged again after recovery")
		}
		f, err := h.fuse("doc", "context", paraphrase, 10)
		if err != nil || f.Provenance != "embedding" {
			return f, fmt.Errorf("fusion provenance=%q err=%v, want embedding", f.Provenance, err)
		}
		return f, nil
	})
	// semengine#724: the Worker does not retry a failed embedding. The repair
	// lane runs every 30 s, so the failure is observed across two passes.
	beforeWindow := proxy.documents.Load()
	window := time.Now()
	h.check("failed_embedding_not_retried_after_recovery", 75*time.Second, func() (any, error) {
		status, err := h.embeddingStatus()
		detail := map[string]any{"graph_embedding": status, "documents_since_recovery": proxy.documents.Load() - beforeWindow}
		if err != nil {
			return detail, err
		}
		if failed, _ := status["failed_count"].(float64); failed == 0 {
			return detail, errors.New("the failed embedding was retried; semengine#724 is fixed — update this assertion")
		}
		if time.Since(window) < 65*time.Second {
			return detail, errors.New("observation window still open")
		}
		return detail, nil
	})

	failedBeforeRestart := 0.0
	if status, err := h.embeddingStatus(); err == nil {
		failedBeforeRestart, _ = status["failed_count"].(float64)
	}
	h.checkedStop("restart_checked_exit", false)
	beforeRestart := proxy.documents.Load()
	h.start()
	if !h.semanticReady("restart_semantic_index_ready", 180*time.Second) {
		return
	}
	h.check("restart_embeds_failed_entity", 30*time.Second, func() (any, error) {
		status, err := h.embeddingStatus()
		if err != nil {
			return status, err
		}
		if failed, _ := status["failed_count"].(float64); failed != 0 {
			return status, errors.New("the failed embedding survived a restart")
		}
		reply, raw, err := h.searchGraph("amber beacon green crates south quay")
		if err != nil {
			return raw, err
		}
		if reply.Data.Text != nil {
			for _, m := range reply.Data.Text.Matches {
				if strings.Contains(m.ID, "outage") && m.VectorRank > 0 {
					return raw, nil
				}
			}
		}
		return raw, errors.New("the outage document is not in the embedding list after restart")
	})
	reembedded := proxy.documents.Load() - beforeRestart
	h.check("restart_reembeds_stored_entities", time.Second, func() (any, error) {
		detail := map[string]any{"seed_texts": seeded, "texts_after_restart": reembedded,
			"failed_entities_before_restart": failedBeforeRestart}
		// semengine#722: the embedder reports width 0 until its first reply,
		// so graph-embedding embeds the entities it replays before that reply
		// again, not only the failed ones. Measured on the fixture: 8 texts
		// after restart against an 11-text seed and one failed entity.
		if float64(reembedded) <= failedBeforeRestart {
			return detail, errors.New("a restart embedded only the failed entities; semengine#722 may be fixed — update this assertion")
		}
		return detail, nil
	})
	h.providerMetrics("after_recovery")
}
