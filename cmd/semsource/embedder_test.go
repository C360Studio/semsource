package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semsource/config"
)

// embeddingsRequest is the OpenAI-shaped request httpembedder sends.
type embeddingsRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	InputType string   `json:"input_type"`
}

func embeddingRegistry(url, apiKeyEnv string) *model.Registry {
	return &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"semembed": {Provider: "openai", URL: url, Model: "Snowflake/snowflake-arctic-embed-s", APIKeyEnv: apiKeyEnv},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			model.CapabilityEmbedding: {Preferred: []string{"semembed"}},
		},
	}
}

// TestHTTPEmbedderResolvesTheEmbeddingEndpoint: the embedder graph-embedding
// gets talks to the registry's embedding endpoint with its model and key, and
// sends queries raw with input_type, so semembed applies the instruction.
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 is an explicit opt-in neural composition
func TestHTTPEmbedderResolvesTheEmbeddingEndpoint(t *testing.T) {
	requests := make(chan embeddingsRequest, 1)
	auth := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req embeddingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- req
		auth <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"Snowflake/snowflake-arctic-embed-s","data":[{"index":0,"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()
	t.Setenv("SEMEMBED_TEST_KEY", "secret")

	embedder, err := httpEmbedderFor(embeddingRegistry(srv.URL+"/v1", "SEMEMBED_TEST_KEY"))()
	if err != nil {
		t.Fatalf("build embedder: %v", err)
	}
	defer embedder.Close()
	vectors, err := embedder.GenerateQuery(context.Background(), []string{"where is retry with backoff"})
	if err != nil {
		t.Fatalf("GenerateQuery: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("vectors = %v, want one of width 3", vectors)
	}
	req := <-requests
	if req.Model != "Snowflake/snowflake-arctic-embed-s" || req.InputType != "query" {
		t.Errorf("request model=%q input_type=%q, want the endpoint's model and input_type query", req.Model, req.InputType)
	}
	if len(req.Input) != 1 || req.Input[0] != "where is retry with backoff" {
		t.Errorf("input = %q, want the query unprefixed: semembed applies the instruction", req.Input)
	}
	if got := <-auth; got != "Bearer secret" {
		t.Errorf("Authorization = %q, want the key from api_key_env", got)
	}
}

// TestHTTPEmbedderRefusesAMissingEndpoint: graph-embedding's Start fails
// naming the missing embedding endpoint instead of embedding nothing.
func TestHTTPEmbedderRefusesAMissingEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		models *model.Registry
		want   string
	}{
		{name: "no registry", models: nil, want: "no model_registry"},
		{name: "embedding unbound", models: &model.Registry{Endpoints: map[string]*model.EndpointConfig{}}, want: "embedding"},
		{name: "base URL unusable", models: embeddingRegistry("semembed:8081", ""), want: "http embedder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := httpEmbedderFor(tc.models)()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

// TestTierOneGraphEmbeddingConfigIsAccepted: graph-embedding decodes its config
// strictly, so the composed config must construct through the registered
// factory, not only declare.
func TestTierOneGraphEmbeddingConfigIsAccepted(t *testing.T) {
	cfg, err := config.LoadConfig("../../configs/tiers/tier1-semantic.json")
	if err != nil {
		t.Fatal(err)
	}
	ssCfg, err := buildSemstreamsConfig(cfg, cfg.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	registry := component.NewRegistry()
	if err := registerComponentFactories(registry, componentWiring{ModelRegistry: cfg.ModelRegistry}); err != nil {
		t.Fatal(err)
	}
	cc := ssCfg.Components["graph-embedding"]
	deps := component.Dependencies{NATSClient: &natsclient.Client{}}
	if _, err := registry.CreateComponent("graph-embedding", cc, deps, nil); err != nil {
		t.Fatalf("graph-embedding refused the composed config %s: %v", cc.Config, err)
	}
}
