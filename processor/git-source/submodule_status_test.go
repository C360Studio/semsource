package gitsource

import (
	"reflect"
	"testing"

	"github.com/c360studio/semsource/internal/sourcestatus"
	"github.com/c360studio/semsource/workspace"
)

// TestClassifySubmodules pins the loudness states (git-submodule-ingestion
// spec): every declared path gets a state; opt-out reads as deliberate
// (excluded_by_config), stale declarations as declared_but_absent, capped
// nesting as beyond_cap — never silently missing.
func TestClassifySubmodules(t *testing.T) {
	sha := "b191a7bf4013da692c381ab21d35462307eefc23"
	inv := &workspace.SubmoduleInventory{
		Submodules: []workspace.SubmoduleInfo{
			{Path: "sub", SHA: sha, Materialized: true},
			{Path: "nested/sub", SHA: sha, Materialized: false},
			{Path: "ghost", SHA: "", Materialized: false},
		},
		BeyondCap: []string{"deep/chain/sub"},
	}

	got := classifySubmodules(inv, false)
	want := []sourcestatus.SubmoduleStatus{
		{Path: "sub", SHA: sha[:12], State: "materialized"},
		{Path: "nested/sub", SHA: sha[:12], State: "unmaterialized"},
		{Path: "ghost", State: "declared_but_absent"},
		{Path: "deep/chain/sub", State: "beyond_cap"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("classify(skip=false) = %+v, want %+v", got, want)
	}

	// Opt-out: unexpected emptiness becomes deliberate exclusion; a tree the
	// user materialized themselves still reads materialized.
	gotSkip := classifySubmodules(inv, true)
	if gotSkip[0].State != "materialized" || gotSkip[1].State != "excluded_by_config" {
		t.Errorf("classify(skip=true) = %+v", gotSkip)
	}

	if classifySubmodules(nil, false) != nil {
		t.Error("nil inventory must classify to nil")
	}
}
