package config_test

import (
	"strings"
	"testing"
)

// semembedRegistry binds the embedding capability to semembed, as the shipped
// tier 1 config does: no query_prefix, so the model name is what classifies it.
const semembedRegistry = `"model_registry":{
	"endpoints":{"semembed":{"provider":"openai","url":"http://semembed:8081/v1","model":"Snowflake/snowflake-arctic-embed-s"}},
	"capabilities":{"embedding":{"preferred":["semembed"]}}
}`

// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 is an explicit opt-in neural composition
func TestGraph_SemanticSearch(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantOn  bool
		wantErr []string // substrings the refusal names; nil means valid
	}{
		{name: "absent is off", body: `"graph":{}`},
		{name: "false is off", body: `"graph":{"semantic_search":false}`},
		{name: "a registry alone does not turn it on", body: `"graph":{},` + semembedRegistry},
		{name: "on with an embedding endpoint", body: `"graph":{"semantic_search":true},` + semembedRegistry, wantOn: true},
		{
			name:    "on without a registry",
			body:    `"graph":{"semantic_search":true}`,
			wantErr: []string{"graph.semantic_search", "model_registry", `"embedding"`},
		},
		{
			name: "on with a registry that leaves embedding unbound",
			body: `"graph":{"semantic_search":true},"model_registry":{
				"endpoints":{"chat":{"provider":"openai","url":"http://chat/v1","model":"qwen3-0.6b"}},
				"capabilities":{"answer_synthesis":{"preferred":["chat"]}}}`,
			wantErr: []string{"graph.semantic_search", `"embedding"`, "resolves to none"},
		},
		{
			name: "on with embedding bound to a chat endpoint",
			body: `"graph":{"semantic_search":true},"model_registry":{
				"endpoints":{"chat":{"provider":"openai","url":"http://chat/v1","model":"qwen3-0.6b"}},
				"capabilities":{"embedding":{"preferred":["chat"]}}}`,
			wantErr: []string{"graph.semantic_search", `"embedding"`, `"chat"`, "not recognisably an embedding endpoint"},
		},
		{
			name:    "on with lexical search off",
			body:    `"graph":{"semantic_search":true,"lexical_search":false},` + semembedRegistry,
			wantErr: []string{"graph.semantic_search", "graph.lexical_search"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadJSON(t, `{"namespace":"acme",`+baseSource+`,`+tc.body+`}`)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("config was accepted; a tier 1 config that cannot run tier 1 must be refused, not boot as tier 0")
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error does not name %s: %v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("config refused: %v", err)
			}
			if got := cfg.Graph.SemanticSearchEnabled(); got != tc.wantOn {
				t.Fatalf("SemanticSearchEnabled() = %v, want %v", got, tc.wantOn)
			}
		})
	}
}
