package mcpgateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	gtypes "github.com/c360studio/semstreams/graph"

	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// maxGraphMatches bounds the match list an agent receives. A search verb's job
// is to rank, not to transfer the corpus: the agent picks from the top and
// follows up with a deterministic tool on the IDs it wants.
const maxGraphMatches = 25

// titlePredicate carries an entity's human-readable name.
const titlePredicate = "dc.terms.title"

// Property rendering caps. A match may carry the entity's own VALUE facts
// (versions, kinds, scopes) so a value question is answerable in one call —
// but bounded hard, because rendering triples wholesale was measured at 197 KB
// for a 38-hit query (11x code_search). Ranking, not corpus transfer.
const (
	maxMatchProperties   = 8
	maxPropertyValueSize = 160
)

// valuePredicates is the explicit allowlist of value-bearing predicates a
// match may render, in declaration order (rendering follows THIS order, never
// map iteration — a wrong-ordered output is the determinism-gate class).
// Relationship edges, bodies, signatures, and wall-clock stamps are excluded
// by not being listed: the failure mode of omission is the status quo
// (absence), never bloat.
var valuePredicates = []string{
	source.ConfigDepName,
	source.ConfigDepVersion,
	source.ConfigDepKind,
	source.ConfigDepScope,
	source.ConfigDepConfiguration,
	source.ConfigDepIndirect,
	source.ConfigModulePath,
	source.ConfigModuleGoVer,
	source.ConfigPkgName,
	source.ConfigPkgVersion,
	source.ConfigProjectGroup,
	source.ConfigProjectArtifact,
	source.ConfigProjectVersion,
	source.ConfigProjectPackaging,
	source.ConfigProjectBuild,
	source.ConfigImageName,
	source.ConfigFilePath,
	semsourceast.CodeVersion,
}

// graphMatch is one ranked hit: enough to judge relevance and to follow up with
// a deterministic tool, and nothing else — plus the entity's own allowlisted
// value facts when the substrate response already carried its triples.
type graphMatch struct {
	ID         string            `json:"id"`
	Type       string            `json:"type,omitempty"`
	Label      string            `json:"label,omitempty"`
	Relevance  float64           `json:"relevance,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// substrateDigest mirrors the substrate's EntityDigest.
type substrateDigest struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Relevance float64  `json:"relevance"`
	Tags      []string `json:"tags"`
}

// substrateEntity is the substrate's own EntityState wire type. Decoding into
// the upstream type (Object is `any`) absorbs every scalar shape the wire can
// carry: a hand-mirrored struct with a string-typed Object failed the WHOLE
// response unmarshal on real entities (numeric code.metric.start-line,
// source.doc.chunk-index — measured live), silently collapsing every match
// into the disclosure-only fallback.
type substrateEntity = gtypes.EntityState

// objectScalar renders a triple object for match rendering: strings pass
// through, numbers and booleans render compactly, and nil and composites
// (maps, slices) are not value-shaped so they render as absent — a null must
// never become the literal string "null" in an agent-facing property.
func objectScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
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
				Properties: entityProperties(e),
			})
		}
	case len(body.EntityIDs) > 0:
		// The auto-summarize path can return IDs without digests.
		total = len(body.EntityIDs)
		for _, id := range body.EntityIDs {
			matches = append(matches, graphMatch{ID: id, Type: entityTypeSegment(id)})
		}
	}

	// The substrate's own count is the true hit total when it exceeds what the
	// shape carried (a summarized response can report more hits than digests,
	// and a hydrating response can drop not-found IDs). Honesty invariant:
	// whenever fewer matches render than exist, truncated says so — an agent
	// must never read a partial list as the complete hit set.
	if body.Count > total {
		total = body.Count
	}
	if len(matches) > maxGraphMatches {
		matches = matches[:maxGraphMatches]
	}
	truncated = len(matches) < total
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
// First RENDERABLE wins, same as entityProperties: a null residue triple must
// not mask a later real title.
func entityTitle(e substrateEntity) string {
	for _, t := range e.Triples {
		if t.Predicate != titlePredicate {
			continue
		}
		if v := objectScalar(t.Object); v != "" {
			return v
		}
	}
	return ""
}

// entityProperties renders the entity's allowlisted value facts, bounded by
// maxMatchProperties and maxPropertyValueSize. Values are the substrate's own
// triple objects — nothing derived, nothing invented. Iteration follows the
// allowlist's declaration order so output is deterministic; per predicate the
// first triple wins, matching entityTitle's semantics. Returns nil (omitted
// from JSON) when the entity carries no allowlisted facts, so code and doc
// matches keep today's exact shape.
func entityProperties(e substrateEntity) map[string]string {
	var props map[string]string
	for _, pred := range valuePredicates {
		if len(props) >= maxMatchProperties {
			break
		}
		for _, t := range e.Triples {
			if t.Predicate != pred {
				continue
			}
			v := objectScalar(t.Object)
			if v == "" {
				// Unrenderable (null/composite/empty): keep scanning — the FIRST
				// RENDERABLE value wins, so a null residue triple cannot mask a
				// later real value for the same predicate.
				continue
			}
			if props == nil {
				props = make(map[string]string, maxMatchProperties)
			}
			props[pred] = truncateValue(v)
			break
		}
	}
	return props
}

// truncateValue enforces the per-value byte cap without ever splitting a
// UTF-8 rune, and marks the cut visibly: a truncated value must never be
// mistakable for the substrate's real triple object — an agent quoting a
// clean-looking prefix as the actual value is the silent-wrong-value class.
func truncateValue(v string) string {
	if len(v) <= maxPropertyValueSize {
		return v
	}
	const marker = "…" // 3 bytes
	cut := maxPropertyValueSize - len(marker)
	for cut > 0 && !utf8.RuneStart(v[cut]) {
		cut--
	}
	return v[:cut] + marker
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
