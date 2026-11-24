package astsource

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// The file set is what folder and repo entities are built from (#347): every
// root-relative path the watch path has indexed, with its file entity ID. A
// folder is always published from the whole set, never from one file, so its
// code.structure.contains set is complete.
//
// shortcut: a file created after a reindex walk passed its directory but
// before replaceFiles drops out of the set until its next event or the next
// pass (its file entity is still published); upgrade to a merge keyed on
// event time if a folder edge is seen missing between passes.

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

// addFile records one indexed file and reports whether the set changed. The
// caller holds filesMu.
func (pw *pathWatcher) addFile(path, id string) bool {
	if old, ok := pw.files[path]; ok && old == id {
		return false
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

// publishContainers publishes the repo and folder entities built from the
// whole file set. With all, every container is published (a full pass);
// otherwise only the repo and the ancestors of changed. Reconcile applies
// only to the repo and the ancestors of changed, and never on a shared system
// (#349). An ancestor left with no indexed file whose directory still exists
// is published with no contains, so its stale edges clear; a removed
// directory is the lifecycle pass's. The caller holds filesMu.
func (c *Component) publishContainers(ctx context.Context, pw *pathWatcher, changed []string, all, reconcile bool) {
	targets := make(map[string]bool)
	if len(changed) > 0 {
		targets[""] = true // the repo entity's Path
	}
	for _, p := range changed {
		for d := filepath.Dir(p); d != "." && d != "" && d != "/"; d = filepath.Dir(d) {
			targets[d] = true
		}
	}
	for _, e := range semsourceast.BuildHierarchyFromFiles(pw.files, c.authority, pw.scopedSystem) {
		target := targets[e.Path]
		if !all && !target {
			continue
		}
		delete(targets, e.Path)
		c.publishContainer(ctx, e, pw.sharedSystem || !reconcile || !target)
	}
	for dir := range targets {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(pw.root, dir)); err != nil || !info.IsDir() {
			continue
		}
		c.publishContainer(ctx, semsourceast.EmptyFolder(dir, c.authority, pw.scopedSystem), pw.sharedSystem || !reconcile)
	}
}

// publishContainer publishes one repo or folder entity.
func (c *Component) publishContainer(ctx context.Context, entity *semsourceast.CodeEntity, skipReconcile bool) {
	state := entity.EntityState()
	payload, err := payloadFromASTState(state, nil)
	if err != nil {
		c.logger.Warn("Invalid hierarchy entity state", "id", state.ID, "error", err)
		return
	}
	payload.SkipReconcile = skipReconcile
	if err := c.publishEntity(ctx, payload); err != nil {
		c.logger.Warn("Failed to publish hierarchy entity", "id", entity.ID, "error", err)
		return
	}
	c.entitiesIndexed.Add(1)
	c.distinct.Observe(state.ID)
}
