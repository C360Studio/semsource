package sourcemanifest

import (
	"strings"
	"testing"
)

// TestValidateRefusesEmbeddingWithoutSearchIndex: tier 1 merges graph-embedding
// with the word index, so a config that claims embedding with the search index
// off describes a composition that cannot exist and is refused.
func TestValidateRefusesEmbeddingWithoutSearchIndex(t *testing.T) {
	off, on := false, true
	for _, tc := range []struct {
		name     string
		semantic *bool
		embed    bool
		wantErr  bool
	}{
		{name: "tier 0 lexical", semantic: nil},
		{name: "tier 0 structural", semantic: &off},
		{name: "tier 1", semantic: &on, embed: true},
		{name: "tier 1 with the default search index", semantic: nil, embed: true},
		{name: "embedding without the search index", semantic: &off, embed: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Namespace: "acme", SemanticEnabled: tc.semantic, EmbeddingEnabled: tc.embed}
			err := cfg.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() = %v, want error %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "embedding_enabled") {
				t.Errorf("refusal does not name embedding_enabled: %v", err)
			}
		})
	}
}
