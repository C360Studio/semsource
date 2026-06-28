package fusion

import (
	"sort"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ontology"
)

// Ranking weights. Resolve order (the graph's own semantic/index ranking) is the
// base signal; ontology coherence reorders locally, with lexical name match as a
// tiebreaker.
//
// We deliberately do NOT use absolute BFO/CCO tree depth as a "specificity"
// signal: depth is a modeling artifact (an artifact file sits deeper in the
// Object spine than a function does in the ICE spine), not a relevance measure,
// and it would systematically bias one domain over another. Query-concept-
// relative specificity is future work.
const (
	lexExact     = 40.0
	lexPrefix    = 20.0
	lexContains  = 8.0
	coherWeight  = 12.0 // proximity of the entity's class to the modal class of the set
	resolveScale = 4.0  // base score per resolve-rank position
)

// classOf returns an entity's BFO/CCO class IRI: the stamped entity.ontology.class
// triple if present, else derived from the ID via ontology.ClassFor (the ranker
// works even on entities emitted before the alignment, per ADR-0005).
func classOf(e *Entity) string {
	if c := e.First(ontology.ClassPredicate); c != "" {
		return c
	}
	domain, typ := entityid.Parts(e.ID)
	if iri, ok := ontology.ClassFor(domain, typ); ok {
		return iri
	}
	return ""
}

// rankEntities orders entities (given in resolve order) by ontology-aware
// relevance: resolve-rank base + lexical name match + class specificity + class
// coherence to the modal class of the resolved set (results that fit the bulk of
// what the query surfaced rank higher). names[i] is entity[i]'s display name.
func rankEntities(entities []*Entity, names []string, query string) []*Entity {
	q := strings.ToLower(strings.TrimSpace(query))
	classes := make([]string, len(entities))
	for i, e := range entities {
		classes[i] = classOf(e)
	}
	modal := modalClass(classes)

	type scored struct {
		e   *Entity
		s   float64
		pos int
	}
	arr := make([]scored, len(entities))
	for i, e := range entities {
		s := resolveScale * float64(len(entities)-i)
		s += lexicalScore(q, strings.ToLower(names[i]))
		if modal != "" && classes[i] != "" {
			if d := ontology.Distance(classes[i], modal); d >= 0 {
				s += coherWeight / float64(1+d)
			}
		}
		arr[i] = scored{e, s, i}
	}
	sort.SliceStable(arr, func(a, b int) bool {
		if arr[a].s != arr[b].s {
			return arr[a].s > arr[b].s
		}
		return arr[a].pos < arr[b].pos
	})

	out := make([]*Entity, len(arr))
	for i := range arr {
		out[i] = arr[i].e
	}
	return out
}

// lexicalScore scores a name against a lowercased query.
func lexicalScore(q, name string) float64 {
	switch {
	case name == q:
		return lexExact
	case strings.HasPrefix(name, q):
		return lexPrefix
	case strings.Contains(name, q):
		return lexContains
	default:
		return 0
	}
}

// modalClass returns the dominant non-empty class, or "" when there is no real
// mode — all classes distinct (max count 1 across more than one class) means the
// set has no coherent concept to reward proximity to, so coherence is disabled
// rather than anchored on an alphabetical accident.
func modalClass(classes []string) string {
	counts := make(map[string]int)
	best, bestN := "", 0
	for _, c := range classes {
		if c == "" {
			continue
		}
		counts[c]++
		if counts[c] > bestN || (counts[c] == bestN && c < best) {
			best, bestN = c, counts[c]
		}
	}
	if bestN < 2 && len(counts) > 1 {
		return ""
	}
	return best
}
