// Package sourceallow enforces the filesystem-root allowlist for path-based
// source registration arriving over an external boundary (the HTTP façade and
// the MCP gateway — ADR-0007 §3). URL-only sources are unaffected; the in-mesh
// NATS path and config-file sources are operator-trusted and do not go through
// this guard.
package sourceallow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/config"
)

// Enforce rejects a path-based source whose path is not under an allowlisted
// root. An empty allowlist refuses all path-based registrations (no arbitrary
// host paths). filepath.Clean collapses any "../" traversal, so a prefix check
// against the cleaned absolute path is a sound containment guard.
func Enforce(src config.SourceEntry, roots []string) error {
	paths := sourcePaths(src)
	if len(paths) == 0 {
		return nil // url-only source — allowlist not applicable
	}
	if len(roots) == 0 {
		return errors.New("path-based source registration requires configured source_roots (none set)")
	}
	absRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("invalid source_root %q: %w", root, err)
		}
		absRoots = append(absRoots, filepath.Clean(abs))
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("invalid path %q: %w", p, err)
		}
		if !underAnyRoot(filepath.Clean(abs), absRoots) {
			return fmt.Errorf("path %q is outside the allowlisted source_roots", p)
		}
	}
	return nil
}

func sourcePaths(src config.SourceEntry) []string {
	out := make([]string, 0, 1+len(src.Paths))
	if src.Path != "" {
		out = append(out, src.Path)
	}
	out = append(out, src.Paths...)
	return out
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
