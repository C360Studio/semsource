package supersession

import (
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semstreams/message"
)

// edgeSource tags emitted lineage triples with their provenance.
const edgeSource = "supersession"

// change classification values for code.lineage.change.
const (
	changeChanged   = "changed"
	changeUnchanged = "unchanged"
)

// passStats summarizes one correspondence/supersession pass for the run report.
type passStats struct {
	Entities      int  `json:"entities"`      // versioned code entities enumerated
	Groups        int  `json:"groups"`        // correspondence groups (incl. singletons)
	Corresponding int  `json:"corresponding"` // groups with >1 member
	Supersedes    int  `json:"supersedes"`    // supersedes edges computed (adjacent pairs)
	Incomparable  int  `json:"incomparable"`  // adjacent pairs skipped as non-orderable
	Changed       int  `json:"changed"`       // pairs classified changed
	Unchanged     int  `json:"unchanged"`     // pairs classified unchanged
	Truncated     bool `json:"truncated"`     // enumeration hit max_entities — lineage may be incomplete
}

// desiredEdges computes the full lineage triple set each entity should carry,
// keyed by subject entity ID, plus pass statistics. For every correspondence
// group it orders the members and walks adjacent, comparable pairs, emitting
// newer→older supersedes, the inverse older→newer superseded_by, and a
// changed/unchanged marker on the newer entity when both body hashes are known.
//
// This is pure and deterministic: the same graph snapshot always yields the same
// triples, so re-runs (after diffNew) converge to no-ops.
//
// Known limitation (retention-first, design D5 + Risks): edges are adjacent over
// the CURRENTLY-known version set and are never retracted. Ingesting a version
// BETWEEN two already-linked versions adds new adjacent edges but leaves the old
// skip edge (and its change marker) in place, so that entity can then carry two
// supersedes/change objects. Reconciling this needs a graph-aware delete, which
// ADR-0008 keeps off the critical path; a future compaction pass is the fix.
func desiredEdges(groups map[corrKey][]candidate) (map[string][]message.Triple, passStats) {
	desired := make(map[string][]message.Triple)
	var stats passStats
	stats.Groups = len(groups)

	for _, group := range groups {
		if len(group) > 1 {
			stats.Corresponding++
		}
		ordered := orderGroup(group)
		for i := 1; i < len(ordered); i++ {
			older, newer := ordered[i-1], ordered[i]
			if !versionComparable(older, newer) {
				stats.Incomparable++
				continue
			}
			stats.Supersedes++

			desired[newer.id] = append(desired[newer.id],
				lineageReferenceTriple(newer.id, semsourceast.CodeSupersedes, older.id))
			desired[older.id] = append(desired[older.id],
				lineageReferenceTriple(older.id, semsourceast.CodeSupersededBy, newer.id))

			if change, ok := classifyChange(older, newer); ok {
				desired[newer.id] = append(desired[newer.id],
					lineageLiteralTriple(newer.id, semsourceast.CodeLineageChange, change))
				if change == changeChanged {
					stats.Changed++
				} else {
					stats.Unchanged++
				}
			}
		}
	}
	return desired, stats
}

// classifyChange compares body hashes across a corresponding pair. It reports
// ("changed"|"unchanged", true) only when both hashes are known AND come from
// the same predicate (their encodings differ across predicates, so a
// cross-predicate compare would fabricate a bogus "changed"). Otherwise it
// returns ("", false) and the marker is omitted rather than guessed.
func classifyChange(older, newer candidate) (string, bool) {
	if older.bodyHash == "" || newer.bodyHash == "" {
		return "", false
	}
	if older.bodyHashKind != newer.bodyHashKind {
		return "", false
	}
	if older.bodyHash != newer.bodyHash {
		return changeChanged, true
	}
	return changeUnchanged, true
}

// diffNew returns, per entity, only the desired triples the entity does not
// already carry (matched on predicate+object). This is what makes the pass
// idempotent: the graph-ingest merge appends without de-duplicating, so
// re-emitting an existing edge would duplicate it — diffNew drops those. Entities
// whose delta is empty are omitted entirely (nothing to publish).
func diffNew(desired, existing map[string][]message.Triple) map[string][]message.Triple {
	delta := make(map[string][]message.Triple)
	for id, want := range desired {
		have := predicateObjectSet(existing[id])
		var fresh []message.Triple
		for _, t := range want {
			if _, ok := have[t.Predicate+"\x00"+objectString(t.Object)]; ok {
				continue
			}
			fresh = append(fresh, t)
		}
		if len(fresh) > 0 {
			delta[id] = fresh
		}
	}
	return delta
}

// predicateObjectSet indexes triples by "predicate\x00object" for membership
// tests in diffNew.
func predicateObjectSet(triples []message.Triple) map[string]struct{} {
	set := make(map[string]struct{}, len(triples))
	for i := range triples {
		set[triples[i].Predicate+"\x00"+objectString(triples[i].Object)] = struct{}{}
	}
	return set
}

func objectString(o any) string {
	if s, ok := o.(string); ok {
		return s
	}
	return ""
}

// lineageReferenceTriple builds a directional lineage relationship triple. Timestamp is
// left zero so the triple is byte-stable across runs (diffNew matches on
// predicate+object, so provenance fields do not affect idempotency).
func lineageReferenceTriple(subject, predicate, object string) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Datatype:   message.EntityReferenceDatatype,
		Source:     edgeSource,
		Confidence: 1.0,
	}
}

// lineageLiteralTriple records lineage metadata whose object is not an entity
// identity, so change classifications remain ordinary string literals.
func lineageLiteralTriple(subject, predicate, object string) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     edgeSource,
		Confidence: 1.0,
	}
}
