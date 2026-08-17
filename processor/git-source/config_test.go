package gitsource

import (
	"encoding/json"
	"testing"
)

// TestConfig_SubmodulesDecode pins the opt-out surface: absent means nil
// (materialization ON — silent absence is the failure mode), explicit false
// survives decode.
func TestConfig_SubmodulesDecode(t *testing.T) {
	var absent Config
	if err := json.Unmarshal([]byte(`{"org":"acme"}`), &absent); err != nil {
		t.Fatal(err)
	}
	if absent.Submodules != nil {
		t.Errorf("absent submodules decoded non-nil: %v", *absent.Submodules)
	}

	var off Config
	if err := json.Unmarshal([]byte(`{"org":"acme","submodules":false}`), &off); err != nil {
		t.Fatal(err)
	}
	if off.Submodules == nil || *off.Submodules {
		t.Errorf("submodules:false decoded as %v, want explicit false", off.Submodules)
	}
}
