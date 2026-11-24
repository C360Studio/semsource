package astsource

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

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
// hold an ancestor of changed. Reconcile applies only to a target, never on a
// shared system (#349), and never to an ID two live folders build (their
// publishes alternate, so neither is the complete set). An ancestor left with
// no indexed file is published with no contains so its stale edges clear,
// whether or not its directory still exists (the lifecycle pass never judges
// containers: they carry no owner fact), unless a live folder holds its ID.
func planContainers(files map[string]string, changed []string, all, reconcile, shared bool,
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
		if !all && !target {
			continue
		}
		out = append(out, containerPublish{e, shared || !reconcile || !target || builders[e.ID] > 1})
	}
	var emptied []*semsourceast.CodeEntity
	for id, e := range targets {
		if e != nil && builders[id] == 0 {
			emptied = append(emptied, e)
		}
	}
	slices.SortFunc(emptied, func(x, y *semsourceast.CodeEntity) int { return strings.Compare(x.Path, y.Path) })
	for _, e := range emptied {
		out = append(out, containerPublish{e, shared || !reconcile})
	}
	return out
}

// publishContainers publishes planContainers' plan for the watch path. The
// changed paths of a call whose publish was refused are kept and joined to
// the next call, so a refused emptied-folder publish is retried even by an
// unchanged full pass. The caller holds filesMu.
func (c *Component) publishContainers(ctx context.Context, pw *pathWatcher, changed []string, all, reconcile bool) {
	changed = append(changed, pw.pendingContainers...)
	pw.pendingContainers = nil
	failed := false
	for _, p := range planContainers(pw.files, changed, all, reconcile, pw.sharedSystem, c.authority, pw.scopedSystem) {
		if !c.publishContainer(ctx, p.entity, p.skipReconcile) {
			failed = true
		}
	}
	if failed {
		slices.Sort(changed)
		pw.pendingContainers = slices.Compact(changed)
	}
}

// publishContainer publishes one repo or folder entity and reports whether
// the publisher admitted it.
func (c *Component) publishContainer(ctx context.Context, entity *semsourceast.CodeEntity, skipReconcile bool) bool {
	state := entity.EntityState()
	payload, err := payloadFromASTState(state, nil)
	if err != nil {
		c.logger.Warn("Invalid hierarchy entity state", "id", state.ID, "error", err)
		return true // retrying cannot make it valid
	}
	payload.SkipReconcile = skipReconcile
	if err := c.publishEntity(ctx, payload); err != nil {
		c.logger.Warn("Failed to publish hierarchy entity; retried on the next container publish",
			"id", entity.ID, "error", err)
		return false
	}
	c.entitiesIndexed.Add(1)
	c.distinct.Observe(state.ID)
	return true
}

// watchContainers applies one watch event's change to the file set and, when
// it changed, republishes the repo and the changed path's ancestors. A root
// sharing its system with another (#349) only updates the set: its
// containers hold the other root's children too, which its set cannot see,
// so a watch publish would drop them; its full passes still publish.
func (c *Component) watchContainers(ctx context.Context, pw *pathWatcher, change func() []string) {
	pw.filesMu.Lock()
	defer pw.filesMu.Unlock()
	if changed := change(); len(changed) > 0 && !pw.sharedSystem {
		c.publishContainers(ctx, pw, changed, false, true)
	}
}
