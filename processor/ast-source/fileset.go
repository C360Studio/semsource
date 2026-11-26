package astsource

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semsource/entityid"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// The file set is what folder and repo entities are built from (#347): every
// root-relative path the watch path has indexed, with its file entity ID. A
// folder is always published from the whole set, never from one file, so its
// code.structure.contains set is complete.
//
// shortcut: a reindex replaces the set with what its walk saw, so a watch
// event landing during the walk is undone until the next event or pass: a
// file created after the walk passed its directory loses its folder edge
// (reconciled away), and a file deleted after the walk parsed it regains one
// (reconciled back, to a file the lifecycle pass then marks stale). Upgrade
// to a merge keyed on event time if either is seen between passes.

// replaceFiles makes the set exactly the results' file entities and returns
// the paths added or removed. The caller holds filesMu.
func (pw *pathWatcher) replaceFiles(results []*semsourceast.ParseResult) []string {
	next := make(map[string]string, len(results))
	for _, r := range results {
		if r != nil && r.FileEntity != nil {
			next[r.Path] = r.FileEntity.ID
		}
	}
	var changed []string
	for p, id := range next {
		if old, ok := pw.files[p]; !ok || old != id {
			changed = append(changed, p)
		}
	}
	for p := range pw.files {
		if _, ok := next[p]; !ok {
			changed = append(changed, p)
		}
	}
	pw.files = next
	return changed
}

// addFile records one indexed file and reports whether the set changed. A
// file whose entity ID another indexed path already holds (a fold the last
// full pass did not see) is not added, so no container lists the ID twice.
// The caller holds filesMu.
func (pw *pathWatcher) addFile(path, id string) bool {
	if old, ok := pw.files[path]; ok && old == id {
		return false
	}
	for p, other := range pw.files {
		if other == id && p != path {
			return false
		}
	}
	if pw.files == nil {
		pw.files = make(map[string]string)
	}
	pw.files[path] = id
	return true
}

// removeFiles drops path and, for a deleted directory, every path under it,
// and returns what it removed. The caller holds filesMu.
func (pw *pathWatcher) removeFiles(path string) []string {
	var removed []string
	prefix := path + "/"
	for p := range pw.files {
		if p == path || strings.HasPrefix(p, prefix) {
			removed = append(removed, p)
			delete(pw.files, p)
		}
	}
	return removed
}

// containerPublish is one planned repo or folder publish.
type containerPublish struct {
	entity        *semsourceast.CodeEntity
	skipReconcile bool
}

// planContainers plans the repo and folder publishes built from the whole
// file set. Targets are keyed by entity ID, not path: sanitizing folds
// folders like a-b and a/b onto one ID. With all, every container is
// published (a full pass); otherwise only the repo and the containers that
// hold an ancestor of changed. Reconcile applies only to a target, and never
// to an ID two live folders build (their publishes alternate, so neither is
// the complete set). An ancestor left with no indexed file is published with
// no contains so its stale edges clear, whether or not its directory still
// exists (the lifecycle pass never judges containers: they carry no owner
// fact), unless a live folder holds its ID.
func planContainers(files map[string]string, changed []string, all, reconcile bool,
	authority entityid.Authority, system string,
) []containerPublish {
	built := semsourceast.BuildHierarchyFromFiles(files, authority, system)
	targets := make(map[string]*semsourceast.CodeEntity) // ID → emptied form
	if len(changed) > 0 {
		targets[built[0].ID] = nil // the repo
	}
	for _, p := range changed {
		for d := filepath.Dir(p); d != "." && d != "" && d != "/"; d = filepath.Dir(d) {
			if e := semsourceast.EmptyFolder(d, authority, system); targets[e.ID] == nil {
				targets[e.ID] = e
			}
		}
	}
	builders := make(map[string]int, len(built))
	for _, e := range built {
		builders[e.ID]++
	}
	var out []containerPublish
	for _, e := range built {
		_, target := targets[e.ID]
		if !all && (!target || builders[e.ID] > 1) {
			// A watch event never writes an ID two live folders build: each
			// carries only its own path's children, so either would replace
			// the other's. Full passes publish both, as before this fix.
			continue
		}
		out = append(out, containerPublish{e, !reconcile || !target || builders[e.ID] > 1})
	}
	var emptied []*semsourceast.CodeEntity
	for id, e := range targets {
		if e != nil && builders[id] == 0 {
			emptied = append(emptied, e)
		}
	}
	slices.SortFunc(emptied, func(x, y *semsourceast.CodeEntity) int { return strings.Compare(x.Path, y.Path) })
	for _, e := range emptied {
		out = append(out, containerPublish{e, !reconcile})
	}
	return out
}

// publishContainers publishes planContainers' plan for the watch path. The
// changed paths of a call whose publish was refused are kept and joined to
// the next call (a watch create or delete, or a full pass, even an
// unchanged one), so a refused emptied-folder publish is retried.
//
// shortcut: a watch modify of an indexed path publishes no container, so it
// does not retry; with index_interval off, a refused publish waits for the
// next create or delete. Upgrade to a retry timer if refusals are seen.
// With skipUnchanged (the periodic reindex only), a container whose
// statements match what the publisher last admitted from this process is
// not sent again when it would only merge (#401). The caller holds filesMu.
func (c *Component) publishContainers(ctx context.Context, pw *pathWatcher, changed []string, all, reconcile, skipUnchanged bool) {
	changed = append(changed, pw.pendingContainers...)
	pw.pendingContainers = nil
	failed := false
	for _, p := range planContainers(pw.files, changed, all, reconcile, c.authority, pw.scopedSystem) {
		if !c.publishContainer(ctx, p.entity, p.skipReconcile, skipUnchanged) {
			failed = true
		}
	}
	if failed {
		slices.Sort(changed)
		pw.pendingContainers = slices.Compact(changed)
	}
}

// publishContainer publishes one repo or folder entity and reports whether
// the publisher admitted it. With skipUnchanged, a merge-only publish whose
// statements equal the last admitted ones counts as admitted without a send.
func (c *Component) publishContainer(ctx context.Context, entity *semsourceast.CodeEntity, skipReconcile, skipUnchanged bool) bool {
	state := entity.EntityState()
	payload, err := payloadFromASTState(state, nil)
	if err != nil {
		c.logger.Warn("Invalid hierarchy entity state", "id", state.ID, "error", err)
		return true // retrying cannot make it valid
	}
	sum := containerFingerprint(state.Triples)
	// Only a merge is skipped: a reconcile target (the repo and ancestors
	// of a changed path) always sends, because its reconcile clears edges a
	// merge left behind (the seed's, or a dropped reconcile's) even when
	// its statements are unchanged.
	if skipUnchanged && skipReconcile && c.sentContainers.same(state.ID, sum) {
		return true
	}
	payload.SkipReconcile = skipReconcile
	if err := c.publishEntity(ctx, payload); err != nil {
		c.logger.Warn("Failed to publish hierarchy entity; retried on the next container publish",
			"id", entity.ID, "error", err)
		return false
	}
	c.sentContainers.record(state.ID, sum)
	c.entitiesIndexed.Add(1)
	c.distinct.Observe(state.ID)
	return true
}

// watchContainers applies one watch event's change to the file set and, when
// it changed, republishes the repo and the changed path's ancestors.
func (c *Component) watchContainers(ctx context.Context, pw *pathWatcher, change func() []string) {
	pw.filesMu.Lock()
	defer pw.filesMu.Unlock()
	if changed := change(); len(changed) > 0 {
		c.publishContainers(ctx, pw, changed, false, true, false)
	}
}

// containerSums records, per container ID, a fingerprint of the statements
// the publisher last admitted from this process (#401). Admitted is the bar,
// as for the file hash gate: a publish lost downstream after admission is not
// resent until the container changes or the process restarts.
//
// shortcut: a third copy of cfgfile-source's and url-source's sentContent
// fingerprint; extract a shared package when a fourth source needs it.
type containerSums struct {
	mu   sync.Mutex
	sums map[string][sha256.Size]byte
}

func (s *containerSums) same(id string, sum [sha256.Size]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.sums[id]
	return ok && prev == sum
}

func (s *containerSums) record(id string, sum [sha256.Size]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sums == nil {
		s.sums = make(map[string][sha256.Size]byte)
	}
	s.sums[id] = sum
}

// containerFingerprint hashes a container's statements order-independently,
// leaving out dc.terms.created, which is the wall clock of every build.
func containerFingerprint(triples []message.Triple) [sha256.Size]byte {
	lines := make([]string, 0, len(triples))
	for _, t := range triples {
		if t.Predicate == semsourceast.DcCreated {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\x00%s\x00%v\x00%s", t.Subject, t.Predicate, t.Object, t.Datatype))
	}
	sort.Strings(lines)
	return sha256.Sum256([]byte(strings.Join(lines, "\n")))
}
