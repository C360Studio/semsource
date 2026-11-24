package astsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/retry"

	"github.com/c360studio/semsource/entityid"
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

// A glob expands only when the watchers start, so Config.Validate cannot see
// that one watch path matches two directories under one system; the
// watchers refuse it after expansion, and the refusal is not retried.
// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestInitializeWatchersRefusesAGlobSharingASystem(t *testing.T) {
	cases := []struct {
		name    string
		dirs    []string
		refused bool
	}{
		{"glob matching two directories", []string{"svc/a", "svc/b"}, true},
		{"glob matching one directory", []string{"svc/a"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, d := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			c := &Component{
				config: Config{WatchPaths: []WatchPathConfig{{
					Path: filepath.Join(root, "svc", "*"), Org: "acme", Project: "svc", Languages: []string{"go"},
				}}},
				authority: entityid.Authority{Org: "acme", Platform: "test"},
			}
			err := c.initializeWatchers()
			if !tc.refused {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("admitted %d watchers over one system", len(c.watchers))
			}
			if !retry.IsNonRetryable(err) {
				t.Errorf("refusal %v is retryable; retrying cannot change the expansion", err)
			}
			for _, want := range []string{filepath.Join(root, "svc", "a"), filepath.Join(root, "svc", "b"), `system "svc"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to name %s", err, want)
				}
			}
		})
	}
}
