//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/message"

	"github.com/c360studio/semengine/natsclient"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	docsource "github.com/c360studio/semsource/processor/doc-source"
	sourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// Repro for #349: two sources under one project mint one system, so a file at
// the same relative path in each root maps to one entity ID and the later
// root's publish replaces the earlier root's. Root a seeds first, root b
// second; the graph should still hold both roots' files. It does not, so these
// tests fail until the change refuses the configuration (task 2.1); task 4.2
// turns them into refusal tests.

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestIntegration_ASTRootsSharingOneProjectKeepBothFiles(t *testing.T) {
	ctx := context.Background()
	tc, deps := startLifecycleStack(t, ctx)
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, a, "main.go", "package main\n\nfunc Alpha() {}\n")
	writeFile(t, b, "main.go", "package main\n\nfunc Beta() {}\n")

	startASTRoot(t, ctx, deps, "ast-source-a", a)
	untilMainFiles(t, ctx, tc.Client, "Alpha")
	startASTRoot(t, ctx, deps, "ast-source-b", b)
	untilMainFiles(t, ctx, tc.Client, "Beta")

	got := mainFiles(ctx, tc.Client)
	if !slices.ContainsFunc(got, func(c []string) bool { return containsSuffix(c, "Alpha") }) {
		t.Fatalf("root a's main.go is gone after root b seeded the same project: main.go file entities %v", got)
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestIntegration_DocRootsSharingOneProjectKeepBothDocuments(t *testing.T) {
	ctx := context.Background()
	tc, deps := startLifecycleStack(t, ctx)
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, a, "README.md", "# Alpha\n\nRoot a.\n")
	writeFile(t, b, "README.md", "# Beta\n\nRoot b.\n")

	startDocRoot(t, ctx, deps, "doc-source-a", a)
	untilDocTitle(t, ctx, tc.Client, "Alpha")
	startDocRoot(t, ctx, deps, "doc-source-b", b)
	untilDocTitle(t, ctx, tc.Client, "Beta")

	if !slices.Contains(docTitles(ctx, tc.Client), "Alpha") {
		t.Fatalf("root a's README.md is gone after root b seeded the same project: document titles %v", docTitles(ctx, tc.Client))
	}
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func startASTRoot(t *testing.T, ctx context.Context, deps component.Dependencies, instance, root string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{
		"watch_paths":    []map[string]any{{"path": root, "org": "acme", "project": "svc", "languages": []string{"go"}}},
		"watch_enabled":  false,
		"index_interval": "",
		"instance_name":  instance,
	})
	d, err := astsource.NewComponent(cfg, deps)
	if err != nil {
		t.Fatalf("ast-source NewComponent: %v", err)
	}
	startLifecycle(t, ctx, d.(*astsource.Component))
}

func startDocRoot(t *testing.T, ctx context.Context, deps component.Dependencies, instance, root string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{
		"paths": []string{root}, "org": "acme", "project": "svc",
		"watch_enabled": false, "instance_name": instance,
	})
	d, err := docsource.NewComponent(cfg, deps)
	if err != nil {
		t.Fatalf("doc-source NewComponent: %v", err)
	}
	startLifecycle(t, ctx, d.(*docsource.Component))
}

func startLifecycle(t *testing.T, ctx context.Context, c component.LifecycleComponent) {
	t.Helper()
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, c.Stop) })
}

// mainFiles returns the contains set of every stored main.go file entity.
func mainFiles(ctx context.Context, client *natsclient.Client) [][]string {
	var out [][]string
	for _, e := range prefixAll(ctx, client, "acme.test-a1b2c3") {
		if tripleObject(e.Triples, sourceast.CodeType) == "file" && tripleObject(e.Triples, sourceast.CodePath) == "main.go" {
			out = append(out, objects(e.Triples, sourceast.CodeContains))
		}
	}
	return out
}

// docTitles returns the title of every stored document entity.
func docTitles(ctx context.Context, client *natsclient.Client) []string {
	var out []string
	for _, e := range prefixAll(ctx, client, "acme.test-a1b2c3") {
		if tripleObject(e.Triples, source.DocType) == "document" {
			out = append(out, fmt.Sprint(tripleObject(e.Triples, source.DcTitle)))
		}
	}
	return out
}

func untilMainFiles(t *testing.T, ctx context.Context, client *natsclient.Client, fn string) {
	t.Helper()
	until(t, func() bool {
		return slices.ContainsFunc(mainFiles(ctx, client), func(c []string) bool { return containsSuffix(c, fn) })
	}, func() string {
		return fmt.Sprintf("no main.go contains %s; main.go file entities %v", fn, mainFiles(ctx, client))
	})
}

func untilDocTitle(t *testing.T, ctx context.Context, client *natsclient.Client, title string) {
	t.Helper()
	until(t, func() bool { return slices.Contains(docTitles(ctx, client), title) },
		func() string { return fmt.Sprintf("no document titled %s; titles %v", title, docTitles(ctx, client)) })
}

func until(t *testing.T, done func() bool, why func() string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if done() {
			return
		}
	}
	t.Fatal(why())
}

func objects(triples []message.Triple, predicate string) []string {
	var out []string
	for _, tr := range triples {
		if tr.Predicate == predicate {
			out = append(out, fmt.Sprint(tr.Object))
		}
	}
	return out
}

func containsSuffix(ids []string, name string) bool {
	return slices.ContainsFunc(ids, func(id string) bool { return strings.HasSuffix(id, name) })
}
