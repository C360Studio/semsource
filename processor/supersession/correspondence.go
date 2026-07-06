package supersession

import (
	"time"

	"github.com/c360studio/semsource/entityid"
	semsourceast "github.com/c360studio/semsource/source/ast"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// candidate is the version-independent projection of a versioned code entity
// used for correspondence and ordering. It is derived purely from the entity's
// triples plus its ID (org is the intrinsic leading ID segment); nothing here
// parses the folded `system` segment.
type candidate struct {
	id      string
	org     string // entity ID segment[0] — intrinsic, never folded
	project string // code.artifact.project (version-independent source identity)
	version string // code.artifact.version (registered version/ref qualifier)

	// Version-independent identity facts the correspondence key is built from.
	path  string // code.artifact.path
	name  string // dc.terms.title
	ctype string // code.artifact.type
	pkg   string // code.artifact.package

	// bodyHash is the verbatim-body content hash and bodyHashKind names the
	// predicate it came from ("bodykey" for code.body.key's "code:<sha>", "hash"
	// for code.artifact.hash's bare digest, "" when absent). The kind is tracked
	// so change classification never compares hashes from different predicates —
	// their encodings differ, so a literal compare would fabricate a "changed".
	bodyHash     string
	bodyHashKind string

	// bodyStore / bodyKey address the offloaded verbatim body (code.body.store /
	// code.body.key) for hydration by the version-diff query. Both empty when the
	// entity has no offloaded body (its body is unavailable, not an error).
	bodyStore string
	bodyKey   string

	// indexedAt is the first-index timestamp (dc.terms.created), used as the
	// version-ordering tiebreak for non-semver versions (design D4).
	indexedAt time.Time
}

// corrKey is the correspondence grouping key. It scopes to a single source via
// (org, project) — so identical path/name across different sources never
// correspond (OQ3) — then keys the same logical symbol across versions via the
// version-independent identity facts.
type corrKey struct {
	org, project, path, name, ctype, pkg string
}

func (c candidate) key() corrKey {
	return corrKey{
		org:     c.org,
		project: c.project,
		path:    c.path,
		name:    c.name,
		ctype:   c.ctype,
		pkg:     c.pkg,
	}
}

// candidateFromEntity projects an enumerated entity into a candidate. It returns
// (_, false) for entities that are not versioned code entities — those carrying
// no code.artifact.version triple cannot participate in cross-version
// correspondence and are skipped.
func candidateFromEntity(e gtypes.EntityState) (candidate, bool) {
	version := tripleString(e.Triples, semsourceast.CodeVersion)
	if version == "" {
		return candidate{}, false
	}
	hash, kind := bodyHashOf(e.Triples)
	c := candidate{
		id:           e.ID,
		org:          entityid.OrgFromID(e.ID),
		project:      tripleString(e.Triples, semsourceast.CodeProject),
		version:      version,
		path:         tripleString(e.Triples, semsourceast.CodePath),
		name:         tripleString(e.Triples, semsourceast.DcTitle),
		ctype:        tripleString(e.Triples, semsourceast.CodeType),
		pkg:          tripleString(e.Triples, semsourceast.CodePackage),
		bodyHash:     hash,
		bodyHashKind: kind,
		bodyStore:    tripleString(e.Triples, semsourceast.CodeBodyStore),
		bodyKey:      tripleString(e.Triples, semsourceast.CodeBodyKey),
		indexedAt:    indexedAtOf(e),
	}
	return c, true
}

// groupByCorrespondence buckets candidates by their correspondence key. Each
// bucket holds the same logical symbol across versions of one source. Grouping
// is hash-based (linear), not pairwise (design D3).
func groupByCorrespondence(cands []candidate) map[corrKey][]candidate {
	groups := make(map[corrKey][]candidate)
	for _, c := range cands {
		k := c.key()
		groups[k] = append(groups[k], c)
	}
	return groups
}

// bodyHashOf returns the verbatim-body content hash and the predicate it came
// from ("bodykey" or "hash"), preferring code.body.key (the offloaded body's
// "code:<sha>") over code.artifact.hash (a bare digest). Returns ("", "") when
// the entity carries neither. The kind lets change classification refuse to
// compare hashes from different predicates, whose encodings are not comparable.
func bodyHashOf(triples []message.Triple) (hash, kind string) {
	if h := tripleString(triples, semsourceast.CodeBodyKey); h != "" {
		return h, "bodykey"
	}
	if h := tripleString(triples, semsourceast.CodeHash); h != "" {
		return h, "hash"
	}
	return "", ""
}

// indexedAtOf reads the first-index timestamp from dc.terms.created, falling
// back to the entity's UpdatedAt when the triple is missing or unparseable.
func indexedAtOf(e gtypes.EntityState) time.Time {
	if s := tripleString(e.Triples, semsourceast.DcCreated); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return e.UpdatedAt
}

// tripleString returns the first triple's object as a string for the given
// predicate, or "" when absent or non-string.
func tripleString(triples []message.Triple, predicate string) string {
	for i := range triples {
		if triples[i].Predicate == predicate {
			if s, ok := triples[i].Object.(string); ok {
				return s
			}
			return ""
		}
	}
	return ""
}
