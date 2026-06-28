package code

import (
	"os"
	"path/filepath"
	"strings"
)

// sourceReader reads verbatim line ranges from worktree files. It holds no cache:
// source files are small and os.ReadFile is cheap relative to the graph
// round-trips a fusion already makes, and a cache would risk serving stale
// content if a lens outlived a single request. Each read reflects the current
// file, matching the line numbers the graph keeps fresh via ast-source's watcher.
type sourceReader struct {
	root string
}

func newSourceReader(root string) *sourceReader {
	return &sourceReader{root: root}
}

// extract returns the verbatim source for the inclusive 1-based range
// [start, end] of relPath, or "" if unreadable or the range is invalid.
func (r *sourceReader) extract(relPath string, start, end int) string {
	if start <= 0 || end < start {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(r.root, relPath))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}
