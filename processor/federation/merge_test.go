package federation_test

import (
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/processor/federation"
)

// --- helpers ---

func entity(id string) graph.GraphEntity {
	return graph.GraphEntity{
		ID: id,
		Provenance: graph.SourceProvenance{
			SourceType: "test",
			SourceID:   "test-source",
			Timestamp:  time.Now(),
			Handler:    "test",
		},
	}
}

func prov(sourceID string) graph.SourceProvenance {
	return graph.SourceProvenance{
		SourceType: "git",
		SourceID:   sourceID,
		Timestamp:  time.Now(),
		Handler:    "test",
	}
}

func seedEvent(ns string, entities ...graph.GraphEntity) *graph.GraphEvent {
	return &graph.GraphEvent{
		Type:       graph.EventTypeSEED,
		SourceID:   "test-source",
		Namespace:  ns,
		Timestamp:  time.Now(),
		Entities:   entities,
		Provenance: prov("test-source"),
	}
}

func deltaEvent(ns string, entities ...graph.GraphEntity) *graph.GraphEvent {
	return &graph.GraphEvent{
		Type:       graph.EventTypeDELTA,
		SourceID:   "test-source",
		Namespace:  ns,
		Timestamp:  time.Now(),
		Entities:   entities,
		Provenance: prov("test-source"),
	}
}

func retractEvent(ns string, ids ...string) *graph.GraphEvent {
	return &graph.GraphEvent{
		Type:        graph.EventTypeRETRACT,
		SourceID:    "test-source",
		Namespace:   ns,
		Timestamp:   time.Now(),
		Retractions: ids,
		Provenance:  prov("test-source"),
	}
}

// entityOrg extracts the org segment (first part) of a 6-part entity ID.
func entityOrg(id string) string {
	for i, ch := range id {
		if ch == '.' {
			return id[:i]
		}
	}
	return id
}

// --- Config tests ---

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     federation.Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard},
			wantErr: false,
		},
		{
			name:    "missing namespace",
			cfg:     federation.Config{MergePolicy: federation.MergePolicyStandard},
			wantErr: true,
		},
		{
			name:    "invalid merge policy",
			cfg:     federation.Config{LocalNamespace: "acme", MergePolicy: "bogus"},
			wantErr: true,
		},
		{
			name:    "default config is valid",
			cfg:     federation.DefaultConfig(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := federation.DefaultConfig()
	if cfg.LocalNamespace == "" {
		t.Error("DefaultConfig().LocalNamespace must not be empty")
	}
	if cfg.MergePolicy == "" {
		t.Error("DefaultConfig().MergePolicy must not be empty")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

// --- MergeEntity tests ---

func TestMergeEntity_PublicMergesUnconditionally(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	incoming := entity("public.semsource.golang.stdlib-net-http.function.ListenAndServe")

	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity public.* should not error: %v", err)
	}
	if result == nil {
		t.Fatal("MergeEntity public.* should return merged entity")
	}
	if result.ID != incoming.ID {
		t.Errorf("ID = %q, want %q", result.ID, incoming.ID)
	}
}

func TestMergeEntity_PublicMergesFromAnyOrg(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	incoming := entity("public.semsource.web.pkg-go-dev.doc.c821de")
	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity public.* URL entity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}
}

func TestMergeEntity_OwnOrgMerges(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	incoming := entity("acme.semsource.golang.github-com-acme-gcs.function.NewController")
	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity own org entity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}
}

func TestMergeEntity_CrossOrgRejected(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	incoming := entity("other.semsource.golang.github-com-other-lib.function.DoSomething")
	result, err := merger.MergeEntity(incoming, nil)
	if err == nil {
		t.Error("expected error for cross-org entity overwrite")
	}
	if result != nil {
		t.Error("result should be nil for rejected cross-org entity")
	}
}

func TestMergeEntity_ProvenanceAppended(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	existingProv := graph.SourceProvenance{SourceType: "git", SourceID: "source-a", Timestamp: time.Now(), Handler: "git"}
	existingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Provenance: existingProv,
	}

	incomingProv := graph.SourceProvenance{SourceType: "ast", SourceID: "source-b", Timestamp: time.Now(), Handler: "ast"}
	incomingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Provenance: incomingProv,
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}

	// Provenance must be appended, not replaced.
	// Incoming becomes primary; existing is appended to AdditionalProvenance.
	if len(result.AdditionalProvenance) == 0 {
		t.Error("expected AdditionalProvenance to contain prior provenance records after merge")
	}
	// The existing provenance record must appear in AdditionalProvenance.
	found := false
	for _, ap := range result.AdditionalProvenance {
		if ap.SourceID == existingProv.SourceID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("existing provenance (source-a) not found in AdditionalProvenance")
	}
}

func TestMergeEntity_ProvenanceAppendedChain(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	// Entity already has one additional provenance record
	existingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Provenance: prov("source-b"),
		AdditionalProvenance: []graph.SourceProvenance{
			{SourceType: "git", SourceID: "source-a", Timestamp: time.Now(), Handler: "git"},
		},
	}

	incomingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Provenance: prov("source-c"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}

	// AdditionalProvenance should now hold source-a and source-b
	if len(result.AdditionalProvenance) < 2 {
		t.Errorf("expected at least 2 additional provenance records, got %d", len(result.AdditionalProvenance))
	}
}

func TestMergeEntity_EdgeUnion(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	existingEdge := graph.GraphEdge{FromID: "public.a", ToID: "public.b", EdgeType: "calls"}
	existingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Edges:      []graph.GraphEdge{existingEdge},
		Provenance: prov("s1"),
	}

	newEdge := graph.GraphEdge{FromID: "public.a", ToID: "public.c", EdgeType: "imports"}
	incomingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Edges:      []graph.GraphEdge{newEdge},
		Provenance: prov("s2"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}

	// Both edges should be present (union, not replace)
	edgeSet := make(map[string]bool)
	for _, e := range result.Edges {
		edgeSet[e.ToID+":"+e.EdgeType] = true
	}
	if !edgeSet["public.b:calls"] {
		t.Error("existing edge public.b:calls should be preserved in union")
	}
	if !edgeSet["public.c:imports"] {
		t.Error("new edge public.c:imports should be added in union")
	}
}

func TestMergeEntity_EdgeUnionDeduplicates(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	sameEdge := graph.GraphEdge{FromID: "public.a", ToID: "public.b", EdgeType: "calls"}
	existingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Edges:      []graph.GraphEdge{sameEdge},
		Provenance: prov("s1"),
	}
	incomingEntity := graph.GraphEntity{
		ID:         "public.semsource.golang.stdlib.function.Foo",
		Edges:      []graph.GraphEdge{sameEdge}, // identical
		Provenance: prov("s2"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}
	if len(result.Edges) != 1 {
		t.Errorf("expected 1 edge after dedup union, got %d", len(result.Edges))
	}
}

// --- ApplyEvent tests ---

func TestApplyEvent_SEEDPublicEntitiesAccepted(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := seedEvent("public",
		entity("public.semsource.golang.stdlib.function.Foo"),
		entity("public.semsource.golang.stdlib.function.Bar"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent SEED public: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(result.Entities))
	}
}

func TestApplyEvent_DELTAOwnOrgAccepted(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := deltaEvent("acme",
		entity("acme.semsource.golang.github-com-acme-repo.function.DoWork"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent DELTA own org: %v", err)
	}
	if len(result.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(result.Entities))
	}
}

func TestApplyEvent_CrossOrgEntitiesFilteredOut(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	// Mix: one own-org, one cross-org, one public
	ev := deltaEvent("mixed",
		entity("acme.semsource.golang.github-com-acme-repo.function.Mine"),
		entity("other.semsource.golang.github-com-other-repo.function.Theirs"),
		entity("public.semsource.golang.stdlib.function.Shared"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent mixed: %v", err)
	}
	// Cross-org entity must be filtered; own-org and public accepted
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities (own+public), got %d", len(result.Entities))
	}
	for _, e := range result.Entities {
		org := entityOrg(e.ID)
		if org != "acme" && org != "public" {
			t.Errorf("cross-org entity leaked through: %s", e.ID)
		}
	}
}

func TestApplyEvent_RETRACTWithinOwnScope(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := retractEvent("acme",
		"acme.semsource.golang.github-com-acme-repo.function.OldFunc",
		"public.semsource.golang.stdlib.function.OldPublic",
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT own scope: %v", err)
	}
	if len(result.Retractions) != 2 {
		t.Errorf("expected 2 retractions, got %d", len(result.Retractions))
	}
}

func TestApplyEvent_RETRACTCrossOrgRejected(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := retractEvent("other",
		"other.semsource.golang.github-com-other-repo.function.TheirFunc", // cross-org: should be dropped
		"acme.semsource.golang.github-com-acme-repo.function.MyFunc",       // own org: should pass
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT cross-org: %v", err)
	}
	// Cross-org retraction must be filtered; own-org retraction passes
	if len(result.Retractions) != 1 {
		t.Errorf("expected 1 retraction (cross-org filtered), got %d", len(result.Retractions))
	}
	if result.Retractions[0] != "acme.semsource.golang.github-com-acme-repo.function.MyFunc" {
		t.Errorf("unexpected retraction ID: %s", result.Retractions[0])
	}
}

func TestApplyEvent_RETRACTPublicPasses(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	// Public entities can always be retracted
	ev := retractEvent("public",
		"public.semsource.golang.stdlib.function.DeprecatedFunc",
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT public: %v", err)
	}
	if len(result.Retractions) != 1 {
		t.Errorf("expected 1 retraction, got %d", len(result.Retractions))
	}
}

func TestApplyEvent_HEARTBEATPassesThrough(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := &graph.GraphEvent{
		Type:       graph.EventTypeHEARTBEAT,
		SourceID:   "test",
		Namespace:  "acme",
		Timestamp:  time.Now(),
		Provenance: prov("test"),
	}

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent HEARTBEAT: %v", err)
	}
	if result.Type != graph.EventTypeHEARTBEAT {
		t.Errorf("Type = %q, want %q", result.Type, graph.EventTypeHEARTBEAT)
	}
}

func TestApplyEvent_NilEventReturnsError(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	_, err := merger.ApplyEvent(nil, nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}

func TestApplyEvent_WithExistingStore_EdgeUnion(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	existingEdge := graph.GraphEdge{FromID: "public.a", ToID: "public.b", EdgeType: "calls"}
	existing := map[string]*graph.GraphEntity{
		"public.semsource.golang.stdlib.function.Foo": {
			ID:         "public.semsource.golang.stdlib.function.Foo",
			Edges:      []graph.GraphEdge{existingEdge},
			Provenance: prov("old-source"),
		},
	}

	newEdge := graph.GraphEdge{FromID: "public.a", ToID: "public.c", EdgeType: "imports"}
	ev := deltaEvent("public",
		graph.GraphEntity{
			ID:         "public.semsource.golang.stdlib.function.Foo",
			Edges:      []graph.GraphEdge{newEdge},
			Provenance: prov("new-source"),
		},
	)

	result, err := merger.ApplyEvent(ev, existing)
	if err != nil {
		t.Fatalf("ApplyEvent with store: %v", err)
	}
	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	merged := result.Entities[0]
	edgeSet := make(map[string]bool)
	for _, e := range merged.Edges {
		edgeSet[e.ToID+":"+e.EdgeType] = true
	}
	if !edgeSet["public.b:calls"] {
		t.Error("existing edge should be preserved")
	}
	if !edgeSet["public.c:imports"] {
		t.Error("new edge should be added")
	}
}

func TestApplyEvent_SEEDEmptyEntities(t *testing.T) {
	cfg := federation.Config{LocalNamespace: "acme", MergePolicy: federation.MergePolicyStandard}
	merger := federation.NewMerger(cfg)

	ev := seedEvent("acme") // no entities
	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent empty SEED: %v", err)
	}
	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(result.Entities))
	}
}
