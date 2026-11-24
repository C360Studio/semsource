package docsource

import (
	"strings"
	"testing"
)

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestConfigValidateRefusesRootsSharingASystem(t *testing.T) {
	cases := []struct {
		name    string
		paths   []string
		project string
		refused string
	}{
		{"several roots under one project", []string{"./a", "./b"}, "handbook", `system "handbook"`},
		{"one root listed twice", []string{"./a", "./a"}, "", `would share entity-ID system`},
		{"distinct path-derived roots", []string{"./a", "./b"}, "", ""},
		{"one root under a project", []string{"./a"}, "handbook", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Org, cfg.Paths, cfg.Project = "acme", tc.paths, tc.project
			err := cfg.Validate()
			if tc.refused == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.refused) || !strings.Contains(err.Error(), tc.paths[1]) {
				t.Fatalf("err = %v, want a refusal naming the roots and %s", err, tc.refused)
			}
		})
	}
}
