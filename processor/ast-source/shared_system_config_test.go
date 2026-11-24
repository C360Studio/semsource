package astsource

import (
	"strings"
	"testing"
)

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestConfigValidateRefusesRootsSharingASystem(t *testing.T) {
	wp := func(path, org, project, version string) WatchPathConfig {
		return WatchPathConfig{Path: path, Org: org, Project: project, Version: version, Languages: []string{"go"}}
	}
	cases := []struct {
		name    string
		paths   []WatchPathConfig
		refused string
	}{
		{"one project, no version", []WatchPathConfig{wp("./a", "acme", "svc", ""), wp("./b", "acme", "svc", "")}, `system "svc"`},
		{"one project and version", []WatchPathConfig{wp("./a", "acme", "lib", "1.0"), wp("./b", "acme", "lib", "1.0")}, `system "lib-1-0"`},
		{"distinct versions", []WatchPathConfig{wp("./a", "acme", "lib", "1.0"), wp("./b", "acme", "lib", "2.0")}, ""},
		{"distinct projects", []WatchPathConfig{wp("./a", "acme", "a", ""), wp("./b", "acme", "b", "")}, ""},
		{"distinct orgs", []WatchPathConfig{wp("./a", "acme", "svc", ""), wp("./b", "other", "svc", "")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.WatchPaths = tc.paths
			err := cfg.Validate()
			if tc.refused == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.refused) || !strings.Contains(err.Error(), "./a") || !strings.Contains(err.Error(), "./b") {
				t.Fatalf("err = %v, want a refusal naming both roots and %s", err, tc.refused)
			}
		})
	}
}
