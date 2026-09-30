package urlhandler

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

func TestURLWithAdmittedAuthorityFitsIDBudget(t *testing.T) {
	authority := entityid.Authority{Org: strings.Repeat("o", 64), Platform: strings.Repeat("p", entityid.MaxAuthorityPairLen-64)}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	// Both DNS labels and the complete hostname are valid, but exceed the
	// source-system budget. The hash suffix preserves distinct long hosts.
	host := strings.Repeat("a", 60) + "." + strings.Repeat("b", 39) + ".example"
	entity := newPageEntity(authority, "https://"+host+"/path", "text/html", "", "hash", time.Now())
	if err := semtypes.ValidateEntityID(entity.ID); err != nil {
		t.Fatalf("admitted authority and URL produced invalid %d-byte ID: %v", len(entity.ID), err)
	}
	wantSystem := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbb-1c9468"
	if entity.System != wantSystem {
		t.Fatalf("URL system=%q, want %q", entity.System, wantSystem)
	}
	other := newPageEntity(authority, "https://"+host+"-other/path", "text/html", "", "hash", time.Now())
	if entity.System == other.System {
		t.Fatal("different long hosts collapsed to one system")
	}
}
