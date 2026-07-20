package source

import (
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

// TestNavigationalMarkerDemotionTier pins the weight and, more importantly, its
// position relative to the other demotion markers. The ordering is the contract:
// a navigational node is live and correct — it simply is not evidence — so it
// must rank below anything carrying content but ABOVE a phantom whose backing
// artifact is gone.
func TestNavigationalMarkerDemotionTier(t *testing.T) {
	nav := vocabulary.GetPredicateMetadata(EntityRoleNavigational)
	if nav == nil {
		t.Fatal("entity.role.navigational is not registered")
	}
	if nav.Weight >= 0 {
		t.Errorf("weight = %v, want negative — the marker exists to demote", nav.Weight)
	}
	stale := vocabulary.GetPredicateMetadata(EntityLifecycleStale)
	if stale == nil {
		t.Fatal("entity.lifecycle.stale is not registered")
	}
	if !(nav.Weight > stale.Weight) {
		t.Errorf("navigational weight %v must be strictly above stale's %v: a live "+
			"body-less node must not sink below an entity whose artifact no longer exists",
			nav.Weight, stale.Weight)
	}
}
