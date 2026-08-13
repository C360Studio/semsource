package astsource

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/handler"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// Shared normalization and diff plumbing for the CI determinism gate (#127):
// ingest a fixed corpus twice and fail, naming the offending entities, on any
// difference in the resulting entity/triple set. Both the fixture-corpus gate
// (determinism_test.go) and the repo-corpus gate (determinism_repo_test.go)
// build on the types and functions in this file, so "normalize the same way"
// cannot drift between the two tiers.

// maxOffendingEntities and maxTripleDiffs bound failure output so a systemic
// break reports a diagnosable sample instead of drowning the test log.
const (
	maxOffendingEntities = 10
	maxTripleDiffs       = 5
)

// detTriple is a comparison-normalized message.Triple. Timestamp is dropped:
// it is a wall-clock read by design (every *_test.go ingest path stamps
// "now"), not part of the entity/predicate claim this gate makes. Every other
// field survives, so a routing or predicate regression still fails the gate.
type detTriple struct {
	Subject    string
	Predicate  string
	Object     string // canonical JSON encoding of message.Triple.Object
	Source     string
	Confidence float64
	Context    string
	Datatype   string
}

// detEntity is a comparison-normalized entity: the fields source/ast.EntityState
// and handler.EntityState both carry. UpdatedAt is dropped for the same reason
// as Triple.Timestamp; StorageRef is dropped per design constraint #6 — bodies
// live in ObjectStore and are not part of the entity-set claim.
type detEntity struct {
	ID              string
	IndexingProfile string
	Triples         []detTriple
}

// canonicalObject renders a triple's Object (subject-typed interface{}) into a
// value comparable across passes regardless of its concrete Go type.
func canonicalObject(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Every Object value on the paths this gate covers is a JSON-marshalable
		// primitive or an entity-ID string; a marshal failure here is a real bug
		// in the producer, not a determinism question — surface it loudly rather
		// than silently dropping the triple from comparison.
		return fmt.Sprintf("!MARSHAL_ERROR:%v:%v", v, err)
	}
	return string(b)
}

// wallClockPredicates are triples whose Predicate is wall-clock BY CONSTRUCTION
// — same rationale as the Triple.Timestamp field, just baked into the Object
// instead. semsourceast.DcCreated ("dc.terms.created") is set from
// CodeEntity.IndexedAt (time.Now(), see source/ast/entities.go's
// NewScopedCodeEntity and Triples()) on every AST entity; it was the one
// concrete false-positive this gate's own probe-becomes-proof pass found —
// two ingestion passes a few hundred milliseconds apart can straddle a
// wall-clock second, which produced a "changed" diff on ~2300 otherwise
// byte-identical entities in the repo-corpus tier before this map existed.
// Excluding it here mirrors constraint #3's stated intent (drop wall-clock
// fields) rather than the letter of "everything else must be identical" —
// see the PR description for the full false-positive trace.
var wallClockPredicates = map[string]bool{
	semsourceast.DcCreated: true,
}

// normalizeTriples converts and sorts triples by (Predicate, canonical Object)
// per design constraint #3, so append-order variance within one entity's
// triple slice can never itself cause a false failure. Wall-clock predicates
// (wallClockPredicates) are dropped entirely, same as Triple.Timestamp.
func normalizeTriples(triples []message.Triple) []detTriple {
	out := make([]detTriple, 0, len(triples))
	for _, tr := range triples {
		if wallClockPredicates[tr.Predicate] {
			continue
		}
		out = append(out, detTriple{
			Subject:    tr.Subject,
			Predicate:  tr.Predicate,
			Object:     canonicalObject(tr.Object),
			Source:     tr.Source,
			Confidence: tr.Confidence,
			Context:    tr.Context,
			Datatype:   tr.Datatype,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Predicate != out[j].Predicate {
			return out[i].Predicate < out[j].Predicate
		}
		return out[i].Object < out[j].Object
	})
	return out
}

// fromASTState adapts a source/ast.EntityState (the AST parsers' and
// BuildHierarchy's output type) to the comparison-normalized form.
func fromASTState(s *semsourceast.EntityState) detEntity {
	return detEntity{ID: s.ID, IndexingProfile: s.IndexingProfile, Triples: normalizeTriples(s.Triples)}
}

// fromHandlerState adapts a handler.EntityState (the cfgfile and doc handlers'
// output type) to the comparison-normalized form.
func fromHandlerState(s *handler.EntityState) detEntity {
	return detEntity{ID: s.ID, IndexingProfile: s.IndexingProfile, Triples: normalizeTriples(s.Triples)}
}

// tripleKey renders a detTriple into a single comparable string, used to diff
// two entities' triple sets against each other independent of slice order.
func tripleKey(tr detTriple) string {
	return strings.Join([]string{
		tr.Subject, tr.Predicate, tr.Object, tr.Source, tr.Context, tr.Datatype,
		fmt.Sprintf("%g", tr.Confidence),
	}, "\x00")
}

// triplesEqual compares two already-normalized (sorted) triple slices.
func triplesEqual(a, b []detTriple) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertDeterministic fails t naming every entity ID that differs — or is
// present on only one side — between two ingestion passes over the same
// corpus (design constraint #4). label identifies which corpus/handler
// produced the passes, so a failure is attributable at a glance.
func assertDeterministic(t *testing.T, label string, pass1, pass2 []detEntity) {
	t.Helper()

	byID1 := indexByID(pass1)
	byID2 := indexByID(pass2)

	var onlyIn1, onlyIn2, changed []string
	for id := range byID1 {
		if _, ok := byID2[id]; !ok {
			onlyIn1 = append(onlyIn1, id)
		}
	}
	for id := range byID2 {
		if _, ok := byID1[id]; !ok {
			onlyIn2 = append(onlyIn2, id)
		}
	}
	for id, e1 := range byID1 {
		e2, ok := byID2[id]
		if !ok {
			continue
		}
		if e1.IndexingProfile != e2.IndexingProfile || !triplesEqual(e1.Triples, e2.Triples) {
			changed = append(changed, id)
		}
	}

	if len(onlyIn1) == 0 && len(onlyIn2) == 0 && len(changed) == 0 {
		return
	}
	sort.Strings(onlyIn1)
	sort.Strings(onlyIn2)
	sort.Strings(changed)

	var b strings.Builder
	fmt.Fprintf(&b, "%s: non-deterministic ingest — pass1 has %d entities, pass2 has %d entities; "+
		"%d only in pass1, %d only in pass2, %d changed\n",
		label, len(pass1), len(pass2), len(onlyIn1), len(onlyIn2), len(changed))
	writeIDList(&b, "only in pass1 (missing from pass2)", onlyIn1)
	writeIDList(&b, "only in pass2 (missing from pass1)", onlyIn2)
	writeChangedEntities(&b, changed, byID1, byID2)
	t.Fatal(b.String())
}

// indexByID builds an ID → entity lookup. A duplicate ID within a single pass
// is itself a determinism-adjacent bug (two producers claimed the same
// identity); the later entry wins here, same as it would at graph-ingest.
func indexByID(entities []detEntity) map[string]detEntity {
	out := make(map[string]detEntity, len(entities))
	for _, e := range entities {
		out[e.ID] = e
	}
	return out
}

// writeIDList appends a bounded, sorted list of entity IDs under a heading.
func writeIDList(b *strings.Builder, heading string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "  %s (%d):\n", heading, len(ids))
	for i, id := range ids {
		if i >= maxOffendingEntities {
			fmt.Fprintf(b, "    ... and %d more\n", len(ids)-maxOffendingEntities)
			return
		}
		fmt.Fprintf(b, "    - %s\n", id)
	}
}

// writeChangedEntities appends a bounded, per-entity triple diff for entities
// present on both sides but with a different triple set or IndexingProfile.
func writeChangedEntities(b *strings.Builder, changed []string, byID1, byID2 map[string]detEntity) {
	if len(changed) == 0 {
		return
	}
	fmt.Fprintf(b, "  changed entities (%d):\n", len(changed))
	for i, id := range changed {
		if i >= maxOffendingEntities {
			fmt.Fprintf(b, "    ... and %d more changed entities\n", len(changed)-maxOffendingEntities)
			return
		}
		fmt.Fprintf(b, "    %s:\n", id)
		writeTripleDiff(b, byID1[id], byID2[id])
	}
}

// writeTripleDiff appends a bounded set-diff between two entities' normalized
// triples: entries only pass1 asserted, entries only pass2 asserted, and an
// IndexingProfile mismatch if present.
func writeTripleDiff(b *strings.Builder, e1, e2 detEntity) {
	set1 := tripleSet(e1.Triples)
	set2 := tripleSet(e2.Triples)

	shown := 0
	for _, tr := range e1.Triples {
		if shown >= maxTripleDiffs {
			fmt.Fprintf(b, "      ...\n")
			break
		}
		if _, ok := set2[tripleKey(tr)]; !ok {
			fmt.Fprintf(b, "      - pass1 only: %s = %s\n", tr.Predicate, tr.Object)
			shown++
		}
	}
	for _, tr := range e2.Triples {
		if shown >= maxTripleDiffs {
			fmt.Fprintf(b, "      ...\n")
			break
		}
		if _, ok := set1[tripleKey(tr)]; !ok {
			fmt.Fprintf(b, "      + pass2 only: %s = %s\n", tr.Predicate, tr.Object)
			shown++
		}
	}
	if e1.IndexingProfile != e2.IndexingProfile {
		fmt.Fprintf(b, "      IndexingProfile: pass1=%q pass2=%q\n", e1.IndexingProfile, e2.IndexingProfile)
	}
}

// tripleSet builds a lookup set of a normalized triple slice, keyed for
// order-independent membership tests.
func tripleSet(triples []detTriple) map[string]struct{} {
	out := make(map[string]struct{}, len(triples))
	for _, tr := range triples {
		out[tripleKey(tr)] = struct{}{}
	}
	return out
}
