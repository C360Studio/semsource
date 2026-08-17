package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast/golang"
)

// TestParse_SubmoduleIdentityIsCheckoutLocationIndependent pins the
// shared-submodule dedup contract (git-submodule-ingestion spec + design D2):
// entity IDs are a pure function of (org, scoped project identity, relative
// path, symbol) — never of the checkout's absolute location. Two parents
// linking the same submodule at the same pinned SHA materialize it at
// different filesystem paths, and the graph merges their publishes only if
// every ID is byte-identical.
func TestParse_SubmoduleIdentityIsCheckoutLocationIndependent(t *testing.T) {
	// The scoped system slug is what subwatch-spawned components hand the
	// parser: canonical project + 12-hex gitlink version.
	scoped := entityid.ScopedSystemSlug("github-com-acme-shared-sub", "b191a7bf4013")

	idsFrom := func(root string) []string {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		src := "package greeter\n\ntype Greeter struct{}\n\nfunc Greet() string { return \"hi\" }\n"
		if err := os.WriteFile(filepath.Join(root, "pkg", "greeter.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		p := golang.NewParser("acme", scoped, root)
		results, err := p.ParseDirectory(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, r := range results {
			for _, e := range r.Entities {
				ids = append(ids, e.ID)
			}
			if r.FileEntity != nil {
				ids = append(ids, r.FileEntity.ID)
			}
		}
		sort.Strings(ids)
		return ids
	}

	a := idsFrom(filepath.Join(t.TempDir(), "parent-one", "libs", "sub"))
	b := idsFrom(filepath.Join(t.TempDir(), "parent-two", "sub"))

	if len(a) == 0 {
		t.Fatal("no entities parsed")
	}
	if len(a) != len(b) {
		t.Fatalf("ID counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("ID diverges by checkout location:\n  %s\n  %s", a[i], b[i])
		}
	}

	for _, id := range a {
		if !strings.Contains(id, scoped) {
			t.Errorf("ID %q lacks the version-scoped system segment %q", id, scoped)
		}
		if err := entityid.ValidateNATSKVKey(id); err != nil {
			t.Errorf("ID %q is not NATS-KV-safe: %v", id, err)
		}
	}

	// Two pins of one submodule must NOT collide: a different gitlink version
	// yields a disjoint ID set for identical content.
	other := entityid.ScopedSystemSlug("github-com-acme-shared-sub", "b1256521ee39")
	if other == scoped {
		t.Fatal("version scoping did not change the system slug")
	}
}
