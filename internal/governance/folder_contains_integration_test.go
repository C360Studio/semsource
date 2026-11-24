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

	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/source/ast"
)

// TestIntegration_WatchEditKeepsFolderContains is the #347 repro. The REAL
// ast-source indexes a/one.go, a/b/x.go and a/c/y.go; a watch-path modify of
// a/b/x.go must not shrink folder a's stored contains set.
func TestIntegration_WatchEditKeepsFolderContains(t *testing.T) {
	ctx, client, _, write := startFolderTree(t)
	seeded, _ := waitForFolderContains(t, ctx, client, "a", 0, 3)

	// The edit must land (X's revision moves past the seed's) before folder a
	// is read; a modify of an indexed path republishes no folder at all.
	_, xRev, _ := waitForFunction(t, ctx, client, "X", 0)
	for deadline, n := time.Now().Add(30*time.Second), 0; ; n++ {
		write("a/b/x.go", fmt.Sprintf("package b\n\nfunc X() {}\n\n// edit %d\n", n))
		if _, rev, ok := readFunction(ctx, client, "X"); ok && rev > xRev {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no revision of X newer than %d reached ENTITY_STATES", xRev)
		}
		time.Sleep(200 * time.Millisecond)
	}
	after, _, _ := readFolderContains(ctx, client, "a")
	if !slices.Equal(after, seeded) {
		t.Fatalf("folder a contains changed on a watch edit of a/b/x.go:\n seeded = %v\n after  = %v", seeded, after)
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestIntegration_WatchCreateAddsToFolder(t *testing.T) {
	ctx, client, _, write := startFolderTree(t)
	seeded, rev1 := waitForFolderContains(t, ctx, client, "a", 0, 3)

	after := untilFolderRevision(t, ctx, client, "a", rev1, func(int) {
		write("a/new.go", "package a\n\nfunc New() {}\n")
	})
	var added []string
	for _, id := range after {
		if !slices.Contains(seeded, id) {
			added = append(added, id)
		}
	}
	if len(after) != 4 || len(added) != 1 || !strings.HasSuffix(added[0], ".file.a-new-go") {
		t.Fatalf("folder a after creating a/new.go = %v, want %v plus a/new.go", after, seeded)
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestIntegration_WatchDeleteRemovesFromFolder(t *testing.T) {
	ctx, client, root, _ := startFolderTree(t)
	seeded, rev1 := waitForFolderContains(t, ctx, client, "a", 0, 3)
	_, revC := waitForFolderContains(t, ctx, client, "a-c", 0, 1)

	after := untilFolderRevision(t, ctx, client, "a", rev1, func(int) {
		_ = os.Remove(filepath.Join(root, "a/c/y.go"))
	})
	want := slices.DeleteFunc(slices.Clone(seeded), func(id string) bool { return strings.HasSuffix(id, ".folder.a-c") })
	if !slices.Equal(after, want) {
		t.Fatalf("folder a after deleting a/c/y.go = %v, want %v", after, want)
	}
	emptied := untilFolderRevision(t, ctx, client, "a-c", revC, func(int) {})
	if len(emptied) != 0 {
		t.Fatalf("folder a/c still contains %v after its only file was deleted", emptied)
	}

	// A whole-directory delete clears the gone folder's edges too: the
	// lifecycle pass never judges containers, so nothing else would.
	_, rev2, _ := readFolderContains(ctx, client, "a")
	_, revB, _ := readFolderContains(ctx, client, "a-b")
	after = untilFolderRevision(t, ctx, client, "a", rev2, func(int) {
		_ = os.RemoveAll(filepath.Join(root, "a/b"))
	})
	want = slices.DeleteFunc(want, func(id string) bool { return strings.HasSuffix(id, ".folder.a-b") })
	if !slices.Equal(after, want) {
		t.Fatalf("folder a after deleting directory a/b = %v, want %v", after, want)
	}
	if gone := untilFolderRevision(t, ctx, client, "a-b", revB, func(int) {}); len(gone) != 0 {
		t.Fatalf("folder a/b still contains %v after its directory was deleted", gone)
	}
}

// startFolderTree starts graph-ingest and a watching ast-source over a/one.go,
// a/b/x.go and a/c/y.go, and returns its root and a writer for files under it.
func startFolderTree(t *testing.T) (context.Context, *natsclient.Client, string, func(rel, body string)) {
	t.Helper()
	ctx := context.Background()
	tc := natstest.New(t,
		natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metric.NewMetricsRegistry())
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })

	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/one.go", "package a\n\nfunc One() {}\n")
	write("a/b/x.go", "package b\n\nfunc X() {}\n")
	write("a/c/y.go", "package c\n\nfunc Y() {}\n")
	runPass(t, ctx, tc.Client, root, true)
	return ctx, tc.Client, root, write
}

// untilFolderRevision applies change until folder <rel> has a revision newer
// than after, and returns that revision's contains set. The change is
// re-applied on every read: the watcher starts after the seed publishes, so a
// change landing before it watches produces no event.
func untilFolderRevision(
	t *testing.T, ctx context.Context, client *natsclient.Client, rel string, after uint64, change func(n int),
) []string {
	t.Helper()
	for deadline, n := time.Now().Add(30*time.Second), 0; time.Now().Before(deadline); n++ {
		change(n)
		if got, rev, ok := readFolderContains(ctx, client, rel); ok && rev > after {
			return got
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no revision of folder %q newer than %d reached ENTITY_STATES", rel, after)
	return nil
}

// waitForFolderContains returns folder <rel>'s sorted contains objects once a
// revision newer than after carries at least minCount of them.
func waitForFolderContains(
	t *testing.T, ctx context.Context, client *natsclient.Client, rel string, after uint64, minCount int,
) ([]string, uint64) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if got, rev, ok := readFolderContains(ctx, client, rel); ok && rev > after && len(got) >= minCount {
			return got, rev
		}
	}
	t.Fatalf("folder %q with %d contains newer than revision %d never reached ENTITY_STATES", rel, minCount, after)
	return nil, 0
}

// readFolderContains reads the stored folder entity whose ID ends in
// ".folder.<rel>" and returns its sorted code.structure.contains objects.
func readFolderContains(ctx context.Context, client *natsclient.Client, rel string) ([]string, uint64, bool) {
	bucket, err := client.GetKeyValueBucket(ctx, semgraph.BucketEntityStates)
	if err != nil {
		return nil, 0, false
	}
	keys, _ := bucket.Keys(ctx)
	for _, k := range keys {
		if !strings.HasSuffix(k, ".folder."+rel) {
			continue
		}
		entry, err := bucket.Get(ctx, k)
		if err != nil {
			return nil, 0, false
		}
		var st semgraph.EntityState
		if json.Unmarshal(entry.Value(), &st) != nil {
			return nil, 0, false
		}
		var out []string
		for _, tr := range st.Triples {
			if tr.Predicate == ast.CodeContains {
				out = append(out, fmt.Sprint(tr.Object))
			}
		}
		slices.Sort(out)
		return out, entry.Revision(), true
	}
	return nil, 0, false
}
