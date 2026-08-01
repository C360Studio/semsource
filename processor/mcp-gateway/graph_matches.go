package mcpgateway

import (
	"encoding/json"
	"strings"
)

// maxGraphMatches bounds the match list an agent receives. A search verb's job
// is to rank, not to transfer the corpus: the agent picks from the top and
// follows up with a deterministic tool on the IDs it wants.
const maxGraphMatches = 25

// titlePredicate carries an entity's human-readable name.
const titlePredicate = "dc.terms.title"

// graphMatch is one ranked hit: enough to judge relevance and to follow up with
// a deterministic tool, and nothing else.
type graphMatch struct {
	ID        string   `json:"id"`
	Type      string   `json:"type,omitempty"`
	Label     string   `json:"label,omitempty"`
	Relevance float64  `json:"relevance,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// substrateDigest mirrors the substrate's EntityDigest.
type substrateDigest struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Relevance float64  `json:"relevance"`
	Tags      []string `json:"tags"`
}

// substrateEntity is the part of an EntityState needed to derive a match when
// the substrate returned entities but no digests.
type substrateEntity struct {
	ID      string `json:"id"`
	Triples []struct {
		Predicate string `json:"predicate"`
		Object    string `json:"object"`
	} `json:"triples"`
}

// graphSearchBody is the substrate payload, read for the fields a ranked match
// list needs.
type graphSearchBody struct {
	EntityDigests      []substrateDigest `json:"entity_digests"`
	Entities           []substrateEntity `json:"entities"`
	EntityIDs          []string          `json:"entity_ids"`
	CommunitySummaries []json.RawMessage `json:"community_summaries"`
	Answer             string            `json:"answer"`
	Count              int               `json:"count"`
}

// deriveMatches turns the substrate response into a bounded ranked list.
//
// Digests are preferred — they carry the substrate's own relevance and labels.
// But the substrate populates `entity_digests` in only three places, none of
// them on the `globalSearch` paths below the summarize threshold, which is the
// common case: a 38-hit query returns 38 full EntityStates and zero digests.
// Upstream's own agent-facing formatter reads digests only, so it renders that
// result as "no summary available" and drops all 38 hits (semstreams#823).
//
// So when digests are absent we derive the same shape from the entities the
// response DID carry. Nothing is invented — the ID is the substrate's, the type
// is the ID's own type segment, and the label is the entity's dc.terms.title
// triple. Relevance is simply absent rather than guessed, because the substrate
// did not report one on this path.
func deriveMatches(body *graphSearchBody) (matches []graphMatch, total int, truncated bool) {
	switch {
	case len(body.EntityDigests) > 0:
		total = len(body.EntityDigests)
		for _, d := range body.EntityDigests {
			matches = append(matches, graphMatch{
				ID: d.ID, Type: d.Type, Label: d.Label, Relevance: d.Relevance, Tags: d.Tags,
			})
		}
	case len(body.Entities) > 0:
		total = len(body.Entities)
		for _, e := range body.Entities {
			matches = append(matches, graphMatch{
				ID: e.ID, Type: entityTypeSegment(e.ID), Label: entityTitle(e),
			})
		}
	case len(body.EntityIDs) > 0:
		// The auto-summarize path can return IDs without digests.
		total = len(body.EntityIDs)
		for _, id := range body.EntityIDs {
			matches = append(matches, graphMatch{ID: id, Type: entityTypeSegment(id)})
		}
	}

	if len(matches) > maxGraphMatches {
		matches = matches[:maxGraphMatches]
		truncated = true
	}
	return matches, total, truncated
}

// entityTypeSegment returns the type segment of a 6-part entity ID. The ID is
// the substrate's; this only reads it.
func entityTypeSegment(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// entityTitle reads the entity's human-readable name from its own triples.
func entityTitle(e substrateEntity) string {
	for _, t := range e.Triples {
		if t.Predicate == titlePredicate {
			return t.Object
		}
	}
	return ""
}

// graphSearchResult is what graph_search returns: the substrate's own values —
// its answer, its community summaries, its entity IDs — plus the derived
// disclosure, and nothing invented. The match list is bounded; TotalMatches and
// Truncated keep that honest rather than presenting a cap as the whole result.
type graphSearchResult struct {
	Retrieval          retrievalDisclosure `json:"retrieval"`
	Answer             string              `json:"answer,omitempty"`
	Matches            []graphMatch        `json:"matches"`
	TotalMatches       int                 `json:"total_matches"`
	Truncated          bool                `json:"truncated"`
	CommunitySummaries []json.RawMessage   `json:"community_summaries,omitempty"`
}
