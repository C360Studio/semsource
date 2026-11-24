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
	seeded, rev1 := waitForFolderContains(t, ctx, tc.Client, "a", 0, 3)

	// The watcher starts after the seed publishes, so re-apply the edit until
	// a newer revision of folder a lands (an identical rewrite is harmless).
	var after []string
	for deadline, n := time.Now().Add(30*time.Second), 0; ; n++ {
		write("a/b/x.go", fmt.Sprintf("package b\n\nfunc X() {}\n\n// edit %d\n", n))
		if got, rev, ok := readFolderContains(ctx, tc.Client, "a"); ok && rev > rev1 {
			after = got
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no revision of folder a newer than %d reached ENTITY_STATES", rev1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !slices.Equal(after, seeded) {
		t.Fatalf("folder a contains changed on a watch edit of a/b/x.go:\n seeded = %v\n after  = %v", seeded, after)
	}
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
