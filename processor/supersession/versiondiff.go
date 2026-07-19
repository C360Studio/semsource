package supersession

import (
	"sort"
)

// Change statuses for a version diff. indeterminate means both versions have the
// symbol but a body hash is absent or the two hashes are incomparable (different
// predicates) — it is surfaced, never guessed as changed/unchanged.
const (
	statusAdded         = "added"
	statusRemoved       = "removed"
	statusChanged       = "changed"
	statusUnchanged     = "unchanged"
	statusIndeterminate = "indeterminate"
)

// Diff response bounds (request may lower them; 0/negative means the default).
const (
	// defaultMaxDiffSymbols caps the number of listed change entries.
	defaultMaxDiffSymbols = 2000
	// defaultMaxBodyBytes caps the cumulative verbatim-body bytes hydrated across
	// the whole response.
	defaultMaxBodyBytes = 512 * 1024
)

// VersionDiffRequest asks "what changed between from and to of one project".
type VersionDiffRequest struct {
	Project string `json:"project"`
	From    string `json:"from"`
	To      string `json:"to"`
	// WantBodies controls verbatim before/after body hydration (default true).
	WantBodies *bool `json:"want_bodies,omitempty"`
	// MaxSymbols / MaxBodyBytes optionally lower the response bounds.
	MaxSymbols   int `json:"max_symbols,omitempty"`
	MaxBodyBytes int `json:"max_body_bytes,omitempty"`
}

func (r VersionDiffRequest) wantBodies() bool { return r.WantBodies == nil || *r.WantBodies }

func (r VersionDiffRequest) maxSymbols() int {
	if r.MaxSymbols <= 0 {
		return defaultMaxDiffSymbols
	}
	return r.MaxSymbols
}

func (r VersionDiffRequest) maxBodyBytes() int {
	if r.MaxBodyBytes <= 0 {
		return defaultMaxBodyBytes
	}
	return r.MaxBodyBytes
}

// Change is one symbol's classification across the two versions. Bodies are
// present only for hydrated entries (changed/added/removed with an offloaded body
// and within budget).
type Change struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Package  string `json:"package,omitempty"`
	Status   string `json:"status"`
	FromID   string `json:"from_id,omitempty"`
	ToID     string `json:"to_id,omitempty"`
	FromBody string `json:"from_body,omitempty"`
	ToBody   string `json:"to_body,omitempty"`
	// FromBodyError/ToBodyError mark a body that EXISTS (an offloaded handle
	// is stamped) but could not be resolved due to a storage failure —
	// distinct from "no body was ever offloaded" (both fields absent) and
	// from a budget skip (counted in OmittedBodies). A consumer can tell an
	// incomplete answer from a complete one (version-registration-surface D3).
	FromBodyError bool `json:"from_body_error,omitempty"`
	ToBodyError   bool `json:"to_body_error,omitempty"`
}

// DiffCounts tallies every corresponding symbol by status (including the
// unchanged ones omitted from the Changes list).
type DiffCounts struct {
	Added         int `json:"added"`
	Removed       int `json:"removed"`
	Changed       int `json:"changed"`
	Unchanged     int `json:"unchanged"`
	Indeterminate int `json:"indeterminate"`
}

// VersionDiffResponse is the changeset over the two version scopes.
type VersionDiffResponse struct {
	Project string     `json:"project"`
	From    string     `json:"from"`
	To      string     `json:"to"`
	Ready   bool       `json:"ready"`
	Note    string     `json:"note,omitempty"`
	Counts  DiffCounts `json:"counts"`
	Changes []Change   `json:"changes"`
	// Truncated is set when the change list was capped at max_symbols;
	// DroppedSymbols is how many entries were omitted. OmittedBodies counts
	// entries whose body was skipped because the byte budget was exhausted.
	Truncated      bool `json:"truncated,omitempty"`
	DroppedSymbols int  `json:"dropped_symbols,omitempty"`
	OmittedBodies  int  `json:"omitted_bodies,omitempty"`
	// FailedBodies counts bodies that resolve-failed (storage error), each
	// also marked on its Change via From/ToBodyError.
	FailedBodies int `json:"failed_bodies,omitempty"`
}

// diffCandidates corresponds candidates of one project across the from/to
// versions and classifies each symbol. It returns the Changes (unchanged omitted,
// each classified but without bodies) and the full counts. Deterministic: output
// is sorted by (path, package, name, status). Bodies are hydrated separately.
//
// It also returns, for each listed change, the from/to candidates so the caller
// can hydrate bodies without re-projecting.
func diffCandidates(cands []candidate, project, from, to string) ([]Change, DiffCounts, map[string]sidePair) {
	type sideAcc struct{ from, to *candidate }
	buckets := map[corrKey]*sideAcc{}
	for i := range cands {
		c := cands[i]
		if c.project != project {
			continue
		}
		if c.version != from && c.version != to {
			continue
		}
		acc := buckets[c.key()]
		if acc == nil {
			acc = &sideAcc{}
			buckets[c.key()] = acc
		}
		cp := c
		if c.version == from {
			acc.from = newestOf(acc.from, &cp)
		}
		// A version string equal to both from and to (from == to) lands on the
		// "to" side too; guard by only setting to when it isn't already the from
		// pick for the identical-version degenerate case.
		if c.version == to {
			acc.to = newestOf(acc.to, &cp)
		}
	}

	var changes []Change
	var counts DiffCounts
	pairs := map[string]sidePair{}
	for key, acc := range buckets {
		switch {
		case acc.from == nil && acc.to != nil:
			counts.Added++
			ch := changeFrom(key, statusAdded)
			ch.ToID = acc.to.id
			changes = append(changes, ch)
			pairs[changeID(ch)] = sidePair{to: acc.to}
		case acc.from != nil && acc.to == nil:
			counts.Removed++
			ch := changeFrom(key, statusRemoved)
			ch.FromID = acc.from.id
			changes = append(changes, ch)
			pairs[changeID(ch)] = sidePair{from: acc.from}
		default:
			status := classifyDiff(*acc.from, *acc.to)
			switch status {
			case statusChanged:
				counts.Changed++
			case statusUnchanged:
				counts.Unchanged++
			default:
				counts.Indeterminate++
			}
			if status == statusUnchanged {
				continue // bulk; counted but not listed
			}
			ch := changeFrom(key, status)
			ch.FromID = acc.from.id
			ch.ToID = acc.to.id
			changes = append(changes, ch)
			pairs[changeID(ch)] = sidePair{from: acc.from, to: acc.to}
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Status < b.Status
	})
	return changes, counts, pairs
}

// sidePair carries the corresponding candidates for a listed change (either side
// may be nil for added/removed).
type sidePair struct{ from, to *candidate }

// changeID is the stable identity of a change entry for the pairs map (from+to id
// uniquely identify it within one diff).
func changeID(c Change) string { return c.FromID + "\x00" + c.ToID }

// newestOf returns the candidate with the later indexedAt (prefers b on ties so a
// same-timestamp duplicate is still deterministic by later insertion).
func newestOf(a, b *candidate) *candidate {
	if a == nil {
		return b
	}
	if b.indexedAt.Before(a.indexedAt) {
		return a
	}
	return b
}

// changeFrom builds a Change from a correspondence key (the version-independent
// identity all sides share) and a status.
func changeFrom(key corrKey, status string) Change {
	return Change{
		Name:    key.name,
		Path:    key.path,
		Type:    key.ctype,
		Package: key.pkg,
		Status:  status,
	}
}

// classifyDiff maps a corresponding from/to pair to a diff status, reusing the
// body-hash comparison (which refuses incomparable hashes → indeterminate).
func classifyDiff(from, to candidate) string {
	res, ok := classifyChange(from, to)
	if !ok {
		return statusIndeterminate
	}
	if res == changeChanged {
		return statusChanged
	}
	return statusUnchanged
}
