package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/model"
)

// TestShippedConfigsRouteCapabilitiesToCompatibleEndpoints is the check that makes
// this class of defect impossible rather than correcting the two instances found.
//
// Every shipped config routed query_classification and answer_synthesis to the
// embeddings endpoint, because defaults.model was a catch-all pointed at semembed
// and nothing asserted that a capability's endpoint can actually serve it. The
// misroute was invisible: it starts cleanly and only fails when called, and the
// calling paths are unexercised in the default profile.
func TestShippedConfigsRouteCapabilitiesToCompatibleEndpoints(t *testing.T) {
	// Discovery, not enumeration, and it reuses the same Walk the existing
	// shipped-config gate uses: a hand-written list is how this defect survived —
	// it would keep passing while a new config shipped unchecked.
	var found int
	err := filepath.Walk("../configs", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		found++
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, lerr := LoadConfig(path)
			if lerr != nil {
				t.Fatalf("shipped config does not load: %v", lerr)
			}
			if cfg.ModelRegistry == nil {
				t.Skip("no model registry")
			}
			if verr := validateCapabilityRoles(cfg.ModelRegistry); verr != nil {
				t.Errorf("capability routed to an endpoint that cannot serve it: %v", verr)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk ../configs: %v", err)
	}
	if found == 0 {
		t.Fatal("no shipped configs discovered — a passing test that checks nothing " +
			"is worse than no test")
	}
}

// TestCapabilityRoleCheckCatchesTheOriginalBug proves the check fires on the exact
// defect it exists for. A guard that has never been shown to fail is not a guard;
// this restores defaults.model to the embedder and requires rejection.
func TestCapabilityRoleCheckCatchesTheOriginalBug(t *testing.T) {
	raw := `{
	  "endpoints": {
	    "semembed": {
	      "provider": "openai",
	      "url": "http://semembed:8081/v1",
	      "model": "Snowflake/snowflake-arctic-embed-s",
	      "query_prefix": "Represent this sentence for searching relevant passages: "
	    }
	  },
	  "capabilities": { "embedding": { "preferred": ["semembed"] } },
	  "defaults": { "model": "semembed" }
	}`
	var reg model.Registry
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	err := validateCapabilityRoles(&reg)
	if err == nil {
		t.Fatal("the pre-fix config passed the role check — the check does not catch " +
			"the defect it was written for")
	}
	for _, want := range []string{"query_classification", "semembed", "unbound"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q, so it does not tell an operator what to do: %v", want, err)
		}
	}
}

// TestUnboundLLMCapabilityIsAccepted is the corollary, and it is what stops the
// check being satisfied by binding everything to something generative. An unbound
// LLM capability is a SUPPORTED state — the component degrades to its documented
// non-LLM path — not an omission to paper over with a default.
func TestUnboundLLMCapabilityIsAccepted(t *testing.T) {
	raw := `{
	  "endpoints": {
	    "semembed": {
	      "provider": "openai",
	      "url": "http://semembed:8081/v1",
	      "model": "Snowflake/snowflake-arctic-embed-s",
	      "query_prefix": "Represent this sentence for searching relevant passages: "
	    }
	  },
	  "capabilities": { "embedding": { "preferred": ["semembed"] } }
	}`
	var reg model.Registry
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := validateCapabilityRoles(&reg); err != nil {
		t.Errorf("an unbound LLM capability was rejected, but it is the documented "+
			"degradation state: %v", err)
	}
}

// TestEmbeddingDirectionIsDeliberatelyNotChecked pins a decision that looks like
// an omission.
//
// The inference is only sound in one direction: a positive signal proves an
// endpoint serves embeddings; its absence proves nothing, because an unrecognised
// embedder looks identical to a chat endpoint. Enforcing the reverse rejected
// model "arctic-s" — a real embedding model — in this package's own fixtures.
//
// A chat endpoint bound to `embedding` fails loudly at first use. The defect this
// check exists for is the opposite kind: silent and deferred.
func TestEmbeddingDirectionIsDeliberatelyNotChecked(t *testing.T) {
	raw := `{
	  "endpoints": {
	    "semembed": { "provider": "openai", "url": "http://localhost:8081/v1", "model": "arctic-s" }
	  },
	  "capabilities": { "embedding": { "preferred": ["semembed"] } }
	}`
	var reg model.Registry
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := validateCapabilityRoles(&reg); err != nil {
		t.Errorf("an embedding model this function does not enumerate was rejected; "+
			"the absence of a positive signal must never be treated as evidence: %v", err)
	}
}

// TestIsEmbeddingEndpoint pins the inference itself against the endpoints actually
// shipped, plus the case with neither signal.
func TestIsEmbeddingEndpoint(t *testing.T) {
	tests := []struct {
		name string
		ep   *model.EndpointConfig
		want bool
		why  string
	}{
		{
			name: "arctic with query_prefix",
			ep:   &model.EndpointConfig{Model: "Snowflake/snowflake-arctic-embed-s", QueryPrefix: "Represent this sentence: "},
			want: true,
			why:  "query_prefix is documented as used ONLY by the embedding capability",
		},
		{
			name: "symmetric embedder, no prefix",
			ep:   &model.EndpointConfig{Model: "nomic-embed-text"},
			want: true,
			why:  "model-name backstop: symmetric embedders need no prefix",
		},
		{
			name: "generative model",
			ep:   &model.EndpointConfig{Model: "qwen3-0.6b", MaxTokens: 16384},
			want: false,
		},
		{
			name: "neither signal defaults to generative",
			ep:   &model.EndpointConfig{Model: "some-custom-model"},
			want: false,
			why: "an unknown endpoint is assumed generative; the embedding direction is " +
				"checked separately, so a misclassification still fails loudly rather than silently",
		},
		{name: "nil", ep: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmbeddingEndpoint(tt.ep); got != tt.want {
				t.Errorf("isEmbeddingEndpoint = %v, want %v (%s)", got, tt.want, tt.why)
			}
		})
	}
}
