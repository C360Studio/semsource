//go:build integration

package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	docsource "github.com/c360studio/semsource/processor/doc-source"
	sourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// #349: two sources of one kind under one project mint one system, so a file
// at the same relative path in each root maps to one entity ID and the later
// root's publish replaces the earlier root's. Two ast-source or doc-source
// components cannot see each other, so the refusal happens where the whole
// source set is visible: the file's sources (`semsource validate` / `run`),
// boot over the components it would spawn, and a runtime add. Each test
// asserts the #349 pair is refused at all three, then runs the same pair
// under distinct projects through the components to ENTITY_STATES and shows
// both roots' entities survive. checkBootCollisions itself lives in
// cmd/semsource (covered by cmd/semsource/shared_system_test.go); here boot is
// its check, CheckSystemCollisions over Planned with the discovery pins.

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestIntegration_ASTRootsSharingOneProjectAreRefused(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, a, "main.go", "package main\n\nfunc Alpha() {}\n")
	writeFile(t, b, "main.go", "package main\n\nfunc Beta() {}\n")
	ast := func(root, project string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: root, Project: project, Language: "go"}
	}
	assertRefusedAtEveryEntryPoint(t, ast(a, "svc"), ast(b, "svc"), a, b)

	ctx := context.Background()
	assertAdmittedAtEveryEntryPoint(t, ast(a, "svc-a"), ast(b, "svc-b"))
	tc, deps := startLifecycleStack(t, ctx)
	startASTRoot(t, ctx, deps, "ast-source-a", a, "svc-a")
	startASTRoot(t, ctx, deps, "ast-source-b", b, "svc-b")
	untilMainFiles(t, ctx, tc.Client, "Alpha")
	untilMainFiles(t, ctx, tc.Client, "Beta")
	got := mainFiles(ctx, tc.Client)
	for _, fn := range []string{"Alpha", "Beta"} {
		if !slices.ContainsFunc(got, func(c []string) bool { return containsSuffix(c, fn) }) {
			t.Errorf("distinct projects: the main.go holding %s is gone; main.go file entities %v", fn, got)
		}
	}
}

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestIntegration_DocRootsSharingOneProjectAreRefused(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, a, "README.md", "# Alpha\n\nRoot a.\n")
	writeFile(t, b, "README.md", "# Beta\n\nRoot b.\n")
	docs := func(root, project string) config.SourceEntry {
		return config.SourceEntry{Type: "docs", Paths: []string{root}, Project: project}
	}
	assertRefusedAtEveryEntryPoint(t, docs(a, "svc"), docs(b, "svc"), a, b)

	ctx := context.Background()
	assertAdmittedAtEveryEntryPoint(t, docs(a, "svc-a"), docs(b, "svc-b"))
	tc, deps := startLifecycleStack(t, ctx)
	startDocRoot(t, ctx, deps, "doc-source-a", a, "svc-a")
	startDocRoot(t, ctx, deps, "doc-source-b", b, "svc-b")
	untilDocTitle(t, ctx, tc.Client, "Alpha")
	untilDocTitle(t, ctx, tc.Client, "Beta")
	titles := docTitles(ctx, tc.Client)
	for _, title := range []string{"Alpha", "Beta"} {
		if !slices.Contains(titles, title) {
			t.Errorf("distinct projects: README.md titled %s is gone; document titles %v", title, titles)
		}
	}
}

var sharedSystemOpts = sourcespawn.Options{Org: "acme"}

// assertRefusedAtEveryEntryPoint asserts the pair first, second is refused by
// the file check, by boot's check and by a runtime add, each naming both
// roots, and that the refused add persists nothing.
func assertRefusedAtEveryEntryPoint(t *testing.T, first, second config.SourceEntry, rootA, rootB string) {
	t.Helper()
	ctx := context.Background()
	namesBoth := func(at string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s admitted two sources under one system", at)
			return
		}
		if sourcespawn.CodeOf(err) != sourcespawn.CodeValidationFailed {
			t.Errorf("%s: err %v has code %q, want %s", at, err, sourcespawn.CodeOf(err), sourcespawn.CodeValidationFailed)
		}
		for _, root := range []string{rootA, rootB} {
			if !strings.Contains(err.Error(), root) {
				t.Errorf("%s: refusal %q does not name root %s", at, err, root)
			}
		}
	}

	namesBoth("file sources", sourcespawn.CheckSourceCollisions(ctx, []config.SourceEntry{first, second}, sharedSystemOpts))
	namesBoth("boot", bootCheck(t, first, second))

	store := newMemConfigStore()
	if _, err := sourcespawn.Add(ctx, first, store, sharedSystemOpts); err != nil {
		t.Fatalf("register the first source: %v", err)
	}
	before := maps.Clone(store.GetConfig().Get().Components)
	_, err := sourcespawn.AddWithChecker(ctx, second, store, nil, sharedSystemOpts)
	namesBoth("runtime add", err)
	if after := store.GetConfig().Get().Components; !maps.EqualFunc(before, after, func(x, y types.ComponentConfig) bool {
		return x.Enabled == y.Enabled && bytes.Equal(x.Config, y.Config)
	}) {
		t.Errorf("a refused add changed the registered components: before %v, after %v", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
	}
}

// assertAdmittedAtEveryEntryPoint is the positive counterpart: the same two
// roots under distinct projects pass every entry point.
func assertAdmittedAtEveryEntryPoint(t *testing.T, first, second config.SourceEntry) {
	t.Helper()
	ctx := context.Background()
	if err := sourcespawn.CheckSourceCollisions(ctx, []config.SourceEntry{first, second}, sharedSystemOpts); err != nil {
		t.Errorf("file sources refused distinct projects: %v", err)
	}
	if err := bootCheck(t, first, second); err != nil {
		t.Errorf("boot refused distinct projects: %v", err)
	}
	store := newMemConfigStore()
	for _, src := range []config.SourceEntry{first, second} {
		if _, err := sourcespawn.Add(ctx, src, store, sharedSystemOpts); err != nil {
			t.Errorf("runtime add refused distinct projects: %v", err)
		}
	}
}

// bootCheck runs boot's collision check (checkBootCollisions' body) over the
// components the two sources build. The #349 pair builds one instance name,
// which one component map cannot hold twice, so each is filed under its own
// name, as a file source and a desired-manifest source of different names
// would be.
func bootCheck(t *testing.T, sources ...config.SourceEntry) error {
	t.Helper()
	components := semconfig.ComponentConfigs{}
	for i, src := range sources {
		built, err := sourcespawn.Build(src, sharedSystemOpts)
		if err != nil {
			t.Fatalf("build %+v: %v", src, err)
		}
		for name, cc := range built {
			components[fmt.Sprintf("%s-%d", name, i)] = cc
		}
	}
	pins, err := sourcespawn.PinInstances(components)
	if err != nil {
		t.Fatal(err)
	}
	return sourcespawn.CheckSystemCollisions(context.Background(), sourcespawn.Planned(components, pins))
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func startASTRoot(t *testing.T, ctx context.Context, deps component.Dependencies, instance, root, project string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{
		"watch_paths":    []map[string]any{{"path": root, "org": "acme", "project": project, "languages": []string{"go"}}},
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

func startDocRoot(t *testing.T, ctx context.Context, deps component.Dependencies, instance, root, project string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{
		"paths": []string{root}, "org": "acme", "project": project,
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
