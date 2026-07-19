package ast

import (
	"path/filepath"
	"slices"
	"strings"
)

// IsTestPath reports whether a repo-relative source path is test code, per
// language convention. Used to stamp the code.artifact.test demotion marker
// (search-ranking-and-reach D2): production symbols outrank their tests in NL
// retrieval while tests stay indexed and structurally queryable.
//
// Detection is deliberately conservative — a miss means "no demotion", never a
// wrong boost:
//   - Go:        *_test.go
//   - TS/JS/Svelte: ".test." / ".spec." filename infixes, __tests__/ dirs
//   - Python:    test_*.py, *_test.py, tests/ dirs
//   - Java:      src/test/ trees, *Test.java
func IsTestPath(path string) bool {
	slash := filepath.ToSlash(path)
	base := filepath.Base(slash)

	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	if strings.HasSuffix(base, ".py") &&
		(strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) {
		return true
	}
	if strings.HasSuffix(base, "Test.java") {
		return true
	}
	if slices.Contains(strings.Split(slash, "/"), "__tests__") {
		return true
	}
	if strings.Contains(slash, "src/test/") && strings.HasSuffix(base, ".java") {
		return true
	}
	if strings.HasSuffix(base, ".py") && slices.Contains(strings.Split(slash, "/"), "tests") {
		return true
	}
	return false
}
