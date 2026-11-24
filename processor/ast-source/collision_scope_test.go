package astsource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/internal/sourcespawn"
)

// The pin-group parse-scope check reads a config that leaves
// max_symbols_per_file out at this component's default, so a pin and a member
// that spell the default out are one scope, and any other cap is not.
//
// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestPinScopeReadsTheDefaultSymbolCap(t *testing.T) {
	pin := func(t *testing.T, instance, root string, maxSymbols *int) sourcespawn.PlannedComponent {
		t.Helper()
		cfg := map[string]any{"watch_paths": []map[string]any{{
			"path": root, "org": "acme", "project": "github-com-acme-lib", "version": "b1256521ee39", "languages": []string{"go"},
		}}}
		if maxSymbols != nil {
			cfg["max_symbols_per_file"] = *maxSymbols
		}
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return sourcespawn.PlannedComponent{Instance: instance, Pin: true,
			Config: types.ComponentConfig{Name: "ast-source", Type: types.ComponentTypeProcessor, Enabled: true, Config: raw}}
	}
	def := DefaultConfig().MaxSymbolsPerFile
	other := def + 1
	for _, tc := range []struct {
		name    string
		cap     *int
		refused bool
	}{
		{"the default spelled out", &def, false},
		{"another cap", &other, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := sourcespawn.CheckSystemCollisions(context.Background(), []sourcespawn.PlannedComponent{
				pin(t, "ast-source-lib-via-p1", "/p1/lib", nil), pin(t, "ast-source-lib-via-p2", "/p2/lib", tc.cap),
			})
			if (err != nil) != tc.refused {
				t.Fatalf("refused = %v, want %v (err %v)", err != nil, tc.refused, err)
			}
		})
	}
}
