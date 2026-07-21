package config_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/model"
)

// loadJSON is a small helper: parse+validate a config from a JSON literal.
func loadJSON(t *testing.T, jsonStr string) (*config.Config, error) {
	t.Helper()
	return config.LoadConfigFromReader(strings.NewReader(jsonStr))
}

const baseSource = `"sources":[{"type":"ast","path":"/tmp/x","language":"go"}]`

func TestModelRegistry_Tier0_NoRegistry_OK(t *testing.T) {
	// bm25 (or unset) needs no registry — the common tier-0 case must stay valid.
	cfg, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,"graph":{"embedder_type":"bm25"}}`)
	if err != nil {
		t.Fatalf("tier-0 bm25 config should be valid: %v", err)
	}
	if cfg.ModelRegistry != nil {
		t.Fatalf("expected nil ModelRegistry, got %+v", cfg.ModelRegistry)
	}
}

func TestModelRegistry_Tier1_HTTP_RequiresEmbeddingCapability(t *testing.T) {
	_, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,"graph":{"embedder_type":"http"}}`)
	if err == nil {
		t.Fatal("http embedder without a model_registry should fail validation")
	}
	if !strings.Contains(err.Error(), "embedding") {
		t.Fatalf("error should mention the embedding capability, got: %v", err)
	}
}

func TestModelRegistry_Tier1_HTTP_WithRegistry_OK(t *testing.T) {
	cfg, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"embedder_type":"http"},
		"model_registry":{
			"endpoints":{"semembed":{"provider":"openai","url":"http://localhost:8081/v1","model":"arctic-s"}},
			"capabilities":{"embedding":{"preferred":["semembed"]}}
		}
	}`)
	if err != nil {
		t.Fatalf("http embedder with a valid embedding registry should validate: %v", err)
	}
	// Passthrough must survive load so run.go can hand it to ssCfg.ModelRegistry.
	if cfg.ModelRegistry == nil {
		t.Fatal("ModelRegistry should be populated after load")
	}
	if got := cfg.ModelRegistry.Resolve(model.CapabilityEmbedding); got != "semembed" {
		t.Fatalf("embedding capability should resolve to semembed, got %q", got)
	}
}

func TestModelRegistry_Tier1_HTTP_RegistryMissingEmbedding_Fails(t *testing.T) {
	// A registry that exists but resolves no embedding endpoint must be caught
	// at load, not deferred to a silent runtime degrade.
	_, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"embedder_type":"http"},
		"model_registry":{
			"endpoints":{"other":{"provider":"openai","url":"http://x/v1","model":"m"}},
			"capabilities":{"community_summary":{"preferred":["other"]}},
			"defaults":{}
		}
	}`)
	if err == nil {
		t.Fatal("http embedder with a registry lacking an embedding endpoint should fail")
	}
	if !strings.Contains(err.Error(), "embedding") {
		t.Fatalf("error should mention embedding, got: %v", err)
	}
}

func TestModelRegistry_Tier2_ClusteringLLM_RequiresCommunitySummary(t *testing.T) {
	// clustering_llm with an embedding-only registry must fail on the missing
	// community_summary capability.
	_, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"embedder_type":"http","enable_clustering":true,"clustering_llm":true},
		"model_registry":{
			"endpoints":{"semembed":{"provider":"openai","url":"http://localhost:8081/v1","model":"arctic-s"}},
			"capabilities":{"embedding":{"preferred":["semembed"]}}
		}
	}`)
	if err == nil {
		t.Fatal("clustering_llm without a community_summary capability should fail")
	}
	if !strings.Contains(err.Error(), "community_summary") {
		t.Fatalf("error should mention community_summary, got: %v", err)
	}
}

func TestModelRegistry_Tier2_Full_OK(t *testing.T) {
	cfg, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"embedder_type":"http","enable_clustering":true,"clustering_llm":true},
		"model_registry":{
			"endpoints":{
				"semembed":{"provider":"openai","url":"http://localhost:8081/v1","model":"arctic-s"},
				"seminstruct":{"provider":"openai","url":"http://localhost:8083/v1","model":"qwen3-0.6b","max_tokens":16384}
			},
			"capabilities":{
				"embedding":{"preferred":["semembed"]},
				"community_summary":{"preferred":["seminstruct"]}
			}
		}
	}`)
	if err != nil {
		t.Fatalf("full tier-2 config should validate: %v", err)
	}
	if got := cfg.ModelRegistry.Resolve(model.CapabilityCommunitySummary); got != "seminstruct" {
		t.Fatalf("community_summary should resolve to seminstruct, got %q", got)
	}
}

func TestModelRegistry_ClusteringLLM_IgnoredWhenClusteringOff(t *testing.T) {
	// clustering_llm is inert unless enable_clustering is set, so a bm25 tier-0
	// config with a stray clustering_llm flag must not demand a registry.
	if _, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,"graph":{"clustering_llm":true}}`); err != nil {
		t.Fatalf("clustering_llm without enable_clustering should be inert, got: %v", err)
	}
}

func TestModelRegistry_InvalidRegistry_Rejected(t *testing.T) {
	// A capability referencing a non-existent endpoint is rejected by
	// semstreams' own Registry.Validate, surfaced through our config load.
	_, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"embedder_type":"bm25"},
		"model_registry":{
			"endpoints":{"semembed":{"provider":"openai","url":"http://x/v1","model":"m"}},
			"capabilities":{"embedding":{"preferred":["does-not-exist"]}},
			"defaults":{}
		}
	}`)
	if err == nil {
		t.Fatal("registry with a dangling capability endpoint should be rejected")
	}
	if !strings.Contains(err.Error(), "model_registry") {
		t.Fatalf("error should be attributed to model_registry, got: %v", err)
	}
}
