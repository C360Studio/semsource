package astsource

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// fakeStore is a minimal in-memory storage.Store for the producer test.
type fakeStore struct{ data map[string][]byte }

func newFakeStore() *fakeStore { return &fakeStore{data: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, data []byte) error {
	f.data[key] = append([]byte(nil), data...)
	return nil
}
func (f *fakeStore) Get(_ context.Context, key string) ([]byte, error)  { return f.data[key], nil }
func (f *fakeStore) List(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (f *fakeStore) Delete(_ context.Context, key string) error         { delete(f.data, key); return nil }

func TestBodiesForResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "svc.go")
	src := "package svc\n\nfunc Dispatch() {\n\tOnEvent()\n}\n\nfunc OnEvent() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newFakeStore()
	c := &Component{logger: slog.Default(), bodyStore: store}

	// ParseResult.Path is relative to the watcher root (as the parser emits it);
	// the producer joins root back to read the file.
	result := &semsourceast.ParseResult{
		Path: "svc.go",
		Entities: []*semsourceast.CodeEntity{
			// A file container: no body of its own, must be skipped.
			{ID: "o.p.golang.s.file.svc", Type: semsourceast.TypeFile, StartLine: 1, EndLine: 7},
			// A function symbol: lines 3–5 → verbatim body offloaded + handle stamped.
			{ID: "o.p.golang.s.function.dispatch", Type: semsourceast.TypeFunction, StartLine: 3, EndLine: 5},
		},
	}

	got := c.bodiesForResult(context.Background(), result, root)

	if _, ok := got["o.p.golang.s.file.svc"]; ok {
		t.Error("file container should not get a body")
	}
	body := got["o.p.golang.s.function.dispatch"]
	if len(body.triples) != 2 {
		t.Fatalf("expected 2 body handle triples for the function, got %d", len(body.triples))
	}

	var instance, key string
	for _, tr := range body.triples {
		switch tr.Predicate {
		case semsourceast.CodeBodyStore:
			instance, _ = tr.Object.(string)
		case semsourceast.CodeBodyKey:
			key, _ = tr.Object.(string)
		}
	}
	if instance != graph.BodyStoreInstance {
		t.Errorf("body store instance = %q; want %q", instance, graph.BodyStoreInstance)
	}

	// The StorageRef must point at the SAME blob as the handle triples so
	// graph-embedding embeds the body via the shared StoreRegistry (ADR-063).
	want := "func Dispatch() {\n\tOnEvent()\n}"
	if body.ref == nil {
		t.Fatal("a body-bearing entity must carry a StorageRef")
	}
	if body.ref.StorageInstance != instance {
		t.Errorf("StorageRef.StorageInstance = %q; want %q (handle instance)", body.ref.StorageInstance, instance)
	}
	if body.ref.Key != key {
		t.Errorf("StorageRef.Key = %q; want %q (handle key)", body.ref.Key, key)
	}
	if int(body.ref.Size) != len(want) {
		t.Errorf("StorageRef.Size = %d; want %d", body.ref.Size, len(want))
	}
	// The stored blob must be the exact verbatim range [3,5], byte-for-byte.
	if got := string(store.data[key]); got != want {
		t.Fatalf("offloaded body = %q; want %q", got, want)
	}
}

func TestSliceLines(t *testing.T) {
	lines := []string{"a", "b", "c", "d"} // 1-based: a=1 … d=4
	cases := []struct {
		start, end int
		want       string
	}{
		{1, 2, "a\nb"},
		{2, 4, "b\nc\nd"},
		{3, 3, "c"},
		{0, 2, ""},      // invalid start
		{3, 2, ""},      // end < start
		{9, 9, ""},      // out of range
		{3, 99, "c\nd"}, // end clamped to len
	}
	for _, tc := range cases {
		if got := sliceLines(lines, tc.start, tc.end); got != tc.want {
			t.Errorf("sliceLines(%d,%d) = %q; want %q", tc.start, tc.end, got, tc.want)
		}
	}
}

// TestBodiesForResult_NoStore: with no store, the producer is a clean no-op.
func TestBodiesForResult_NoStore(t *testing.T) {
	c := &Component{logger: slog.Default()}
	if got := c.bodiesForResult(context.Background(), &semsourceast.ParseResult{Path: "x.go"}, "/tmp"); got != nil {
		t.Fatalf("no store should yield nil, got %+v", got)
	}
}
