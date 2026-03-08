package normalizer_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/normalizer"
	"github.com/c360studio/semstreams/federation"
)

func makeNormalizer(org string) *normalizer.Normalizer {
	return normalizer.New(normalizer.Config{Org: org})
}

// ---------------------------------------------------------------------------
// Normalize — one entity at a time
// ---------------------------------------------------------------------------

func TestNormalize_GitEntity(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     "github.com-acme-gcs",
		EntityType: "commit",
		Instance:   "a3f9b2",
		Properties: map[string]any{
			"author":  "alice",
			"message": "initial commit",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "acme.semsource.git.github.com-acme-gcs.commit.a3f9b2"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
	if got.Provenance.SourceType != handler.SourceTypeGit {
		t.Errorf("Provenance.SourceType = %q, want %q", got.Provenance.SourceType, handler.SourceTypeGit)
	}
	if got.Provenance.Timestamp.IsZero() {
		t.Error("Provenance.Timestamp is zero")
	}

	// Triples should be generated from Properties
	if len(got.Triples) == 0 {
		t.Error("expected at least one triple from Properties, got none")
	}
	// All triples must reference this entity as subject
	for _, tr := range got.Triples {
		if tr.Subject != wantID {
			t.Errorf("Triple.Subject = %q, want %q", tr.Subject, wantID)
		}
	}
}

func TestNormalize_TriplePredicateFormat(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     "github.com-acme-gcs",
		EntityType: "commit",
		Instance:   "a3f9b2",
		Properties: map[string]any{
			"author": "alice",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	// Predicate format should be "{domain}.{entityType}.{key}"
	found := false
	for _, tr := range got.Triples {
		if tr.Predicate == "git.commit.author" && tr.Object == "alice" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate 'git.commit.author', not found")
	}
}

func TestNormalize_ASTEntity(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
		Properties: map[string]any{
			"package": "main",
			"file":    "main.go",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "acme.semsource.golang.github.com-acme-gcs.function.NewController"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
}

func TestNormalize_DocEntity(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeDoc,
		Domain:     handler.DomainWeb,
		System:     "docs.acme.io",
		EntityType: "doc",
		Instance:   "ab12cd",
		Properties: map[string]any{
			"title": "Getting Started",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "acme.semsource.web.docs.acme.io.doc.ab12cd"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
}

func TestNormalize_ConfigEntity(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeConfig,
		Domain:     handler.DomainConfig,
		System:     "github.com-acme-gcs",
		EntityType: "gomod",
		Instance:   "fe9a12",
		Properties: map[string]any{
			"path": "go.mod",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "acme.semsource.config.github.com-acme-gcs.gomod.fe9a12"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
}

func TestNormalize_URLEntity(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeURL,
		Domain:     handler.DomainWeb,
		System:     "docs.acme.io",
		EntityType: "doc",
		Instance:   "c821de",
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "acme.semsource.web.docs.acme.io.doc.c821de"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
}

// ---------------------------------------------------------------------------
// Normalize — edges become relationship triples
// ---------------------------------------------------------------------------

func TestNormalize_EdgeBecomesRelationshipTriple(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
		Edges: []handler.RawEdge{
			{
				FromHint: "NewController",
				ToHint:   "NewService",
				EdgeType: "calls",
			},
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	// Find the relationship triple.
	wantSubject := "acme.semsource.golang.github.com-acme-gcs.function.NewController"
	wantObject := "acme.semsource.golang.github.com-acme-gcs.function.NewService"
	found := false
	for _, tr := range got.Triples {
		if tr.Subject == wantSubject && tr.Predicate == "golang.function.calls" && tr.Object == wantObject {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected relationship triple for 'calls' edge, not found")
	}
}

func TestNormalize_EdgeToTypeOverride(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeVideo,
		Domain:     handler.DomainMedia,
		System:     "media-root",
		EntityType: "keyframe",
		Instance:   "abc123-15s",
		Edges: []handler.RawEdge{
			{
				FromHint: "abc123-15s",
				ToHint:   "abc123",
				EdgeType: "keyframe_of",
				ToType:   "video",
			},
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	// The relationship triple's Object should use "video" type, not "keyframe".
	wantSubject := "acme.semsource.media.media-root.keyframe.abc123-15s"
	wantObject := "acme.semsource.media.media-root.video.abc123"
	found := false
	for _, tr := range got.Triples {
		if tr.Subject == wantSubject && tr.Object == wantObject {
			found = true
			if tr.Predicate != "media.keyframe.keyframe_of" {
				t.Errorf("Predicate = %q, want %q", tr.Predicate, "media.keyframe.keyframe_of")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected relationship triple with Object=%q, not found", wantObject)
	}
}

func TestNormalize_EdgeToTypeEmpty(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
		Edges: []handler.RawEdge{
			{
				FromHint: "NewController",
				ToHint:   "NewService",
				EdgeType: "calls",
				// ToType intentionally empty — should default to "function".
			},
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantObject := "acme.semsource.golang.github.com-acme-gcs.function.NewService"
	found := false
	for _, tr := range got.Triples {
		if obj, ok := tr.Object.(string); ok && obj == wantObject {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected relationship triple with Object=%q, not found", wantObject)
	}
}

// ---------------------------------------------------------------------------
// Normalize — org override key excluded from triples
// ---------------------------------------------------------------------------

func TestNormalize_OrgPropertyExcluded(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
		Properties: map[string]any{
			"org":     "public",
			"package": "main",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	for _, tr := range got.Triples {
		if strings.HasSuffix(tr.Predicate, ".org") {
			t.Error("'org' property should not appear as a triple")
		}
	}
}

// ---------------------------------------------------------------------------
// Normalize — missing required fields
// ---------------------------------------------------------------------------

func TestNormalize_MissingDomain(t *testing.T) {
	n := makeNormalizer("acme")
	_, err := n.Normalize(handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
	})
	if err == nil {
		t.Fatal("expected error for missing Domain, got nil")
	}
}

func TestNormalize_MissingSystem(t *testing.T) {
	n := makeNormalizer("acme")
	_, err := n.Normalize(handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		EntityType: "function",
		Instance:   "NewController",
	})
	if err == nil {
		t.Fatal("expected error for missing System, got nil")
	}
}

func TestNormalize_MissingInstance(t *testing.T) {
	n := makeNormalizer("acme")
	_, err := n.Normalize(handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
	})
	if err == nil {
		t.Fatal("expected error for missing Instance, got nil")
	}
}

// ---------------------------------------------------------------------------
// NormalizeBatch
// ---------------------------------------------------------------------------

func TestNormalizeBatch(t *testing.T) {
	n := makeNormalizer("acme")

	raws := []handler.RawEntity{
		{
			SourceType: handler.SourceTypeAST,
			Domain:     handler.DomainGolang,
			System:     "github.com-acme-gcs",
			EntityType: "function",
			Instance:   "NewController",
		},
		{
			SourceType: handler.SourceTypeGit,
			Domain:     handler.DomainGit,
			System:     "github.com-acme-gcs",
			EntityType: "commit",
			Instance:   "a3f9b2",
		},
	}

	got, err := n.NormalizeBatch(raws)
	if err != nil {
		t.Fatalf("NormalizeBatch() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("NormalizeBatch() count: got %d, want 2", len(got))
	}

	ids := map[string]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	if !ids["acme.semsource.golang.github.com-acme-gcs.function.NewController"] {
		t.Error("expected function entity ID in batch output")
	}
	if !ids["acme.semsource.git.github.com-acme-gcs.commit.a3f9b2"] {
		t.Error("expected commit entity ID in batch output")
	}
}

func TestNormalizeBatch_StopsOnError(t *testing.T) {
	n := makeNormalizer("acme")

	raws := []handler.RawEntity{
		{
			SourceType: handler.SourceTypeAST,
			Domain:     handler.DomainGolang,
			System:     "github.com-acme-gcs",
			EntityType: "function",
			Instance:   "NewController",
		},
		{
			SourceType: handler.SourceTypeAST,
			Domain:     "",
			System:     "",
			EntityType: "",
			Instance:   "",
		},
	}

	_, err := n.NormalizeBatch(raws)
	if err == nil {
		t.Fatal("expected error from NormalizeBatch with invalid entity, got nil")
	}
}

// ---------------------------------------------------------------------------
// GraphEntity ID is a valid NATS KV key
// ---------------------------------------------------------------------------

func TestNormalize_IDIsValidNATSKey(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-gcs",
		EntityType: "function",
		Instance:   "NewController",
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	if err := normalizer.ValidateNATSKVKey(got.ID); err != nil {
		t.Errorf("Normalized ID %q is not a valid NATS KV key: %v", got.ID, err)
	}
}

// ---------------------------------------------------------------------------
// Public namespace: org override
// ---------------------------------------------------------------------------

func TestNormalize_PublicNamespaceOverride(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-gin-gonic-gin",
		EntityType: "function",
		Instance:   "New",
		Properties: map[string]any{
			"org": "public",
		},
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	wantID := "public.semsource.golang.github.com-gin-gonic-gin.function.New"
	if got.ID != wantID {
		t.Errorf("ID = %q, want %q", got.ID, wantID)
	}
}

// ---------------------------------------------------------------------------
// Type assertion — Normalize returns *federation.Entity
// ---------------------------------------------------------------------------

func TestNormalize_ReturnType(t *testing.T) {
	n := makeNormalizer("acme")

	raw := handler.RawEntity{
		SourceType: handler.SourceTypeGit,
		Domain:     handler.DomainGit,
		System:     "github.com-acme-gcs",
		EntityType: "commit",
		Instance:   "deadbe",
	}

	got, err := n.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize() error: %v", err)
	}

	// Compile-time: verify the return is *federation.Entity
	var _ *federation.Entity = got
}
