// Package gitboundary detects nested git working-tree boundaries during
// filesystem walks. A directory that is itself a git checkout — a submodule
// (whose .git is a gitlink file) or an unrelated nested repo — is a different
// source: walking into it would attribute its files to the enclosing watch
// path's identity. Boundary detection at walk time is what keeps that
// exclusion race-free: it needs no configuration to arrive first.
package gitboundary

import (
	"os"
	"path/filepath"
)

// IsBoundary reports whether dir is the root of its own git working tree
// (contains a .git entry — file or directory). The root a walk starts from
// is usually a repo itself, so callers must exempt the walk root rather than
// call this on it.
func IsBoundary(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// Under reports whether path lies inside a nested git working tree strictly
// below root. Watchers use this at event-emission time: a submodule tree that
// materializes while watching gains its .git gitlink only after its content
// is written, so a create-time check on the directory alone can race — by
// flush time the boundary marker exists and this check catches it.
func Under(root, path string) bool {
	dir := filepath.Dir(path)
	for len(dir) > len(root) {
		if IsBoundary(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}
