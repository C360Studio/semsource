package config_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semsource/config"
)

// loadJSON is a small helper: parse+validate a config from a JSON literal.
func loadJSON(t *testing.T, jsonStr string) (*config.Config, error) {
	t.Helper()
	return config.LoadConfigFromReader(strings.NewReader(jsonStr))
}

const baseSource = `"sources":[{"type":"ast","path":"/tmp/x","language":"go"}]`

func TestGraph_LexicalSearchDefaultsOn(t *testing.T) {
	for name, graph := range map[string]string{"no graph block": "", "empty graph block": `,"graph":{}`} {
		cfg, err := loadJSON(t, `{"namespace":"acme",`+baseSource+graph+`}`)
		if err != nil {
			t.Fatalf("%s: tier 0 config should be valid: %v", name, err)
		}
		if !cfg.Graph.LexicalSearchEnabled() {
			t.Fatalf("%s: lexical search should default on", name)
		}
		if cfg.ModelRegistry != nil {
			t.Fatalf("%s: expected nil ModelRegistry, got %+v", name, cfg.ModelRegistry)
		}
	}
}

// spec: openspec/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestGraph_LexicalSearchFalseIsStructuralOnly(t *testing.T) {
	cfg, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,"graph":{"lexical_search":false}}`)
	if err != nil {
		t.Fatalf("structural-only config should be valid without model_registry: %v", err)
	}
	if cfg.Graph.LexicalSearchEnabled() {
		t.Fatal("lexical_search false must disable lexical search")
	}
}

// spec: openspec/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestGraph_EmbedderTypeIsRetiredAtEveryValue(t *testing.T) {
	// SemEngine refuses embedder_type whatever its value, so every former value
	// must fail at load with the migration, not at boot.
	for _, value := range []string{`"bm25"`, `"none"`, `"http"`, `""`, `"bogus"`, `null`} {
		_, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,"graph":{"embedder_type":`+value+`}}`)
		if err == nil {
			t.Fatalf("embedder_type %s should be refused", value)
		}
		for _, want := range []string{"graph.embedder_type", "graph.lexical_search"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("embedder_type %s: error should name %s, got: %v", value, want, err)
			}
		}
	}
}

func TestModelRegistry_Tier2_RetiredClusteringLLM_Rejected(t *testing.T) {
	// clustering_llm is retired in SemEngine (semengine#485 tracks LLM community summaries).
	_, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"enable_clustering":true,"clustering_llm":true},
		"model_registry":{
			"endpoints":{"semembed":{"provider":"openai","url":"http://localhost:8081/v1","model":"arctic-s"}},
			"capabilities":{"embedding":{"preferred":["semembed"]}}
		}
	}`)
	if err == nil {
		t.Fatal("retired clustering_llm should fail validation")
	}
	if !strings.Contains(err.Error(), "graph.clustering_llm") {
		t.Fatalf("error should mention graph.clustering_llm, got: %v", err)
	}
}

func TestModelRegistry_Tier2_Full_OK(t *testing.T) {
	cfg, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
		"graph":{"enable_clustering":true},
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

func TestModelRegistry_InvalidRegistry_Rejected(t *testing.T) {
	// A capability referencing a non-existent endpoint is rejected by
	// semstreams' own Registry.Validate, surfaced through our config load.
	_, err := loadJSON(t, `{
		"namespace":"acme",`+baseSource+`,
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
