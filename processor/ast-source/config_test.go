package astsource

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
)

func TestConfigValidateRequiresWatchPaths(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "watch_paths") {
		t.Fatalf("Validate() error = %v, want required watch_paths error", err)
	}
}

func TestConfigValidateCanonicalWatchPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WatchPaths = []WatchPathConfig{
		{Path: ".", Org: "acme", Project: "service", Languages: []string{"go"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestNewComponentRejectsRemovedASTKeys(t *testing.T) {
	for _, key := range []string{"repo_path", "org", "project", "version", "languages", "exclude_patterns"} {
		t.Run(key, func(t *testing.T) {
			raw := json.RawMessage(`{"watch_paths":[{"path":".","org":"acme","project":"service"}],"` + key + `":null}`)
			_, err := NewComponent(raw, component.Dependencies{})
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+key+`"`) {
				t.Fatalf("NewComponent() error = %v, want ordinary unknown-field error for %q", err, key)
			}
		})
	}
}

func TestNewComponentRejectsTrailingJSONValue(t *testing.T) {
	raw := json.RawMessage(`{"watch_paths":[{"path":".","org":"acme","project":"service"}]} {}`)
	if _, err := NewComponent(raw, component.Dependencies{}); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("NewComponent() error = %v, want trailing JSON value rejection", err)
	}
}

func TestConfigSchemaOmitsRemovedASTKeys(t *testing.T) {
	for _, key := range []string{"repo_path", "org", "project", "version", "languages", "exclude_patterns"} {
		if _, ok := astSourceSchema.Properties[key]; ok {
			t.Errorf("schema still advertises removed top-level key %q", key)
		}
	}
}

func TestDefaultConfigRetainsGlobalDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.WatchEnabled || cfg.IndexInterval != "60s" || cfg.StreamName != "GRAPH" {
		t.Fatalf("global defaults changed: %+v", cfg)
	}
}
