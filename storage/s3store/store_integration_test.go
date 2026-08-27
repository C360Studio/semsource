//go:build integration

package s3store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/storage"

	"github.com/c360studio/semsource/internal/miniotest"
	"github.com/c360studio/semsource/storage/s3store"
)

// These tests prove the one thing the in-process fake in store_test.go cannot:
// that the client speaks S3 to a real server — path-style addressing, listing
// continuation, and the response shapes error classification reads.
//
// The server itself is provisioned by internal/miniotest, which every suite
// that needs a bucket shares. That is deliberate: two copies of a container
// definition drift, and the one that drifts is always the one you are not
// looking at.

// newContainerStore creates a bucket for this test and returns a Store scoped
// to it.
func newContainerStore(t *testing.T) *s3store.Store {
	t.Helper()
	store, _ := miniotest.NewStore(t)
	return store
}

// put is a fixture helper: a failure here is setup going wrong, not the
// behavior under test.
func put(t *testing.T, store *s3store.Store, key string, data []byte) {
	t.Helper()
	if err := store.Put(t.Context(), key, data); err != nil {
		t.Fatalf("Put %q: %v", key, err)
	}
}

// ─── Round trips ────────────────────────────────────────────────────────────

func TestIntegration_PutGet_RoundTrip(t *testing.T) {
	store := newContainerStore(t)

	cases := []struct {
		name string
		key  string
		data []byte
	}{
		{"text", "reports/q3.md", []byte("# Q3\n\nfindings\n")},
		{"binary", "assets/logo.bin", []byte{0x00, 0x01, 0xff, 0xfe, 0x7f}},
		{"empty", "reports/empty.md", []byte{}},
		{"nested key", "a/b/c/d/report.md", []byte("deep")},
		{"key with spaces", "reports/Q3 final draft.md", []byte("spaces")},
		{"non-ascii key", "rapports/résumé-北京.md", []byte("unicode")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			put(t, store, tc.key, tc.data)

			got, err := store.Get(t.Context(), tc.key)
			if err != nil {
				t.Fatalf("Get %q: %v", tc.key, err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Errorf("Get %q = %q, want %q", tc.key, got, tc.data)
			}
		})
	}
}

func TestIntegration_Put_Overwrite(t *testing.T) {
	store := newContainerStore(t)
	const key = "reports/q3.md"

	put(t, store, key, []byte("first"))
	put(t, store, key, []byte("second"))

	got, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Get = %q, want %q", got, "second")
	}
}

func TestIntegration_Open_StreamsContent(t *testing.T) {
	store := newContainerStore(t)
	const key = "reports/big.md"
	data := bytes.Repeat([]byte("semsource\n"), 4096)
	put(t, store, key, data)

	r, err := store.Open(t.Context(), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("streamed %d bytes, want %d", len(got), len(data))
	}
}

// ─── Error classification on the wire ───────────────────────────────────────

func TestIntegration_Get_MissingKey(t *testing.T) {
	store := newContainerStore(t)

	_, err := store.Get(t.Context(), "reports/never-written.md")
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected storage.ErrObjectNotFound, got: %v", err)
	}
}

func TestIntegration_Open_MissingKey(t *testing.T) {
	store := newContainerStore(t)

	r, err := store.Open(t.Context(), "reports/never-written.md")
	if err == nil {
		r.Close()
		t.Fatal("expected an error opening a key that holds no object")
	}
	// Open resolves the object before returning, so the caller learns this at
	// the call rather than on a later read.
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected storage.ErrObjectNotFound, got: %v", err)
	}
}

// TestIntegration_Get_MissingBucket is the wire proof for the classification
// the fake can only assert in the abstract. A real HEAD 404 carries no body,
// so the client reports a wrong bucket and a missing key identically; the
// store has to ask a second question to tell them apart. Getting this wrong
// makes a typo in the bucket name read as a corpus with nothing in it.
func TestIntegration_Get_MissingBucket(t *testing.T) {
	endpoint := miniotest.Endpoint(t)
	miniotest.SetCredentials(t)

	store, err := s3store.New(s3store.Config{
		Bucket:    "bucket-that-was-never-created",
		Endpoint:  endpoint,
		Region:    miniotest.Region,
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}

	_, err = store.Get(t.Context(), "reports/q3.md")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("a missing bucket must not classify as a missing object, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bucket-that-was-never-created") {
		t.Errorf("error should name the bucket, got: %v", err)
	}
}

// ─── Listing ────────────────────────────────────────────────────────────────

func TestIntegration_List_EmptyPrefixReturnsAll(t *testing.T) {
	store := newContainerStore(t)
	keys := []string{"a.md", "reports/b.md", "reports/nested/c.md"}
	for _, key := range keys {
		put(t, store, key, []byte(key))
	}

	got, err := store.List(t.Context(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	assertKeys(t, got, keys)
}

func TestIntegration_List_WithPrefixFilters(t *testing.T) {
	store := newContainerStore(t)
	for _, key := range []string{
		"reports/a.md", "reports/b.md", "reports-archive/c.md", "assets/d.png",
	} {
		put(t, store, key, []byte(key))
	}

	got, err := store.List(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	assertKeys(t, got, []string{"reports/a.md", "reports/b.md"})
}

func TestIntegration_List_Sorted(t *testing.T) {
	store := newContainerStore(t)
	written := []string{"z.md", "m.md", "a.md", "b/z.md", "b/a.md"}
	for _, key := range written {
		put(t, store, key, []byte(key))
	}

	got, err := store.List(t.Context(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("keys are not sorted: %v", got)
	}
}

func TestIntegration_List_NoMatchesReturnsEmptySlice(t *testing.T) {
	store := newContainerStore(t)
	put(t, store, "reports/a.md", []byte("a"))

	got, err := store.List(t.Context(), "nothing-here/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Fatal("expected an empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("List = %v, want empty", got)
	}
}

// TestIntegration_List_AcrossContinuationPages proves the client walks a real
// server's continuation tokens. S3 caps a listing response at 1000 keys, so
// this is the smallest corpus that produces a second page at all.
func TestIntegration_List_AcrossContinuationPages(t *testing.T) {
	store := newContainerStore(t)
	const total = 1001

	// Written concurrently: 1001 sequential round trips is the slowest part of
	// this suite by an order of magnitude, and nothing here depends on order.
	const writers = 16
	keys := make(chan string, total)
	for i := range total {
		keys <- fmt.Sprintf("page/%04d.md", i)
	}
	close(keys)

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range keys {
				if err := store.Put(t.Context(), key, []byte(key)); err != nil {
					errs <- fmt.Errorf("put %q: %w", key, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("seed the bucket: %v", err)
	}

	got, err := store.List(t.Context(), "page/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != total {
		t.Errorf("listed %d keys, want %d — a listing that stops at the first page is exactly this short", len(got), total)
	}

	// The metadata listing is what change detection consumes, so it has to
	// cross pages too. It shares an implementation with List today; asserting
	// both means a future divergence fails here rather than silently halving
	// the corpus a source can see.
	infos, err := store.Objects(t.Context(), "page/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != total {
		t.Errorf("enumerated %d objects, want %d", len(infos), total)
	}
}

// ─── Deletion and cancellation ──────────────────────────────────────────────

func TestIntegration_Delete_RemovesObject(t *testing.T) {
	store := newContainerStore(t)
	const key = "reports/q3.md"
	put(t, store, key, []byte("content"))

	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(t.Context(), key); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected the object to be gone, got: %v", err)
	}
}

func TestIntegration_Delete_Idempotent(t *testing.T) {
	store := newContainerStore(t)

	if err := store.Delete(t.Context(), "reports/never-written.md"); err != nil {
		t.Errorf("deleting an absent key should be a no-op, got: %v", err)
	}
}

func TestIntegration_ContextCancellation(t *testing.T) {
	store := newContainerStore(t)
	put(t, store, "reports/q3.md", []byte("content"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := store.Get(ctx, "reports/q3.md"); err == nil {
		t.Error("Get: expected an error on a cancelled context")
	}
	if _, err := store.List(ctx, ""); err == nil {
		t.Error("List: expected an error on a cancelled context")
	}
	if err := store.Put(ctx, "reports/new.md", []byte("x")); err == nil {
		t.Error("Put: expected an error on a cancelled context")
	}
}

// assertKeys compares a listing against the keys a test wrote.
func assertKeys(t *testing.T, got, want []string) {
	t.Helper()

	sorted := append([]string(nil), want...)
	sort.Strings(sorted)

	if len(got) != len(sorted) {
		t.Fatalf("List = %v, want %v", got, sorted)
	}
	for i := range sorted {
		if got[i] != sorted[i] {
			t.Errorf("key %d = %q, want %q", i, got[i], sorted[i])
		}
	}
}

// TestIntegration_Objects_CarryChangeMetadata is the wire proof that a real
// store's listing brings back what change detection compares. The values are
// the server's, not ours: an ETag that arrives quoted, or a size the parse
// dropped, would make every pass look like a change.
func TestIntegration_Objects_CarryChangeMetadata(t *testing.T) {
	store := newContainerStore(t)
	const key = "reports/q3.md"
	put(t, store, key, []byte("first"))

	infos, err := store.Objects(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("Objects returned %d entries, want 1", len(infos))
	}

	first := infos[0]
	if first.Key != key {
		t.Errorf("Key = %q, want %q", first.Key, key)
	}
	if first.ETag == "" {
		t.Error("ETag is empty — change detection has nothing to compare")
	}
	if strings.Contains(first.ETag, `"`) {
		t.Errorf("ETag = %q, want the quotes stripped", first.ETag)
	}
	if first.Size != int64(len("first")) {
		t.Errorf("Size = %d, want %d", first.Size, len("first"))
	}
	if first.LastModified.IsZero() {
		t.Error("LastModified is zero")
	}

	// Replacing the content at the same key must move the token, or a changed
	// object would never be re-ingested.
	put(t, store, key, []byte("second"))

	infos, err = store.Objects(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("Objects after overwrite: %v", err)
	}
	if infos[0].ETag == first.ETag {
		t.Errorf("ETag did not change when the content did: %q", infos[0].ETag)
	}
}

func TestIntegration_Objects_ScopedToPrefix(t *testing.T) {
	store := newContainerStore(t)
	for _, key := range []string{"reports/a.md", "reports/b.md", "assets/c.png"} {
		put(t, store, key, []byte(key))
	}

	infos, err := store.Objects(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("Objects returned %d entries, want 2", len(infos))
	}
	for _, info := range infos {
		if !strings.HasPrefix(info.Key, "reports/") {
			t.Errorf("key %q is outside the requested prefix", info.Key)
		}
	}
}
