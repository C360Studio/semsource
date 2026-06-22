package governance

import (
	"sort"
	"strings"

	"github.com/c360studio/semsource/graph"
	semast "github.com/c360studio/semsource/source/ast"
	_ "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/pkg/projection"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

const (
	// OwnerID is the static owner registered by standalone SemSource for its
	// source entity projection.
	OwnerID = "semsource.source-service"

	sourceEntityPattern = "*.semsource.*.*.*.*"
)

// OwnedPredicates returns the exact predicate strings SemSource currently emits
// through semsource.entity.v1. Ownership claims reject predicate wildcards, so
// this list is deliberately expanded before contract binding.
func OwnedPredicates() []string {
	set := map[string]struct{}{}

	for _, predicate := range semvocab.ListRegisteredPredicates() {
		if strings.HasPrefix(predicate, "source.") || strings.HasPrefix(predicate, "code.") {
			set[predicate] = struct{}{}
		}
	}

	for _, predicate := range []string{
		semast.CodeCapabilityName,
		semast.CodeCapabilityDescription,
		semast.CodeCapabilityTools,
		semast.CodeCapabilityInputs,
		semast.CodeCapabilityOutputs,
		semast.CodeSignature,
		semast.DcTitle,
		semast.DcCreated,
		semast.DcModified,
	} {
		set[predicate] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for predicate := range set {
		out = append(out, predicate)
	}
	sort.Strings(out)
	return out
}

// SourceEntityContract returns the SemStreams projection contract for
// semsource.entity.v1 source entities.
func SourceEntityContract() projection.Contract {
	return projection.Contract{
		Name:          graph.EntityType.Key(),
		MessageType:   graph.EntityType.Key(),
		EntityPattern: sourceEntityPattern,
		Groups: []projection.PredicateGroup{
			{
				Mode:       ownership.ModeReplaceOwned,
				Predicates: OwnedPredicates(),
			},
		},
	}
}
