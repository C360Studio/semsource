package sourcespawn

import (
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestAstComponentConfig_TrueFreeze pins the entity-staleness spec's D5
// true-freeze semantics: watch:false with no explicit index_interval must
// disable periodic reindex entirely (not the previous always-on "60s"
// default), while an explicit index_interval is always honored regardless of
// Watch, and watch:true keeps its historical "60s" default.
func TestAstComponentConfig_TrueFreeze(t *testing.T) {
	tests := []struct {
		name              string
		watch             bool
		explicitInterval  string
		wantIndexInterval string
	}{
		{"watch false, no explicit interval => frozen (disabled)", false, "", ""},
		{"watch true, no explicit interval => default 60s", true, "", "60s"},
		{"watch false, explicit interval => honored", false, "30s", "30s"},
		{"watch true, explicit interval => honored", true, "45s", "45s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := config.SourceEntry{
				Type:          "ast",
				Path:          "./service",
				Watch:         tt.watch,
				IndexInterval: tt.explicitInterval,
			}
			_, compCfg, err := astComponentConfig(src, "acme")
			if err != nil {
				t.Fatalf("astComponentConfig() error: %v", err)
			}
			got, _ := compCfg["index_interval"].(string)
			if got != tt.wantIndexInterval {
				t.Errorf("index_interval = %q, want %q", got, tt.wantIndexInterval)
			}
		})
	}
}
