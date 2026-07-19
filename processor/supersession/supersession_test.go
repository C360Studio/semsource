package supersession

import (
	"testing"
	"time"

	semsourceast "github.com/c360studio/semsource/source/ast"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// --- helpers ---------------------------------------------------------------

// codeEntity builds an enumerated EntityState with the version-scoping triples
// the pass reads. Empty bodyHash/created omit those triples.
func codeEntity(id, project, version, path, name, ctype, pkg, bodyHash, created string) gtypes.EntityState {
	tr := []message.Triple{
		{Subject: id, Predicate: semsourceast.CodeType, Object: ctype},
		{Subject: id, Predicate: semsourceast.DcTitle, Object: name},
		{Subject: id, Predicate: semsourceast.CodePath, Object: path},
		{Subject: id, Predicate: semsourceast.CodePackage, Object: pkg},
		{Subject: id, Predicate: semsourceast.CodeProject, Object: project},
		{Subject: id, Predicate: semsourceast.CodeVersion, Object: version},
	}
	if bodyHash != "" {
		tr = append(tr, message.Triple{Subject: id, Predicate: semsourceast.CodeBodyKey, Object: bodyHash})
	}
	if created != "" {
		tr = append(tr, message.Triple{Subject: id, Predicate: semsourceast.DcCreated, Object: created})
	}
	return gtypes.EntityState{ID: id, Triples: tr}
}

// cand builds a candidate that always corresponds with other cand()s (fixed
// org/project/path/name/type/pkg), varying only id, version, body hash, and
// first-index time. Body hashes here model the code.body.key predicate ("bodykey"
// kind), matching the "code:<sha>" values the tests pass.
func cand(id, version, bodyHash string, indexedAt time.Time) candidate {
	kind := ""
	if bodyHash != "" {
		kind = "bodykey"
	}
	return candidate{
		id: id, org: "acme", project: "semstreams", version: version,
		path: "pkg/run.go", name: "Run", ctype: "function", pkg: "run",
		bodyHash: bodyHash, bodyHashKind: kind, indexedAt: indexedAt,
	}
}

func hasTriple(triples []message.Triple, pred, obj string) bool {
	for _, t := range triples {
		if t.Predicate == pred && objectString(t.Object) == obj {
			return true
		}
	}
	return false
}

func assertTriple(t *testing.T, triples []message.Triple, pred, obj string) {
	t.Helper()
	if !hasTriple(triples, pred, obj) {
		t.Errorf("missing triple %s -> %s in %v", pred, obj, triples)
	}
}

func assertReferenceTriple(t *testing.T, triples []message.Triple, pred, obj string) {
	t.Helper()
	for _, triple := range triples {
		if triple.Predicate != pred || objectString(triple.Object) != obj {
			continue
		}
		if triple.Datatype != message.EntityReferenceDatatype {
			t.Errorf("triple %s -> %s datatype = %q, want %q", pred, obj, triple.Datatype, message.EntityReferenceDatatype)
		}
		if !message.IsValidEntityID(obj) {
			t.Errorf("triple %s -> %s object is not a canonical entity ID", pred, obj)
		}
		return
	}
	t.Errorf("missing triple %s -> %s in %v", pred, obj, triples)
}

// --- 5.1 correspondence ----------------------------------------------------

func TestCorrespondence_SameSymbolAcrossVersionsCorresponds(t *testing.T) {
	e1 := codeEntity("acme.semsource.golang.semstreams-v1-9-0.function.pkg-run-go-Run",
		"semstreams", "v1.9.0", "pkg/run.go", "Run", "function", "run", "code:aaa", "")
	e2 := codeEntity("acme.semsource.golang.semstreams-v1-10-0.function.pkg-run-go-Run",
		"semstreams", "v1.10.0", "pkg/run.go", "Run", "function", "run", "code:bbb", "")

	c1, ok1 := candidateFromEntity(e1)
	c2, ok2 := candidateFromEntity(e2)
	if !ok1 || !ok2 {
		t.Fatalf("both versioned entities should be eligible (ok1=%v ok2=%v)", ok1, ok2)
	}
	groups := groupByCorrespondence([]candidate{c1, c2})
	if len(groups) != 1 {
		t.Fatalf("same symbol at two versions must form 1 group, got %d", len(groups))
	}
	for _, g := range groups {
		if len(g) != 2 {
			t.Fatalf("correspondence group should have 2 members, got %d", len(g))
		}
	}
}

func TestCorrespondence_DifferentSourcesDoNotCorrespond(t *testing.T) {
	// Identical path/name/type/pkg but different project (source identity).
	a := codeEntity("acme.semsource.golang.alpha.function.pkg-run-go-Run",
		"alpha", "v1.0.0", "pkg/run.go", "Run", "function", "run", "code:a", "")
	b := codeEntity("acme.semsource.golang.beta.function.pkg-run-go-Run",
		"beta", "v1.0.0", "pkg/run.go", "Run", "function", "run", "code:b", "")

	ca, _ := candidateFromEntity(a)
	cb, _ := candidateFromEntity(b)
	groups := groupByCorrespondence([]candidate{ca, cb})
	if len(groups) != 2 {
		t.Fatalf("different sources must not correspond; want 2 groups, got %d", len(groups))
	}
}

func TestCandidateFromEntity_SkipsVersionless(t *testing.T) {
	e := gtypes.EntityState{
		ID: "acme.semsource.golang.semstreams.function.pkg-run-go-Run",
		Triples: []message.Triple{
			{Predicate: semsourceast.CodeProject, Object: "semstreams"}, // project but NO version
			{Predicate: semsourceast.CodePath, Object: "pkg/run.go"},
		},
	}
	if _, ok := candidateFromEntity(e); ok {
		t.Fatal("version-less entity must not be an eligible candidate")
	}
}

func TestCandidateFromEntity_ExtractsFieldsAndOrg(t *testing.T) {
	e := codeEntity("acme.semsource.golang.semstreams-v1-9-0.function.pkg-run-go-Run",
		"semstreams", "v1.9.0", "pkg/run.go", "Run", "function", "run", "code:aaa",
		"2026-07-04T10:00:00Z")
	c, ok := candidateFromEntity(e)
	if !ok {
		t.Fatal("expected eligible candidate")
	}
	if c.org != "acme" {
		t.Errorf("org = %q, want acme (ID segment[0])", c.org)
	}
	if c.project != "semstreams" || c.version != "v1.9.0" {
		t.Errorf("project/version = %q/%q, want semstreams/v1.9.0", c.project, c.version)
	}
	if c.bodyHash != "code:aaa" {
		t.Errorf("bodyHash = %q, want code:aaa (code.body.key)", c.bodyHash)
	}
	if c.indexedAt.IsZero() {
		t.Error("indexedAt should parse from dc.terms.created")
	}
}

// --- ordering --------------------------------------------------------------

func TestOrderGroup_SemverNotLexical(t *testing.T) {
	// v1.9.0 < v1.10.0 by semver, though lexically "v1.10.0" < "v1.9.0".
	in := []candidate{
		cand("c", "v1.10.0", "h", time.Time{}),
		cand("a", "v1.9.0", "h", time.Time{}),
		cand("b", "v1.9.5", "h", time.Time{}),
	}
	ordered := orderGroup(in)
	want := []string{"v1.9.0", "v1.9.5", "v1.10.0"}
	for i, w := range want {
		if ordered[i].version != w {
			t.Errorf("position %d: version = %q, want %q", i, ordered[i].version, w)
		}
	}
}

// TestOrderGroup_NonSemverNaturalOrder pins D2 (version-registration-surface):
// non-semver versions order by natural version-string comparison — a pure
// function of the strings — never by index timestamps, which graph-ingest
// rewrites on restart (the audit's lineage-inversion defect).
func TestOrderGroup_NonSemverNaturalOrder(t *testing.T) {
	// Deliberately adversarial timestamps: "develop" indexed LATER than
	// "main"; natural string order (develop < main) must win regardless.
	develop := cand("dev", "develop", "h", time.Unix(200, 0))
	main := cand("main", "main", "h", time.Unix(100, 0))
	ordered := orderGroup([]candidate{main, develop})
	if ordered[0].id != "dev" || ordered[1].id != "main" {
		t.Errorf("natural order = [%s %s], want [dev main]", ordered[0].id, ordered[1].id)
	}
	if !versionComparable(ordered[0], ordered[1]) {
		t.Error("distinct non-semver version strings should be comparable")
	}
	same1 := cand("a", "snapshot", "h", time.Unix(100, 0))
	same2 := cand("b", "snapshot", "h", time.Unix(999, 0))
	if versionComparable(same1, same2) {
		t.Error("identical version strings must be incomparable (timestamps must not matter)")
	}
}

// TestCandidateLess_NaturalNonSemver pins the numeric-aware comparison: digit
// runs by value, text runs lexically, plus transitivity spot-checks.
func TestCandidateLess_NaturalNonSemver(t *testing.T) {
	at := func(v string) candidate { return cand("id-"+v, v, "h", time.Unix(0, 0)) }
	// All strings here are genuinely non-semver ("v9"-style parses as valid
	// semver and orders in the semver partition, which its own tests cover).
	pairs := []struct{ lo, hi string }{
		{"build9", "build10"},  // numeric run by value, not lexically
		{"r2023b", "r2024a"},   // year then letter
		{"build7", "build07x"}, // equal numeric value, longer string later
		{"alpha", "beta"},
	}
	for _, p := range pairs {
		if !candidateLess(at(p.lo), at(p.hi)) || candidateLess(at(p.hi), at(p.lo)) {
			t.Errorf("want %q < %q in natural order", p.lo, p.hi)
		}
	}
	// Transitivity spot-check within the non-semver partition.
	seq := []string{"alpha", "beta", "build9", "build10"}
	for i := 0; i < len(seq); i++ {
		for j := i + 1; j < len(seq); j++ {
			if !candidateLess(at(seq[i]), at(seq[j])) {
				t.Errorf("transitive chain broken: %q !< %q", seq[i], seq[j])
			}
		}
	}
}

// TestOrdering_RestartStable pins D2's core claim: rewriting every index
// timestamp (what a restart/re-ingest does to dc.terms.created) cannot change
// non-semver ordering or edge comparability.
func TestOrdering_RestartStable(t *testing.T) {
	before := []candidate{
		cand("a", "r2024a", "h", time.Unix(100, 0)),
		cand("b", "r2023b", "h", time.Unix(200, 0)),
	}
	after := []candidate{ // same versions, timestamps inverted by "restart"
		cand("a", "r2024a", "h", time.Unix(999, 0)),
		cand("b", "r2023b", "h", time.Unix(1, 0)),
	}
	ob, oa := orderGroup(before), orderGroup(after)
	if ob[0].id != oa[0].id || ob[1].id != oa[1].id {
		t.Fatalf("restart flipped ordering: before=[%s %s] after=[%s %s]",
			ob[0].id, ob[1].id, oa[0].id, oa[1].id)
	}
	if ob[0].id != "b" {
		t.Errorf("natural order start = %s, want b (r2023b oldest)", ob[0].id)
	}
}

// --- 5.2 supersession ------------------------------------------------------

func TestSupersession_NewerSupersedesOlderWithInverse(t *testing.T) {
	older := cand("acme.semsource.golang.demo.function.older", "v1.9.0", "code:a", time.Time{})
	newer := cand("acme.semsource.golang.demo.function.newer", "v1.10.0", "code:b", time.Time{})

	// Input order deliberately reversed — ordering is by version, not input.
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{newer, older}))
	if stats.Supersedes != 1 {
		t.Fatalf("supersedes edges = %d, want 1", stats.Supersedes)
	}
	assertReferenceTriple(t, desired[newer.id], semsourceast.CodeSupersedes, older.id)
	assertReferenceTriple(t, desired[older.id], semsourceast.CodeSupersededBy, newer.id)
}

func TestSupersession_NewOnlySymbolNoEdge(t *testing.T) {
	only := cand("id-a", "v1.0.0", "code:a", time.Time{})
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{only}))
	if stats.Supersedes != 0 {
		t.Fatalf("singleton group must produce no supersedes edge, got %d", stats.Supersedes)
	}
	if len(desired) != 0 {
		t.Fatalf("singleton group must produce no edges, got %d entities", len(desired))
	}
}

// --- 5.3 idempotency -------------------------------------------------------

func TestSupersession_IdempotentReRun(t *testing.T) {
	older := cand("id-older", "v1.9.0", "code:a", time.Time{})
	newer := cand("id-newer", "v1.10.0", "code:b", time.Time{})
	desired, _ := desiredEdges(groupByCorrespondence([]candidate{older, newer}))

	// First run: graph carries no lineage edges yet.
	delta1 := diffNew(desired, map[string][]message.Triple{})
	if len(delta1) == 0 {
		t.Fatal("first run should produce lineage edges")
	}

	// Simulate the graph now carrying exactly those edges.
	existing := map[string][]message.Triple{}
	for id, trs := range delta1 {
		existing[id] = append(existing[id], trs...)
	}

	// Second run over the unchanged graph must be a no-op.
	delta2 := diffNew(desired, existing)
	if len(delta2) != 0 {
		t.Fatalf("re-run over unchanged graph must add nothing, got %d entities with new triples", len(delta2))
	}
}

// --- 5.4 incomparable versions ---------------------------------------------

// Distinct non-semver versions of ONE project now relate with a stable,
// string-derived direction (D2) — the old same-timestamp incomparability made
// lineage a function of ingest timing luck. Branch-scoped sources cannot meet
// in one correspondence group (BranchScopedSlug differentiates project), so
// in-group versions are genuinely declared versions of the same project.
func TestSupersession_DistinctNonSemverVersionsRelateStably(t *testing.T) {
	t0 := time.Unix(100, 0)
	a := cand("id-a", "feature-x", "code:a", t0) // non-semver
	b := cand("id-b", "feature-y", "code:b", t0) // non-semver, same timestamp
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{a, b}))
	if stats.Supersedes != 1 {
		t.Fatalf("distinct non-semver versions must relate, got %d edges", stats.Supersedes)
	}
	if stats.Incomparable != 0 {
		t.Fatalf("incomparable count = %d, want 0", stats.Incomparable)
	}
	if len(desired) == 0 {
		t.Fatal("expected supersession edges")
	}
}

// A semver/non-semver pair stays cross-scheme incomparable — no guessed edges.
func TestSupersession_CrossSchemeStaysIncomparable(t *testing.T) {
	t0 := time.Unix(100, 0)
	a := cand("id-a", "1.2.3", "code:a", t0)
	b := cand("id-b", "snapshot", "code:b", t0)
	_, stats := desiredEdges(groupByCorrespondence([]candidate{a, b}))
	if stats.Supersedes != 0 || stats.Incomparable != 1 {
		t.Fatalf("cross-scheme pair: supersedes=%d incomparable=%d, want 0/1", stats.Supersedes, stats.Incomparable)
	}
}

// --- 5.5 changed classification --------------------------------------------

func TestSupersession_ChangedWhenBodyDiffers(t *testing.T) {
	older := cand("id-older", "v1.9.0", "code:a", time.Time{})
	newer := cand("id-newer", "v1.10.0", "code:b", time.Time{})
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{older, newer}))
	assertTriple(t, desired[newer.id], semsourceast.CodeLineageChange, changeChanged)
	if stats.Changed != 1 || stats.Unchanged != 0 {
		t.Fatalf("changed/unchanged = %d/%d, want 1/0", stats.Changed, stats.Unchanged)
	}
}

func TestSupersession_UnchangedWhenBodyIdentical(t *testing.T) {
	older := cand("id-older", "v1.9.0", "code:same", time.Time{})
	newer := cand("id-newer", "v1.10.0", "code:same", time.Time{})
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{older, newer}))
	assertTriple(t, desired[newer.id], semsourceast.CodeLineageChange, changeUnchanged)
	if stats.Unchanged != 1 || stats.Changed != 0 {
		t.Fatalf("changed/unchanged = %d/%d, want 0/1", stats.Changed, stats.Unchanged)
	}
}

func TestClassifyChange_UnknownWhenBodyHashMissing(t *testing.T) {
	older := cand("id-older", "v1.9.0", "", time.Time{}) // no body hash
	newer := cand("id-newer", "v1.10.0", "code:b", time.Time{})
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{older, newer}))
	// Supersedes edge still emitted; the change marker is omitted (indeterminate).
	assertTriple(t, desired[newer.id], semsourceast.CodeSupersedes, older.id)
	if hasTriple(desired[newer.id], semsourceast.CodeLineageChange, changeChanged) ||
		hasTriple(desired[newer.id], semsourceast.CodeLineageChange, changeUnchanged) {
		t.Error("change marker must be omitted when a body hash is unknown")
	}
	if stats.Changed != 0 || stats.Unchanged != 0 {
		t.Fatalf("no classification expected, got changed=%d unchanged=%d", stats.Changed, stats.Unchanged)
	}
}

func TestClassifyChange_CrossPredicateHashOmitsMarker(t *testing.T) {
	// One entity's hash is code.body.key ("code:<sha>"), the other's is
	// code.artifact.hash (bare digest). Even if the underlying body is identical
	// the encodings differ, so the marker must be omitted, never guessed.
	older := cand("id-older", "v1.9.0", "code:abc", time.Time{})
	older.bodyHashKind = "bodykey"
	newer := cand("id-newer", "v1.10.0", "abc", time.Time{})
	newer.bodyHashKind = "hash"

	desired, stats := desiredEdges(groupByCorrespondence([]candidate{older, newer}))
	assertTriple(t, desired[newer.id], semsourceast.CodeSupersedes, older.id)
	if hasTriple(desired[newer.id], semsourceast.CodeLineageChange, changeChanged) ||
		hasTriple(desired[newer.id], semsourceast.CodeLineageChange, changeUnchanged) {
		t.Error("change marker must be omitted when the two hashes come from different predicates")
	}
	if stats.Changed != 0 || stats.Unchanged != 0 {
		t.Fatalf("no classification expected across predicates, got changed=%d unchanged=%d", stats.Changed, stats.Unchanged)
	}
}

func TestSupersession_CrossSchemeSemverAndRefIncomparable(t *testing.T) {
	// A semver release and a non-semver ref that share a correspondence key must
	// NOT be related — ordering them by ingest timing would assert a direction
	// the versions don't carry.
	release := cand("id-release", "v1.0.0", "code:a", time.Unix(100, 0))
	ref := cand("id-ref", "main", "code:b", time.Unix(200, 0)) // indexed later
	desired, stats := desiredEdges(groupByCorrespondence([]candidate{release, ref}))
	if stats.Supersedes != 0 {
		t.Fatalf("cross-scheme pair must get no edge, got %d", stats.Supersedes)
	}
	if stats.Incomparable != 1 {
		t.Fatalf("cross-scheme incomparable count = %d, want 1", stats.Incomparable)
	}
	if len(desired) != 0 {
		t.Fatalf("no edges expected across schemes, got %d entities", len(desired))
	}
}

// --- 5.6 retention-safety (additive-only) ----------------------------------

func TestPass_EmitsOnlyAdditiveLineageTriples(t *testing.T) {
	older := cand("id-older", "v1.9.0", "code:a", time.Time{})
	newer := cand("id-newer", "v1.10.0", "code:b", time.Time{})
	desired, _ := desiredEdges(groupByCorrespondence([]candidate{older, newer}))

	lineage := map[string]bool{
		semsourceast.CodeSupersedes:    true,
		semsourceast.CodeSupersededBy:  true,
		semsourceast.CodeLineageChange: true,
	}
	for id, trs := range desired {
		if len(trs) == 0 {
			t.Errorf("entity %s has an empty desired set", id)
		}
		for _, tr := range trs {
			if !lineage[tr.Predicate] {
				t.Errorf("entity %s: emitted non-lineage predicate %q — the pass must be additive lineage only", id, tr.Predicate)
			}
			if tr.Subject != id {
				t.Errorf("entity %s: triple subject %q must equal the entity ID", id, tr.Subject)
			}
		}
	}
}

// --- config ----------------------------------------------------------------

func TestConfig_Validate(t *testing.T) {
	def := DefaultConfig()
	if err := def.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
	bad := DefaultConfig()
	bad.Interval = "not-a-duration"
	if err := bad.Validate(); err == nil {
		t.Error("invalid interval should fail validation")
	}
	neg := DefaultConfig()
	neg.MaxEntities = -1
	if err := neg.Validate(); err == nil {
		t.Error("negative max_entities should fail validation")
	}
}
