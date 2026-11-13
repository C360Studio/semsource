package config_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The composition version orders the binary and the operator
func TestLoadConfigFromReader_ConfigRevision(t *testing.T) {
	const sources = `"sources": [{"type": "docs", "paths": ["docs/"]}]`
	tests := []struct {
		name     string
		revision string // the config_revision member, or "" to omit it
		want     int
		wantErr  string
	}{
		{name: "absent defaults to zero", want: 0},
		{name: "zero", revision: `"config_revision": 0,`, want: 0},
		{name: "positive", revision: `"config_revision": 7,`, want: 7},
		{name: "negative is refused", revision: `"config_revision": -1,`, wantErr: "config_revision"},
		{name: "fraction is refused", revision: `"config_revision": 1.5,`, wantErr: "config_revision"},
		{name: "string is refused", revision: `"config_revision": "1",`, wantErr: "config_revision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := `{"namespace": "acme", ` + tc.revision + sources + `}`
			cfg, err := config.LoadConfigFromReader(strings.NewReader(doc))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadConfigFromReader(%s) succeeded with revision %d; want an error naming %q", doc, cfg.ConfigRevision, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not name %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfigFromReader(%s): %v", doc, err)
			}
			if cfg.ConfigRevision != tc.want {
				t.Fatalf("ConfigRevision = %d, want %d", cfg.ConfigRevision, tc.want)
			}
		})
	}
}
