package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

func TestEntityLifecycleStale_Registered(t *testing.T) {
	meta := vocabulary.GetPredicateMetadata(EntityLifecycleStale)
	if meta == nil {
		t.Fatalf("predicate %s not registered", EntityLifecycleStale)
	}
	if meta.DataType != "string" {
		t.Errorf("DataType = %q, want %q", meta.DataType, "string")
	}
	if meta.Description == "" {
		t.Error("expected non-empty Description")
	}
}

// TestEntityLifecycleStale_WeightBelowSupersededBy pins the demotion ordering
// D1 requires: a stale fact must rank below a historical-but-alive version, so
// its weight must be strictly more negative than code.lineage.superseded-by's
// -2.0 (registered in source/ast/vocabulary.go).
func TestEntityLifecycleStale_WeightBelowSupersededBy(t *testing.T) {
	const codeLineageSupersededByWeight = -2.0 // source/ast.CodeSupersededBy

	meta := vocabulary.GetPredicateMetadata(EntityLifecycleStale)
	if meta == nil {
		t.Fatalf("predicate %s not registered", EntityLifecycleStale)
	}
	if meta.Weight != -3.0 {
		t.Errorf("Weight = %v, want -3.0", meta.Weight)
	}
	if meta.Weight >= codeLineageSupersededByWeight {
		t.Errorf("Weight = %v must be strictly below code.lineage.superseded-by's %v",
			meta.Weight, codeLineageSupersededByWeight)
	}
}

func TestLifecycleReasonValues(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  string
		want string
	}{
		{"FileDeleted", LifecycleReasonFileDeleted, "file_deleted"},
		{"SourceRemoved", LifecycleReasonSourceRemoved, "source_removed"},
		{"PathMissing", LifecycleReasonPathMissing, "path_missing"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
