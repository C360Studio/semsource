package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semengine/config"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckBootCollisions(t *testing.T) {
	pin := func(parent string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: "/" + parent + "/lib", Project: "github-com-acme-lib",
			Version: "abc123def456", InstanceSuffix: "via-" + parent + "-lib"}
	}
	operator := config.SourceEntry{Type: "ast", Path: "/r/lib", Project: "github-com-acme-lib", Version: "abc123def456"}
	cases := []struct {
		name    string
		entries []config.SourceEntry
		pins    int // the first pins entries are filed as submodule pins
		refused bool
	}{
		{"two pins of one SHA", []config.SourceEntry{pin("p1"), pin("p2")}, 2, false},
		{"a pin and a standalone source at its identity", []config.SourceEntry{pin("p1"), operator}, 1, false},
		{"pins the record does not name are operator sources", []config.SourceEntry{pin("p1"), pin("p2")}, 0, true},
		{"one doc source over two roots of one project", []config.SourceEntry{{Type: "docs", Paths: []string{"/r/a", "/r/b"}, Project: "svc"}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			components := semconfig.ComponentConfigs{}
			var pinned []string
			for i, e := range tc.entries {
				built, err := sourcespawn.Build(e, reconcileOpts)
				if err != nil {
					t.Fatal(err)
				}
				for name, cc := range built {
					components[name] = cc
					if i < tc.pins {
						pinned = append(pinned, name)
					}
				}
			}
			components["source-manifest"] = manifestWith(t, nil, []sourcemanifest.DiscoveredRecord{
				{Parent: "/p", Kind: "submodule", Components: pinned},
			})
			err := checkBootCollisions(components)
			if tc.refused != (err != nil) {
				t.Fatalf("refused = %v, want %v (err %v)", err != nil, tc.refused, err)
			}
			if err != nil && !strings.Contains(err.Error(), "refusing to boot") {
				t.Errorf("boot refusal %q does not say boot stopped", err)
			}
		})
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestLoadAndExpandConfigRefusesSourcesSharingASystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "semsource.json")
	body := `{"namespace":"acme","sources":[` +
		`{"type":"ast","path":"./a","project":"svc"},{"type":"ast","path":"./b","project":"svc"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadAndExpandConfig(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), `system "svc"`) {
		t.Fatalf("run accepted two ast sources under one project: err = %v", err)
	}
}
