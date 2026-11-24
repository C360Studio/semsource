package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestValidateRefusesSourcesSharingASystem(t *testing.T) {
	cases := []struct {
		name    string
		sources []config.SourceEntry
		refused []string
	}{
		{
			name: "two ast sources under one project",
			sources: []config.SourceEntry{
				{Type: "ast", Path: "./a", Project: "svc"},
				{Type: "ast", Path: "./b", Project: "svc"},
			},
			refused: []string{"(root ./a)", "(root ./b)", `system "svc"`},
		},
		{
			name: "a doc source with several roots under one project",
			sources: []config.SourceEntry{
				{Type: "docs", Paths: []string{"./a", "./b"}, Project: "handbook"},
			},
			refused: []string{"(root ./a)", "(root ./b)", `system "handbook"`},
		},
		{
			name: "different versions of one project",
			sources: []config.SourceEntry{
				{Type: "ast", Path: "./v1", Project: "lib", Version: "1.0"},
				{Type: "ast", Path: "./v2", Project: "lib", Version: "2.0"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "semsource.json")
			writeSourcesConfig(t, path, tc.sources)
			term, _ := newTestTerm("")
			err := Validate(context.Background(), term, path)
			if tc.refused == nil {
				if err != nil {
					t.Fatalf("valid config refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("colliding config validated")
			}
			for _, want := range tc.refused {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
		})
	}
}
