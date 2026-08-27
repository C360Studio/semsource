// Package buildpins holds agreement tests for version pins that must match
// across files the build depends on.
//
// Some pins have a single home and need no test: the Go toolchain CI uses comes
// from go.mod via setup-go's go-version-file, the revive version lives once in
// Taskfile.yml's REVIVE_VERSION, and the Task runner version lives once in
// .github/actions/setup-task. Those are unified by construction.
//
// A pin lands here when unifying it would cost more than checking it. The
// builder image is the case: making the Dockerfile read go.mod would mean a
// build ARG plus CI wiring to populate it, which is more machinery than a test
// that reads both files and compares.
package buildpins

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// repoFile reads a file relative to the repository root. Tests run with the
// package directory as the working directory, so the root is two levels up.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

var (
	goDirectiveRe = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)\s*$`)
	golangImageRe = regexp.MustCompile(`(?m)^FROM golang:(\S+?)-alpine`)
)

// TestDockerfileGoVersionMatchesGoMod pins the builder image to the toolchain
// the module declares.
//
// A drift here is quiet and awkward: CI compiles with go.mod's version through
// setup-go and goes green, while the released image is built by a different
// compiler. Nothing fails until something depends on a version-specific
// behavior, and by then the binary in the registry is the one that disagrees.
func TestDockerfileGoVersionMatchesGoMod(t *testing.T) {
	gomod := goDirectiveRe.FindStringSubmatch(repoFile(t, "go.mod"))
	if gomod == nil {
		t.Fatal("go.mod: no `go <version>` directive found")
	}
	dockerfile := golangImageRe.FindStringSubmatch(repoFile(t, "Dockerfile"))
	if dockerfile == nil {
		t.Fatal("Dockerfile: no `FROM golang:<version>-alpine` line found")
	}

	if gomod[1] != dockerfile[1] {
		t.Errorf("Go version disagreement:\n"+
			"  go.mod     declares %s\n"+
			"  Dockerfile builds with golang:%s-alpine\n"+
			"Bump both together.", gomod[1], dockerfile[1])
	}
}
