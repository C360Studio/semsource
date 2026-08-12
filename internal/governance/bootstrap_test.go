package governance

import (
	"testing"

	semsourcegraph "github.com/c360studio/semsource/graph"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

// The beta.160 bootstrap is a pure local declaration: no registry, no
// heartbeat, no bucket. These tests are the whole behavioral contract —
// validity, named disjoint groups, pattern coverage, and idempotence.

func TestBootstrapStandalone_DeclaresValidatedIntent(t *testing.T) {
	boot, err := BootstrapStandalone(nil)
	if err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	if boot == nil {
		t.Fatal("BootstrapStandalone() returned nil bootstrap")
	}
	if err := boot.Contract.Validate(); err != nil {
		t.Fatalf("declared contract does not validate: %v", err)
	}
}

func TestBootstrapStandalone_IsIdempotent(t *testing.T) {
	first, err := BootstrapStandalone(nil)
	if err != nil {
		t.Fatalf("first BootstrapStandalone() error = %v", err)
	}
	second, err := BootstrapStandalone(nil)
	if err != nil {
		t.Fatalf("second BootstrapStandalone() error = %v", err)
	}
	if first.Contract.Name != second.Contract.Name ||
		len(first.Contract.Groups) != len(second.Contract.Groups) {
		t.Fatalf("bootstrap is not idempotent: %#v vs %#v", first.Contract, second.Contract)
	}
}

func TestSourceEntityContract_GroupsAreNamedAndDisjoint(t *testing.T) {
	contract := semsourcegraph.SourceEntityContract()

	groups := map[string][]string{}
	for _, g := range contract.Groups {
		if g.Name == "" {
			t.Fatal("contract contains an unnamed predicate group")
		}
		groups[g.Name] = g.Predicates
	}
	if _, ok := groups[semsourcegraph.GroupSource]; !ok {
		t.Fatalf("contract missing group %q", semsourcegraph.GroupSource)
	}
	if _, ok := groups[semsourcegraph.GroupLifecycle]; !ok {
		t.Fatalf("contract missing group %q", semsourcegraph.GroupLifecycle)
	}

	seen := map[string]string{}
	for name, predicates := range groups {
		for _, p := range predicates {
			if other, dup := seen[p]; dup {
				t.Fatalf("predicate %q appears in groups %q and %q", p, other, name)
			}
			seen[p] = name
		}
	}
}

func TestSourceEntityContract_PatternCoversSemsourceEntities(t *testing.T) {
	contract := semsourcegraph.SourceEntityContract()
	for id, want := range map[string]bool{
		"acme.semsource.golang.workspace.function.cli-add-go-Add": true,
		"acme.semteams.golang.workspace.function.other":           false,
	} {
		got, err := semtypes.MatchEntityIDPattern(contract.EntityPattern, id)
		if err != nil {
			t.Fatalf("MatchEntityIDPattern(%q, %q) error = %v", contract.EntityPattern, id, err)
		}
		if got != want {
			t.Fatalf("MatchEntityIDPattern(%q, %q) = %v, want %v", contract.EntityPattern, id, got, want)
		}
	}
}
