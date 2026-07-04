package ast_test

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"

	"github.com/c360studio/semsource/source/ast"
)

// TestPredicateSalienceWeights pins the ranking weights the fusion engine folds
// in via fusionvocab.PredicateSalience (max-over-predicates × salienceScale).
// A weight silently dropped (e.g. a role-less re-Register, per the label-alias
// hazard) would quietly regress code_search ranking with no test failure — this
// guards that class. Neutral predicates must stay unweighted so they don't
// flatten the gradient.
func TestPredicateSalienceWeights(t *testing.T) {
	weighted := map[string]float64{
		ast.CodeCapabilityName:        3.0,
		ast.CodeCapabilityDescription: 3.0,
		ast.CodeDocComment:            2.5,
		ast.CodeImplements:            2.0,
		ast.CodeExported:              2.0,
		ast.CodeSignature:             1.5,
		// Signed salience (ADR-0008 #3): superseded_by DEMOTES historical versions
		// below their current (un-superseded) sibling — negative on purpose.
		ast.CodeSupersededBy: -2.0,
	}
	for pred, want := range weighted {
		meta := vocabulary.GetPredicateMetadata(pred)
		if meta == nil {
			t.Errorf("%s: not registered", pred)
			continue
		}
		if meta.Weight != want {
			t.Errorf("%s: Weight = %v, want %v", pred, meta.Weight, want)
		}
	}

	// Universal / housekeeping predicates must remain unweighted (0): weighting
	// a predicate ~every entity carries adds a uniform boost that differentiates
	// nothing and only dilutes the gradient.
	// CodeSupersedes and CodeLineageChange are structural lineage facts, not
	// ranking signals — only superseded_by carries a (negative) weight.
	for _, pred := range []string{
		ast.CodeType, ast.CodePath, ast.CodeHash, ast.CodeLines, ast.CodeCalls,
		ast.CodeSupersedes, ast.CodeLineageChange,
	} {
		if meta := vocabulary.GetPredicateMetadata(pred); meta != nil && meta.Weight != 0 {
			t.Errorf("%s: Weight = %v, want 0 (universal/housekeeping predicate)", pred, meta.Weight)
		}
	}
}
