package filestore_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/storage/filestore"
)

// newStore is a test helper that creates a Store rooted at a fresh temp dir.
func newStore(t *testing.T) *filestore.Store {
	t.Helper()
	s, err := filestore.New(t.TempDir(), false)
	if err != nil {
		t.Fatalf("filestore.New: %v", err)
	}
	return s
}

// ─── Constructor ────────────────────────────────────────────────────────────

func TestNew_CreateIfMissingTrue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sub", "dir")
	s, err := filestore.New(root, true)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
	info, statErr := os.Stat(root)
	if statErr != nil {
		t.Fatalf("rootDir not created: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("rootDir is not a directory")
	}
}

func TestNew_CreateIfMissingFalse_MissingDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nonexistent")
	_, err := filestore.New(root, false)
	if err == nil {
		t.Fatal("expected error for missing dir with createIfMissing=false")
	}
}

func TestNew_ExistingDir(t *testing.T) {
	root := t.TempDir()
	s, err := filestore.New(root, false)
	if err != nil {
		t.Fatalf("expected no error for existing dir: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
}

// ─── Put + Get ───────────────────────────────────────────────────────────────

func TestPutGet_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  string
		data []byte
	}{
		{"small data", "docs/hello.txt", []byte("hello world")},
		{"large data", "blobs/large", bytes.Repeat([]byte("x"), 1<<20)}, // 1 MiB
		{"empty value", "empty/file", []byte{}},
		{"binary data", "bin/raw", []byte{0x00, 0xFF, 0xAB, 0xCD}},
		{"nested key", "a/b/c/d/e.json", []byte(`{"ok":true}`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			ctx := context.Background()

			if err := s.Put(ctx, tc.key, tc.data); err != nil {
				t.Fatalf("Put: %v", err)
			}

			got, err := s.Get(ctx, tc.key)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			if !bytes.Equal(got, tc.data) {
				t.Errorf("data mismatch: got %q, want %q", got, tc.data)
			}
		})
	}
}

func TestPut_CreatesIntermediateDirectories(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	key := "deep/nested/path/file.txt"
	if err := s.Put(ctx, key, []byte("content")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "content" {
		t.Errorf("got %q, want %q", got, "content")
	}
}

func TestPut_Overwrite(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	key := "file.txt"
	if err := s.Put(ctx, key, []byte("first")); err != nil {
		t.Fatalf("Put first: %v", err)
	}
	if err := s.Put(ctx, key, []byte("second")); err != nil {
		t.Fatalf("Put second: %v", err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// ─── Get errors ──────────────────────────────────────────────────────────────

func TestGet_MissingKey(t *testing.T) {
	s := newStore(t)
	_, err := s.Get(context.Background(), "nonexistent/key")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestList_EmptyPrefix_ReturnsAll(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	keys := []string{"a/1.txt", "b/2.txt", "c/3.txt"}
	for _, k := range keys {
		if err := s.Put(ctx, k, []byte(k)); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	got, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(got) != len(keys) {
		t.Fatalf("got %d keys, want %d: %v", len(got), len(keys), got)
	}
	for i, want := range keys {
		if got[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestList_WithPrefix_FiltersCorrectly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	puts := map[string]string{
		"docs/readme.md":  "readme",
		"docs/guide.md":   "guide",
		"code/main.go":    "main",
		"code/util.go":    "util",
		"config/app.json": "cfg",
	}
	for k, v := range puts {
		if err := s.Put(ctx, k, []byte(v)); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	got, err := s.List(ctx, "docs/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := map[string]bool{"docs/guide.md": true, "docs/readme.md": true}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d: %v", len(got), len(want), got)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q in results", k)
		}
	}
}

func TestList_Sorted(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	// Insert in intentionally unsorted order.
	keys := []string{"z/3.txt", "a/1.txt", "m/2.txt"}
	for _, k := range keys {
		if err := s.Put(ctx, k, []byte(k)); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	got, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []string{"a/1.txt", "m/2.txt", "z/3.txt"}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestList_EmptyStore_ReturnsEmptySlice(t *testing.T) {
	s := newStore(t)
	got, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestList_PrefixWithNoMatches_ReturnsEmptySlice(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, "docs/readme.md", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.List(ctx, "code/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ─── Delete ──────────────────────────────────────────────────────────────────

func TestDelete_RemovesFile(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	key := "to-delete/file.txt"
	if err := s.Put(ctx, key, []byte("bye")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, key)
	if err == nil {
		t.Fatal("expected Get to fail after Delete")
	}
}

func TestDelete_Idempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	key := "ghost/file.txt"
	// First delete: key never existed.
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of nonexistent key: %v", err)
	}
	// Second delete: still should not error.
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Second delete of nonexistent key: %v", err)
	}
}

// ─── Path traversal ──────────────────────────────────────────────────────────

func TestPathTraversal_Rejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	malicious := []string{
		"../escape.txt",
		"docs/../../escape.txt",
		"a/../../../etc/passwd",
	}

	for _, key := range malicious {
		t.Run(key, func(t *testing.T) {
			if err := s.Put(ctx, key, []byte("pwned")); err == nil {
				t.Errorf("Put with traversal key %q should have failed", key)
			}
			if _, err := s.Get(ctx, key); err == nil {
				t.Errorf("Get with traversal key %q should have failed", key)
			}
			if err := s.Delete(ctx, key); err == nil {
				t.Errorf("Delete with traversal key %q should have failed", key)
			}
		})
	}
}

func TestEmptyKey_Rejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, "", []byte("data")); err == nil {
		t.Error("Put with empty key should fail")
	}
	if _, err := s.Get(ctx, ""); err == nil {
		t.Error("Get with empty key should fail")
	}
	if err := s.Delete(ctx, ""); err == nil {
		t.Error("Delete with empty key should fail")
	}
}

// ─── Context cancellation ────────────────────────────────────────────────────

func TestContextCancellation(t *testing.T) {
	s := newStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	if err := s.Put(ctx, "key", []byte("data")); err == nil {
		t.Error("Put with cancelled context should fail")
	}
	if _, err := s.Get(ctx, "key"); err == nil {
		t.Error("Get with cancelled context should fail")
	}
	if _, err := s.List(ctx, ""); err == nil {
		t.Error("List with cancelled context should fail")
	}
	if err := s.Delete(ctx, "key"); err == nil {
		t.Error("Delete with cancelled context should fail")
	}
}

// ─── Close ───────────────────────────────────────────────────────────────────

func TestClose_NoOp(t *testing.T) {
	s := newStore(t)
	if err := s.Close(); err != nil {
		t.Errorf("Close returned unexpected error: %v", err)
	}
}
