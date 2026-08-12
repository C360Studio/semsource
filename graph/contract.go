package graph

import (
	"sort"
	"strings"

	semast "github.com/c360studio/semsource/source/ast"
	// Also registers the source predicate vocabulary via its init() so the
	// predicate-schema projection below sees a fully-seeded registry.
	sourcevocab "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/pkg/projection"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

const (
	// OwnerID is the static owner registered by standalone SemSource for its
	// source entity projection.
	OwnerID = "semsource.source-service"

	sourceEntityPattern = "*.semsource.*.*.*.*"

	// GroupSource is the reconcile-mode predicate group covering everything
	// SemSource emits through semsource.entity.v1.
	GroupSource = "source"
	// GroupLifecycle is the reconcile-mode predicate group for the
	// entity.lifecycle.stale marker: reconciling it to one marker triple sets
	// the marker; reconciling it to empty clears it. Reconcile-not-append makes
	// both operations idempotent by construction.
	GroupLifecycle = "lifecycle"
)

// OwnedPredicates returns the exact predicate strings SemSource currently emits
// through semsource.entity.v1. Contract groups take exact predicates, not
// wildcards, so this list is deliberately expanded before validation.
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
		Name:          EntityType.Key(),
		MessageType:   EntityType.Key(),
		EntityPattern: sourceEntityPattern,
		Groups: []projection.PredicateGroup{
			{
				Name:       GroupSource,
				Mode:       projection.ModeReconcile,
				Predicates: OwnedPredicates(),
			},
			{
				Name:       GroupLifecycle,
				Mode:       projection.ModeReconcile,
				Predicates: []string{sourcevocab.EntityLifecycleStale},
			},
		},
	}
}
