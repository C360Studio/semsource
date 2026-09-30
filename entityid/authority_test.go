package entityid_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

func TestDeploymentAuthorityOwnsIdentity(t *testing.T) {
	first := entityid.Authority{Org: "acme", Platform: "catalog-a1b2c3"}
	second := entityid.Authority{Org: "acme", Platform: "catalog-d4e5f6"}
	for _, authority := range []entityid.Authority{first, second} {
		if err := authority.Validate(); err != nil {
			t.Fatal(err)
		}
		id := authority.Build("checkout", "golang", "function", "Fetch")
		want := authority.Org + "." + authority.Platform + ".checkout.golang.function.Fetch"
		if id != want {
			t.Fatalf("identity = %q, want %q", id, want)
		}
		domain, typ := entityid.Parts(id)
		if domain != "golang" || typ != "function" {
			t.Fatalf("taxonomy = %s.%s", domain, typ)
		}
	}
	if first.Build("checkout", "golang", "function", "Fetch") == second.Build("checkout", "golang", "function", "Fetch") {
		t.Fatal("independent deployments collided")
	}
}

func TestAuthorityRejectsInvalidAndExhaustedBudget(t *testing.T) {
	for _, authority := range []entityid.Authority{
		{}, {Org: "acme", Platform: ""}, {Org: "acme.io", Platform: "catalog"},
		{Org: "acme", Platform: "catalog.*"}, {Org: "acme", Platform: strings.Repeat("p", entityid.MaxAuthorityPairLen)},
	} {
		if err := authority.Validate(); err == nil {
			t.Errorf("accepted invalid authority %#v", authority)
		}
	}
	authority := entityid.Authority{Org: strings.Repeat("o", entityid.MaxOrgLen), Platform: strings.Repeat("p", entityid.MaxAuthorityPairLen-entityid.MaxOrgLen)}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	id := authority.Build(strings.Repeat("s", 80), strings.Repeat("d", 10), strings.Repeat("t", 12), strings.Repeat("i", 500))
	if err := semtypes.ValidateEntityID(id); err != nil {
		t.Fatalf("admitted authority produced rejected identity %q: %v", id, err)
	}
}
