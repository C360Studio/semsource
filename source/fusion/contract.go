// Package fusion is the deterministic fusion engine: it resolves a query against
// the existing graph (graph.query.*), expands the structural neighborhood,
// hydrates verbatim bodies, and assembles one fused, ready-to-act response with
// entity IDs demoted to opaque handles. It is the deterministic counterpart to
// semstreams research_graph — same resolve→expand→assemble shape, no LLM. A Lens
// (lens.go) supplies the only domain-specific parts (which edges, how to hydrate,
// how to read fields), so the same engine serves code, docs, and other domains.
//
// Ranking is ontology-aware (BFO/CCO via source/ontology), not lexical. See
// docs/adr/0004 (fusion gateway) and 0005 (ontology alignment).
package fusion

// ContractVersion identifies the wire shape of Request/Response.
const ContractVersion = "1"

// Want enumerates the optional facets a caller can request. Empty defaults to
// body plus immediate relations.
type Want string

// The requestable facets.
const (
	WantBody      Want = "body"      // verbatim source/passage
	WantRelations Want = "relations" // callers/callees, links/sections
	WantPaths     Want = "paths"     // bounded relation paths from the seed
	WantImpact    Want = "impact"    // transitive reverse-relation closure
)

// Budget bounds a response. Zero fields take engine defaults.
type Budget struct {
	MaxNodes int `json:"max_nodes,omitempty"`
	MaxBytes int `json:"max_bytes,omitempty"`
}

// Request is the fused query, keyed by what an agent already knows — never an
// entity ID.
type Request struct {
	Query  string `json:"query"`
	Repo   string `json:"repo,omitempty"`
	Want   []Want `json:"want,omitempty"`
	Budget Budget `json:"budget,omitzero"`
}

// Provenance records how an answer was produced so callers can calibrate trust.
type Provenance string

// The provenance tiers, in increasing order of uncertainty.
const (
	ProvenanceDeterministic Provenance = "deterministic" // exact lookup + structural walk
	ProvenanceEmbedding     Provenance = "embedding"     // seeds came from semantic search
	ProvenanceLLM           Provenance = "llm"           // an LLM reasoned over the result
)

// IndexState mirrors the graph readiness phase.
type IndexState string

// The readiness phases. Only Ready permits a not-found conclusion.
const (
	StateBuilding IndexState = "building"
	StateReady    IndexState = "ready"
	StateDegraded IndexState = "degraded"
)

// IndexStatus is attached to every response. Ready is load-bearing: when false
// the caller must fall back (e.g. to grep) rather than treat empty as not-found.
type IndexStatus struct {
	Ready      bool       `json:"ready"`
	State      IndexState `json:"state"`
	Revision   string     `json:"revision,omitempty"`
	LastSynced string     `json:"last_synced,omitempty"`
}

// Locator is a domain-general place: a file path or a URL, an optional section/
// anchor fragment (docs), and an optional line range (code). One of Lines or
// Fragment is typically set, not both.
type Locator struct {
	Path     string `json:"path,omitempty"`     // file path or URL
	Fragment string `json:"fragment,omitempty"` // section / anchor (docs)
	Lines    [2]int `json:"lines,omitempty"`    // line range (code)
}

// Ref points to a node by what a human reads — never the entity ID.
type Ref struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Fragment string `json:"fragment,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// Node is one fused result: verbatim body plus the structure around it. Domains
// differ only in which roles populate Relations (code: callers/callees; docs:
// links/sections).
type Node struct {
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"`
	Path     string `json:"path,omitempty"`
	Fragment string `json:"fragment,omitempty"`
	// Lines is [start,end] for code, nil for line-less domains (docs) — a slice
	// so omitempty actually omits it rather than emitting a spurious [0,0].
	Lines     []int            `json:"lines,omitempty"`
	Body      string           `json:"body,omitempty"`
	Relations map[string][]Ref `json:"relations,omitempty"`
	// Class is the BFO/CCO class IRI (provenance/debug; the agent ignores it).
	Class string `json:"class,omitempty"`
	// Handle is an opaque continuation token (internally the entity ID). Not an
	// addressing scheme: never parse or construct it.
	Handle string `json:"handle,omitempty"`
}

// Miss reports a query that resolved to nothing while the graph was ready, with
// near-matches. A Miss only appears when Ready is true.
type Miss struct {
	Query      string   `json:"query"`
	DidYouMean []string `json:"did_you_mean,omitempty"`
}

// Impact summarizes the transitive reverse-relation closure of the seeds.
// Truncated is true when the closure hit the engine's node cap, so Files/Nodes
// are a floor, not an exact count.
type Impact struct {
	Files     int    `json:"files"`
	Nodes     int    `json:"nodes"`
	Truncated bool   `json:"truncated,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// Response is the fused answer. Nodes is the payload; Paths/Impact are facets;
// Index and Provenance are the honesty envelope.
type Response struct {
	Index           IndexStatus `json:"index"`
	Provenance      Provenance  `json:"provenance"`
	Nodes           []Node      `json:"nodes,omitempty"`
	Paths           [][]string  `json:"paths,omitempty"`
	Impact          *Impact     `json:"impact,omitempty"`
	Misses          []Miss      `json:"misses,omitempty"`
	Truncated       bool        `json:"truncated"`
	ContractVersion string      `json:"contract_version"`
}

const (
	defaultMaxNodes = 20
	defaultMaxBytes = 60000
)

// wantSet expands a Want slice into a lookup set, applying defaults when empty.
func wantSet(wants []Want) map[Want]bool {
	if len(wants) == 0 {
		return map[Want]bool{WantBody: true, WantRelations: true}
	}
	set := make(map[Want]bool, len(wants))
	for _, w := range wants {
		set[w] = true
	}
	return set
}
