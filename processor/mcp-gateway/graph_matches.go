package mcpgateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	gtypes "github.com/c360studio/semengine/graph"

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

// graphSearchReply is searchGraph's enveloped reply at the pinned substrate:
// the word search's matches, the community groups holding them, a plain-text
// answer built from those fields, and the hints the question carried beyond
// its words.
type graphSearchReply = gtypes.QueryResponse[gtypes.GlobalSearchData]

// deriveMatches turns the substrate's word-search matches into a bounded
// ranked list, in the substrate's own order (best first). Nothing is invented:
// the ID, label, and score are the substrate's; the type is the ID's own type
// segment; properties are the matched entity's own triple objects. The label
// falls back to the entity's dc.terms.title only when the substrate sent none.
//
// total is the substrate's count of matches before its limit, so truncated
// says so whenever fewer matches render than exist — an agent must never read
// a partial list as the complete hit set.
func deriveMatches(text *gtypes.TextSearchData) (matches []graphMatch, total int, truncated bool) {
	if text == nil {
		return nil, 0, false
	}
	for _, m := range text.Matches {
		gm := graphMatch{ID: m.ID, Type: entityTypeSegment(m.ID), Label: m.Label, Relevance: m.Score}
		var e substrateEntity
		if len(m.Entity) > 0 && json.Unmarshal(m.Entity, &e) == nil {
			if gm.Label == "" {
				gm.Label = entityTitle(e)
			}
			gm.Properties = entityProperties(e)
		}
		matches = append(matches, gm)
	}
	total = max(text.Matched, len(matches))
	if len(matches) > maxGraphMatches {
		matches = matches[:maxGraphMatches]
	}
	truncated = text.Truncated || len(matches) < total
	return matches, total, truncated
}

// communitySummary is one community group holding matches whose record the
// substrate read: its identifier and level (an identifier resolves only within
// its level), and the record's own values.
type communitySummary struct {
	CommunityID string   `json:"community_id"`
	Level       int      `json:"level"`
	MemberCount int      `json:"member_count"`
	Matches     int      `json:"matches"`
	Summary     string   `json:"summary,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

// deriveCommunities lists the groups whose COMMUNITY_INDEX record was read. A
// group answered not_ready (no clustering) or unreadable carries no record and
// is not community evidence, so it is left out.
func deriveCommunities(text *gtypes.TextSearchData) []communitySummary {
	if text == nil {
		return nil
	}
	var out []communitySummary
	for _, g := range text.Groups {
		if g.RecordState != gtypes.RecordRead || g.Record == nil {
			continue
		}
		out = append(out, communitySummary{
			CommunityID: g.CommunityID, Level: text.Level, MemberCount: g.Record.MemberCount,
			Matches: g.Matches, Summary: g.Record.CommunitySummary, Keywords: g.Record.Keywords,
		})
	}
	return out
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
// its answer, its community records, its entity IDs, its hints — plus the
// derived disclosure, and nothing invented. The match list is bounded;
// TotalMatches and Truncated keep that honest rather than presenting a cap as
// the whole result. Hints say what the question asked for beyond its words
// (a time range, an area, a count) and whether it was applied.
type graphSearchResult struct {
	Retrieval          retrievalDisclosure       `json:"retrieval"`
	Answer             string                    `json:"answer,omitempty"`
	Matches            []graphMatch              `json:"matches"`
	TotalMatches       int                       `json:"total_matches"`
	Truncated          bool                      `json:"truncated"`
	CommunitySummaries []communitySummary        `json:"community_summaries,omitempty"`
	Entity             *graphMatch               `json:"entity,omitempty"`
	Path               *gtypes.PathSearchData    `json:"path,omitempty"`
	Hints              []gtypes.GlobalSearchHint `json:"hints,omitempty"`
}

// Hint kinds whose answer the substrate carries beside the word matches.
const (
	hintEntity = "entity"
	hintPath   = "path"
)

// gatewayUnreadableCode marks a hint the gateway downgraded because it could
// not deliver the substrate's answer for it. The code is the gateway's own, so
// a consumer can tell it from a substrate refusal.
const gatewayUnreadableCode = "mcp_gateway_answer_unreadable"

// deriveHintAnswers carries the answers behind honoured entity and path hints.
// The substrate marks a hint honoured only when its reply holds the answer —
// for a one-word reference or a path question that answer may be the whole
// reply, with no word matches at all — so dropping it would leave an honoured
// claim with nothing behind it. The entity renders as a bounded match (no
// triples); the path is the substrate's own walk, already bounded by its
// depth and node limits and carrying IDs and edges only.
//
// Invariant: no hint leaves here honoured without its answer. A hint whose
// answer is absent or unreadable is reported not honoured, with the gateway's
// own code and reason so it is never mistaken for a substrate refusal.
func deriveHintAnswers(data *gtypes.GlobalSearchData) (*graphMatch, *gtypes.PathSearchData, []gtypes.GlobalSearchHint) {
	var entity *graphMatch
	var e substrateEntity
	if len(data.Entity) > 0 && json.Unmarshal(data.Entity, &e) == nil && e.ID != "" {
		entity = &graphMatch{
			ID: e.ID, Type: entityTypeSegment(e.ID), Label: entityTitle(e), Properties: entityProperties(e),
		}
	}
	hints := make([]gtypes.GlobalSearchHint, len(data.Hints))
	copy(hints, data.Hints)
	for i := range hints {
		h := &hints[i]
		if !h.Honoured {
			continue
		}
		missing := (h.Hint == hintEntity && entity == nil) || (h.Hint == hintPath && data.Path == nil)
		if missing {
			h.Honoured = false
			h.Code = gatewayUnreadableCode
			h.Reason = "the substrate honoured this hint, but the gateway could not read its answer"
		}
	}
	return entity, data.Path, hints
}
