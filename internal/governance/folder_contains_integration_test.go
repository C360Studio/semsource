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
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
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
	seeded, _ := waitForFolderContains(t, ctx, client, "a", 0, 3)

	// The create is re-applied until it lands: the watcher starts after the
	// seed publishes. The wait is on the complete outcome.
	untilFolderChange(t, ctx, client, "a", func(got []string) bool {
		var added []string
		for _, id := range got {
			if !slices.Contains(seeded, id) {
				added = append(added, id)
			}
		}
		return len(got) == len(seeded)+1 && len(added) == 1 && strings.HasSuffix(added[0], ".file.a-new-go")
	}, func() { write("a/new.go", "package a\n\nfunc New() {}\n") })
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestIntegration_WatchDeleteRemovesFromFolder(t *testing.T) {
	ctx, client, root, write := startFolderTree(t)
	seeded, _ := waitForFolderContains(t, ctx, client, "a", 0, 3)
	probe := awaitWatching(t, ctx, client, write)

	if err := os.Remove(filepath.Join(root, "a/c/y.go")); err != nil {
		t.Fatal(err)
	}
	want := slices.DeleteFunc(append(slices.Clone(seeded), probe), func(id string) bool { return strings.HasSuffix(id, ".folder.a-c") })
	slices.Sort(want)
	untilFolder(t, ctx, client, "a", func(got []string) bool { return slices.Equal(got, want) })
	untilFolder(t, ctx, client, "a-c", func(got []string) bool { return len(got) == 0 })

	// A whole-directory delete clears the gone folder's edges too: the
	// lifecycle pass never judges containers, so nothing else would.
	if err := os.RemoveAll(filepath.Join(root, "a/b")); err != nil {
		t.Fatal(err)
	}
	want = slices.DeleteFunc(want, func(id string) bool { return strings.HasSuffix(id, ".folder.a-b") })
	untilFolder(t, ctx, client, "a", func(got []string) bool { return slices.Equal(got, want) })
	untilFolder(t, ctx, client, "a-b", func(got []string) bool { return len(got) == 0 })
}

// TestIntegration_WatchDeleteKeepsCollidingFolder: folders a-b and a/b share
// one entity ID. Deleting a-b's only file must not empty the ID a/b still
// holds (codex review P1).
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestIntegration_WatchDeleteKeepsCollidingFolder(t *testing.T) {
	ctx, client, root, write := startFolderTree(t,
		"a-b/one.go", "package ab\n\nfunc One() {}\n",
		"a/b/two.go", "package b\n\nfunc Two() {}\n",
		"c/z.go", "package c\n\nfunc Z() {}\n")
	awaitWatching(t, ctx, client, write)

	if err := os.Remove(filepath.Join(root, "a-b/one.go")); err != nil {
		t.Fatal(err)
	}
	// Barrier 1: the repo drops top-level folder a-b only once the first
	// delete's event has been handled and its publishes (and reconciles)
	// queued. Only then is the sentinel delete issued, so the two can never
	// share a watcher batch.
	untilContains(t, ctx, client, ".code.repo.", func(got []string) bool {
		return len(got) > 0 && !slices.ContainsFunc(got, func(id string) bool { return strings.HasSuffix(id, ".folder.a-b") })
	})
	// Barrier 2: reconciles run one at a time in enqueue order, and only a
	// reconcile empties a folder. Once c is empty, every reconcile the first
	// delete queued has run.
	if err := os.Remove(filepath.Join(root, "c/z.go")); err != nil {
		t.Fatal(err)
	}
	untilFolder(t, ctx, client, "c", func(got []string) bool { return len(got) == 0 })

	got, _, _ := readFolderContains(ctx, client, "a-b")
	if !slices.ContainsFunc(got, func(id string) bool { return strings.HasSuffix(id, ".file.a-b-two-go") }) {
		t.Fatalf("folder ID a-b = %v after deleting a-b/one.go; a/b still holds a/b/two.go", got)
	}
}

// awaitWatching creates a/probe.go until folder a lists it, proving the
// watcher delivers events before a test acts once (a delete cannot be
// re-applied). It returns the probe file's entity ID.
//
// The seed walks before it starts the watcher, so a probe written during the
// walk is indexed by the walk and proves nothing (#363). The seed publishes
// folders only after its walk, so the probe is written only once folder a
// exists: from then on only the watcher can deliver it.
func awaitWatching(t *testing.T, ctx context.Context, client *natsclient.Client, write func(rel, body string)) string {
	t.Helper()
	untilFolder(t, ctx, client, "a", func([]string) bool { return true })
	var probe string
	untilFolderChange(t, ctx, client, "a", func(got []string) bool {
		i := slices.IndexFunc(got, func(id string) bool { return strings.HasSuffix(id, ".file.a-probe-go") })
		if i >= 0 {
			probe = got[i]
		}
		return i >= 0
	}, func() { write("a/probe.go", "package a\n\nfunc Probe() {}\n") })
	return probe
}

// untilContains waits until the first entity whose key contains infix has a
// contains set that satisfies done.
func untilContains(t *testing.T, ctx context.Context, client *natsclient.Client, infix string, done func([]string) bool) {
	t.Helper()
	var got []string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if c, ok := readContains(ctx, client, func(k string) bool { return strings.Contains(k, infix) }); ok {
			got = c
			if done(got) {
				return
			}
		}
	}
	t.Fatalf("entity %q never reached the expected contains set; last read %v", infix, got)
}

// untilFolder waits until folder <rel>'s contains set satisfies done.
func untilFolder(t *testing.T, ctx context.Context, client *natsclient.Client, rel string, done func([]string) bool) {
	t.Helper()
	untilFolderChange(t, ctx, client, rel, done, func() {})
}

// untilFolderChange applies change, then reads folder <rel> until its
// contains set satisfies done or 30s pass. It waits on the outcome, not on a
// newer revision, so a stream write seen before its reconcile lands is not
// mistaken for the final state.
func untilFolderChange(
	t *testing.T, ctx context.Context, client *natsclient.Client, rel string, done func([]string) bool, change func(),
) {
	t.Helper()
	var got []string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		change()
		if c, _, ok := readFolderContains(ctx, client, rel); ok {
			got = c
			if done(got) {
				return
			}
		}
	}
	t.Fatalf("folder %q never reached the expected contains set; last read %v", rel, got)
}

// startFolderTree starts graph-ingest and a watching ast-source over files
// (path, body pairs; default a/one.go, a/b/x.go and a/c/y.go), and returns its root and a writer for files under it.
func startFolderTree(t *testing.T, files ...string) (context.Context, *natsclient.Client, string, func(rel, body string)) {
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
	if len(files) == 0 {
		files = []string{
			"a/one.go", "package a\n\nfunc One() {}\n",
			"a/b/x.go", "package b\n\nfunc X() {}\n",
			"a/c/y.go", "package c\n\nfunc Y() {}\n",
		}
	}
	for i := 0; i+1 < len(files); i += 2 {
		write(files[i], files[i+1])
	}
	runPass(t, ctx, tc.Client, root, true)
	return ctx, tc.Client, root, write
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
	var rev uint64
	got, ok := readContainsRev(ctx, client, func(k string) bool { return strings.HasSuffix(k, ".folder."+rel) }, &rev)
	return got, rev, ok
}

// readContains reads the first stored entity whose key matches and returns
// its sorted code.structure.contains objects.
func readContains(ctx context.Context, client *natsclient.Client, match func(string) bool) ([]string, bool) {
	return readContainsRev(ctx, client, match, nil)
}

func readContainsRev(ctx context.Context, client *natsclient.Client, match func(string) bool, rev *uint64) ([]string, bool) {
	bucket, err := client.GetKeyValueBucket(ctx, semgraph.BucketEntityStates)
	if err != nil {
		return nil, false
	}
	keys, _ := bucket.Keys(ctx)
	for _, k := range keys {
		if !match(k) {
			continue
		}
		entry, err := bucket.Get(ctx, k)
		if err != nil {
			return nil, false
		}
		var st semgraph.EntityState
		if json.Unmarshal(entry.Value(), &st) != nil {
			return nil, false
		}
		var out []string
		for _, tr := range st.Triples {
			if tr.Predicate == ast.CodeContains {
				out = append(out, fmt.Sprint(tr.Object))
			}
		}
		slices.Sort(out)
		if rev != nil {
			*rev = entry.Revision()
		}
		return out, true
	}
	return nil, false
}
