package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
)

func TestParserKeepsRelationshipsInsideEffectiveAuthority(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "call.go")
	if err := os.WriteFile(path, []byte("package fixture\nfunc Caller() { Target() }\nfunc Target() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	firstIDs := map[string]string{}
	for index, platform := range []string{"catalog-a1b2c3", "catalog-d4e5f6"} {
		authority := entityid.Authority{Org: "acme", Platform: platform}
		parser := NewParser(authority, "github.com/acme/fixture", root)
		result, err := parser.ParseFile(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		var callerID, targetID string
		var calls []string
		for _, entity := range result.Entities {
			if !strings.HasPrefix(entity.ID, "acme."+platform+".github-com-acme-fixture.golang.") {
				t.Fatalf("wrong authority or taxonomy: %s", entity.ID)
			}
			if index == 0 {
				firstIDs[entity.Name] = entity.ID
			} else if firstIDs[entity.Name] == entity.ID {
				t.Fatalf("two deployment authorities collided for %s", entity.Name)
			}
			if entity.Name == "Caller" {
				callerID = entity.ID
				calls = entity.Calls
			}
			if entity.Name == "Target" {
				targetID = entity.ID
			}
		}
		if callerID == "" || targetID == "" || len(calls) != 1 || calls[0] != targetID {
			t.Fatalf("relationship does not resolve to same-deployment declaration: caller=%s target=%s calls=%v", callerID, targetID, calls)
		}
	}
}
