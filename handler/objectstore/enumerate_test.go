package objectstore_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/storage/s3store"
)

var modTime = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

// fakeStore is an in-memory stand-in for an object store.
//
// The cases here are about what the source concludes from a listing — one that
// failed part way through, one that over-answers a prefix, one whose metadata
// did not move. Those are the fixture against a fake and would be arranged
// state against a real server; that the client speaks S3 on the wire is proven
// separately, against MinIO, in storage/s3store.
type fakeStore struct {
	objects []s3store.ObjectInfo
	listErr error

	// gets counts body reads. Change detection is supposed to decide
	// everything from listing metadata, so a planning path that grew a body
	// read — or a HEAD — shows up here as a non-zero count.
	gets atomic.Int32
}

func (f *fakeStore) Objects(_ context.Context, prefix string) ([]s3store.ObjectInfo, error) {
	if f.listErr != nil {
		// A listing that fails yields no objects, whatever it had collected.
		return nil, f.listErr
	}
	var out []s3store.ObjectInfo
	for _, info := range f.objects {
		if strings.HasPrefix(info.Key, prefix) {
			out = append(out, info)
		}
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, key string) ([]byte, error) {
	f.gets.Add(1)
	return []byte(key), nil
}

// obj builds listing metadata for a key.
func obj(key, etag string, size int64) s3store.ObjectInfo {
	return s3store.ObjectInfo{Key: key, ETag: etag, Size: size, LastModified: modTime}
}

// ─── Enumeration ────────────────────────────────────────────────────────────

func TestEnumerate_ObservesEveryObject(t *testing.T) {
	store := &fakeStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("reports/b.md", "etag-b", 20),
		obj("reports/nested/c.md", "etag-c", 30),
	}}

	pass, err := objectstore.Enumerate(t.Context(), store, "reports/")
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if pass.Len() != 3 {
		t.Errorf("pass observed %d objects, want 3", pass.Len())
	}
	if pass.Prefix() != "reports/" {
		t.Errorf("Prefix() = %q", pass.Prefix())
	}

	var keys []string
	for _, info := range pass.Objects() {
		keys = append(keys, info.Key)
	}
	want := []string{"reports/a.md", "reports/b.md", "reports/nested/c.md"}
	assertStrings(t, keys, want)
}

// TestEnumerate_FailedListingYieldsNoPass is the shape the completeness
// guarantee rests on. A listing that died has nothing to say about which
// objects exist, so it does not get to produce the value that answers that
// question.
func TestEnumerate_FailedListingYieldsNoPass(t *testing.T) {
	sentinel := errors.New("listing failed on page two")
	store := &fakeStore{listErr: sentinel}

	pass, err := objectstore.Enumerate(t.Context(), store, "reports/")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap the cause, got: %v", err)
	}
	if pass != nil {
		t.Errorf("a failed listing must produce no pass, got: %+v", pass)
	}
}

// TestEnumerate_AuthenticationFailureIsNotAnEmptyPrefix keeps the two apart at
// the only place they could be confused. An expired credential and a bucket
// with nothing in it produce the same *number* of objects; only one of them is
// an answer.
func TestEnumerate_AuthenticationFailureIsNotAnEmptyPrefix(t *testing.T) {
	denied := &fakeStore{listErr: errors.New("AccessDenied")}
	if pass, err := objectstore.Enumerate(t.Context(), denied, "reports/"); err == nil || pass != nil {
		t.Fatalf("an authentication failure must not read as an empty prefix (pass=%v err=%v)", pass, err)
	}

	empty := &fakeStore{}
	pass, err := objectstore.Enumerate(t.Context(), empty, "reports/")
	if err != nil {
		t.Fatalf("an empty prefix is a legitimate answer, got: %v", err)
	}
	if pass == nil || pass.Len() != 0 {
		t.Errorf("expected an empty pass, got %v", pass)
	}
}

// TestEnumerate_ScopesToPrefix pins the source's own promise rather than the
// store's. The fake here answers with more than it was asked for, standing in
// for an S3 implementation that honors the prefix parameter loosely.
func TestEnumerate_ScopesToPrefix(t *testing.T) {
	store := &overAnsweringStore{objects: []s3store.ObjectInfo{
		obj("reports/a.md", "etag-a", 10),
		obj("assets/b.png", "etag-b", 20),
		obj("reports-archive/c.md", "etag-c", 30),
	}}

	pass, err := objectstore.Enumerate(t.Context(), store, "reports/")
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	if pass.Len() != 1 {
		t.Fatalf("pass observed %d objects, want 1", pass.Len())
	}
	if got := pass.Objects()[0].Key; got != "reports/a.md" {
		t.Errorf("observed %q, want only the object under the prefix", got)
	}
}

// overAnsweringStore returns everything it holds, ignoring the prefix.
type overAnsweringStore struct {
	objects []s3store.ObjectInfo
}

func (s *overAnsweringStore) Objects(context.Context, string) ([]s3store.ObjectInfo, error) {
	return s.objects, nil
}

func (s *overAnsweringStore) Get(_ context.Context, key string) ([]byte, error) {
	return []byte(key), nil
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
