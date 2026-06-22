package governance

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

func TestOwnedPredicates_ExactAndSorted(t *testing.T) {
	predicates := OwnedPredicates()
	if len(predicates) == 0 {
		t.Fatal("OwnedPredicates returned no predicates")
	}

	seen := map[string]bool{}
	var previous string
	for _, predicate := range predicates {
		if strings.TrimSpace(predicate) == "" {
			t.Fatal("OwnedPredicates contains an empty predicate")
		}
		if strings.ContainsAny(predicate, "*>") {
			t.Fatalf("OwnedPredicates contains wildcard predicate %q", predicate)
		}
		if seen[predicate] {
			t.Fatalf("OwnedPredicates contains duplicate predicate %q", predicate)
		}
		if previous != "" && predicate < previous {
			t.Fatalf("OwnedPredicates is not sorted: %q before %q", previous, predicate)
		}
		seen[predicate] = true
		previous = predicate
	}
}

func TestOwnedPredicates_CoversRepresentativeSourceFamilies(t *testing.T) {
	predicates := predicateSet(OwnedPredicates())
	for _, predicate := range []string{
		source.DocContent,
		source.WebURL,
		source.GitCommitSubject,
		source.ConfigRequires,
		source.MediaStorageRef,
		source.MediaByteRange,
		source.MediaExtractionFinding,
		ast.CodeDocComment,
		ast.CodeSignature,
		ast.CodeCalls,
		ast.CodeCapabilityName,
		ast.DcCreated,
	} {
		if !predicates[predicate] {
			t.Errorf("OwnedPredicates missing %q", predicate)
		}
	}
}

func TestSourceEntityContract_Valid(t *testing.T) {
	contract := SourceEntityContract()
	if contract.Name != "semsource.entity.v1" {
		t.Errorf("Name = %q, want semsource.entity.v1", contract.Name)
	}
	if contract.MessageType != "semsource.entity.v1" {
		t.Errorf("MessageType = %q, want semsource.entity.v1", contract.MessageType)
	}
	if contract.EntityPattern != sourceEntityPattern {
		t.Errorf("EntityPattern = %q, want %q", contract.EntityPattern, sourceEntityPattern)
	}
	if err := contract.Validate(); err != nil {
		t.Fatalf("contract Validate() error: %v", err)
	}
}

func predicateSet(predicates []string) map[string]bool {
	set := make(map[string]bool, len(predicates))
	for _, predicate := range predicates {
		set[predicate] = true
	}
	return set
}
