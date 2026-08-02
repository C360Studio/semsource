//go:build integration

package astsource

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/storage/objectstore"

	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

// bodyOffloadStore returns a real object store backed by the test NATS.
func bodyOffloadStore(t *testing.T) *objectstore.Store {
	t.Helper()
	tc := natsclient.NewTestClient(t, natsclient.WithJetStream())
	store, err := objectstore.NewStoreWithConfig(context.Background(), tc.Client, objectstore.Config{
		BucketName:   graph.BodyStoreBucket,
		InstanceName: graph.BodyStoreInstance,
	})
	if err != nil {
		t.Fatalf("object store: %v", err)
	}
	return store
}

// synthBodies builds n distinct bodies of roughly the measured average size
// (14.7 MB over 25,960 bodies on OSH Core ~= 570 bytes).
func synthBodies(n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = []byte(fmt.Sprintf("// body %d\n%s", i, strings.Repeat("x = compute(i);\n", 34)))
	}
	return out
}

// TestIntegration_BodyPutCost measures what a per-entity object-store Put
// actually costs, and what concurrency buys.
//
// Context: on OSH Core the whole CPU side of body offload — parse, read, slice,
// sha256 — is 7.5s, of which sha256 is 43ms. The container nonetheless took
// 600s+. The difference is this loop: one SYNCHRONOUS Put per body-bearing
// entity, 25,960 of them for Java alone.
func TestIntegration_BodyPutCost(t *testing.T) {
	store := bodyOffloadStore(t)
	ctx := context.Background()

	const n = 300
	bodies := synthBodies(n)

	// Serial — how the loop works today.
	start := time.Now()
	for i, b := range bodies {
		if err := store.Put(ctx, fmt.Sprintf("code:serial-%d", i), b); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	serial := time.Since(start)
	perPut := serial / n

	// Concurrent — the same work with a bounded worker pool.
	const workers = 16
	start = time.Now()
	var wg sync.WaitGroup
	ch := make(chan int, n)
	for i := range bodies {
		ch <- i
	}
	close(ch)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				if err := store.Put(ctx, fmt.Sprintf("code:conc-%d", i), bodies[i]); err != nil {
					t.Errorf("put: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	concurrent := time.Since(start)

	t.Logf("serial     %d puts in %s  (%s per put)", n, serial.Round(time.Millisecond), perPut.Round(time.Microsecond))
	t.Logf("concurrent %d puts in %s  (%d workers)", n, concurrent.Round(time.Millisecond), workers)
	if concurrent > 0 {
		t.Logf("speedup    %.1fx", float64(serial)/float64(concurrent))
	}
	t.Logf("EXTRAPOLATED to the 25,960 Java bodies on OSH Core:")
	t.Logf("  serial     ~%s", (perPut * 25960).Round(time.Second))
	t.Logf("  concurrent ~%s", (time.Duration(float64(concurrent) / float64(n) * 25960)).Round(time.Second))

	if perPut <= 0 {
		t.Fatal("per-put cost measured as zero; the benchmark is not exercising the store")
	}
}

// TestIntegration_DuplicateBodyPutIsRedundant pins the second finding: keys are
// content-addressed, so re-Putting a body already stored writes identical bytes
// to an identical key. Measured on OSH Core, 12.3% of all Puts are exactly this.
func TestIntegration_DuplicateBodyPutIsRedundant(t *testing.T) {
	store := bodyOffloadStore(t)
	ctx := context.Background()
	body := []byte("func f() { return 1 }")
	key := bodyKeyPrefix + hashBody(string(body))

	if err := store.Put(ctx, key, body); err != nil {
		t.Fatalf("first put: %v", err)
	}
	first, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := store.Put(ctx, key, body); err != nil {
		t.Fatalf("second put: %v", err)
	}
	second, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if string(first) != string(second) {
		t.Fatal("content-addressed key returned different bytes; the dedup premise is wrong")
	}
}

// TestIntegration_DedupSkipsThePutNotTheAttachment is the trap in the dedup:
// two entities with byte-identical bodies share one content-addressed key, so
// the second Put is skipped — but the second ENTITY still needs its handle
// triples. Skipping the attachment as well would silently strip that entity's
// body, and nothing downstream would report it.
func TestIntegration_DedupSkipsThePutNotTheAttachment(t *testing.T) {
	c := &Component{bodyStore: bodyOffloadStore(t), logger: testLogger()}

	// Two entities, identical bodies -> identical keys.
	src := "package a\n\nfunc x() { return }\n\nfunc y() { return }\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res := &semsourceast.ParseResult{
		Path: "a.go",
		Entities: []*semsourceast.CodeEntity{
			{ID: "ent.one", Type: semsourceast.TypeFunction, StartLine: 3, EndLine: 3},
			{ID: "ent.two", Type: semsourceast.TypeFunction, StartLine: 5, EndLine: 5},
		},
	}
	// Lines 3 and 5 differ; force identical bodies by pointing both at line 3.
	res.Entities[1].StartLine, res.Entities[1].EndLine = 3, 3

	// The dedup only fires once a key is already recorded, which within a single
	// call it is not (the first Put has not completed when the second entity is
	// examined). Parsing the SAME file twice is both the deterministic way to
	// exercise it and the realistic one — a watch re-parse of an unchanged file.
	if first := c.bodiesForResult(context.Background(), res, dir); len(first) != 2 {
		t.Fatalf("first pass got %d bodies, want 2", len(first))
	}
	out := c.bodiesForResult(context.Background(), res, dir)

	if len(out) != 2 {
		t.Fatalf("re-parse got %d bodies, want 2 — a deduped body must still attach to its entity", len(out))
	}
	one, two := out["ent.one"], out["ent.two"]
	if one.ref == nil || two.ref == nil {
		t.Fatal("an entity lost its StorageReference to a deduped body")
	}
	if one.ref.Key != two.ref.Key {
		t.Errorf("identical bodies got different keys: %q vs %q", one.ref.Key, two.ref.Key)
	}
	if len(two.triples) != 2 {
		t.Errorf("deduped entity has %d handle triples, want 2", len(two.triples))
	}
}
