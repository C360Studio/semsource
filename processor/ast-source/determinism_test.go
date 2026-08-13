package astsource

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// CI determinism gate (#127), fixture tier: two in-process ingestion passes
// over a small, fixed multi-language fixture corpus, diffed with
// assertDeterministic (determinism_support_test.go). Runs unconditionally in
// `go test ./...` — no build tag; see determinism_repo_test.go's header for
// why the heavier repo-corpus tier also runs untagged (measured well under
// the ~90s bar for both tiers combined). Measured here at ~0.01s including
// -race.
//
// Coverage: java, python, and a combined c+cpp watch path. The combined corpus
// is deliberate, not incidental: C and C++ both claim the ".h" extension (see
// the extensionPrecedence comment in routing.go), and a watch path declaring
// both languages is exactly the configuration that used to route ".h" by
// ranging a Go map — the same header parsing as a different language from one
// run to the next, and changing the {domain} segment of every entity ID it
// produced. This corpus exercises that exact contested-extension path, twice,
// through the real production routing table (routeExtensions /
// Component.initializeWatchers), not a reimplementation of it.
//
// go.mod / package.json / Dockerfile (cfgfile) and markdown (doc) fixtures are
// intentionally NOT part of this tier: their production content — this
// repository's own go.mod, ui/package.json, docs — only exists in the checked
// out repo, so cfgfile/doc/hierarchy-at-scale coverage lives in the repo-corpus
// tier (determinism_repo_test.go). This tier's job is the AST routing
// regression class specifically, kept small enough to iterate on quickly.

// javaFixture is a minimal but non-trivial Java source: one class, one field,
// one constructor, one method — enough surface for identity, containment, and
// visibility triples to vary if anything upstream stops being deterministic.
const javaFixture = `package com.example.det;

/** Wraps a single sensor reading. */
public class Sensor {
    private int value;

    public Sensor(int value) {
        this.value = value;
    }

    public int read() {
        return value;
    }
}
`

// pythonFixture mirrors javaFixture's shape: a module-level function plus a
// class with one method.
const pythonFixture = `"""Math helpers for the determinism fixture."""


def add(a: int, b: int) -> int:
    """Add two integers."""
    return a + b


class Calculator:
    """A tiny calculator."""

    def multiply(self, a: int, b: int) -> int:
        return a * b
`

// cHeaderFixture is the CONTESTED file: a bare ".h" that both the c and cpp
// parsers claim. Which language reads it is the routing decision under test.
const cHeaderFixture = `#ifndef SHARED_H
#define SHARED_H

int shared_helper(int x);

#endif
`

// cSourceFixture includes the shared header and defines the C side.
const cSourceFixture = `#include "shared.h"

/** Doubles the input. */
int shared_helper(int x) {
    return x * 2;
}

static int counter = 0;
`

// cppSourceFixture includes the same shared header and defines the C++ side,
// so the watch path's declared language set is genuinely {c, cpp} and ".h" is
// genuinely contested rather than only nominally declared.
const cppSourceFixture = `#include "shared.h"

namespace det {

class Radio {
public:
    Radio();
    int send(int x);
};

Radio::Radio() {}

int Radio::send(int x) {
    return shared_helper(x);
}

}  // namespace det
`

// buildFixtureCorpus writes the java/python/c+cpp fixtures into fresh
// subdirectories of dir and returns the WatchPathConfig for each — org and
// project are fixed constants; only the routing/parsing determinism is under
// test, not entity-ID content.
func buildFixtureCorpus(t *testing.T, dir string) []WatchPathConfig {
	t.Helper()

	javaDir := filepath.Join(dir, "java")
	pythonDir := filepath.Join(dir, "python")
	mixedDir := filepath.Join(dir, "mixed")
	for _, d := range []string{javaDir, pythonDir, mixedDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	writeFixtureFile(t, javaDir, "Sensor.java", javaFixture)
	writeFixtureFile(t, pythonDir, "calc.py", pythonFixture)
	writeFixtureFile(t, mixedDir, "shared.h", cHeaderFixture)
	writeFixtureFile(t, mixedDir, "mesh.c", cSourceFixture)
	writeFixtureFile(t, mixedDir, "radio.cpp", cppSourceFixture)

	return []WatchPathConfig{
		{Path: javaDir, Org: "acme", Project: "det-java", Languages: []string{"java"}},
		{Path: pythonDir, Org: "acme", Project: "det-python", Languages: []string{"python"}},
		{Path: mixedDir, Org: "acme", Project: "det-mixed", Languages: []string{"c", "cpp"}},
	}
}

func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// runASTPass drives the real production AST pipeline — Component.initializeWatchers
// (which builds the extension→language routing table via routeExtensions),
// Component.parseDirectory, cross-file type-ref resolution, and
// semsourceast.BuildHierarchy — over watchPaths, and returns every entity it
// produced in comparison-normalized form. ctx is threaded through the parse
// walk for cancellation, per the repo's I/O convention.
func runASTPass(ctx context.Context, t *testing.T, watchPaths []WatchPathConfig) []detEntity {
	t.Helper()

	var logBuf bytes.Buffer
	c := &Component{
		config: Config{WatchPaths: watchPaths},
		logger: slog.New(slog.NewTextHandler(&logBuf, nil)),
	}
	if err := c.initializeWatchers(); err != nil {
		t.Fatalf("initializeWatchers: %v", err)
	}

	var out []detEntity
	for _, pw := range c.watchers {
		results, err := c.parseDirectory(ctx, pw)
		if err != nil {
			t.Fatalf("parseDirectory(%s): %v", pw.root, err)
		}
		for _, parser := range pw.parsers {
			if r, ok := parser.(typeRefResolver); ok {
				r.ResolveTypeRefs(results)
			}
		}
		for _, res := range results {
			for _, entity := range res.Entities {
				out = append(out, fromASTState(entity.EntityState()))
			}
		}
		for _, entity := range semsourceast.BuildHierarchy(results, pw.config.Org, pw.scopedSystem) {
			out = append(out, fromASTState(entity.EntityState()))
		}
	}
	if n := c.parseFailures.Load(); n != 0 {
		t.Fatalf("parseDirectory recorded %d parse failures: %s", n, logBuf.String())
	}
	return out
}

// TestDeterminism_LocalFixtures is the always-on tier of the CI determinism
// gate (#127): parse the fixture corpus twice in one process and require a
// byte-identical (after normalization) entity/triple set. Measured locally at
// well under 1s including -race, so it runs in the standard `go test -race
// ./...` job with no build tag.
func TestDeterminism_LocalFixtures(t *testing.T) {
	dir := t.TempDir()
	watchPaths := buildFixtureCorpus(t, dir)
	ctx := context.Background()

	pass1 := runASTPass(ctx, t, watchPaths)
	pass2 := runASTPass(ctx, t, watchPaths)

	if len(pass1) == 0 {
		t.Fatal("fixture corpus produced zero entities — fixture or routing is broken, not just non-deterministic")
	}
	assertDeterministic(t, "local AST fixtures (java/python/c+cpp)", pass1, pass2)
}
