//go:build garage

package s3store_test

import (
	"bytes"
	"crypto/md5" //nolint:gosec // not a security use: this is the value an ETag must NOT equal
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/c360studio/semstreams/storage"

	"github.com/c360studio/semsource/internal/garagetest"
	"github.com/c360studio/semsource/storage/s3store"
)

// Garage is what the first adopter of the object-store source runs, and the
// MinIO suite next door does not prove it works: two implementations can both
// pass a compatibility matrix and still disagree about the details this store
// reads. These tests cover only where a compatible-but-not-identical
// implementation could plausibly diverge, plus the round trips that would make
// any of the rest meaningless if they failed (#202).
//
// Bucket event notifications and object versioning are deliberately absent.
// Garage implements neither, both are already out of scope in the source's
// design, and a test for a settled non-goal is a test that fails for the wrong
// reason.
//
// This suite carries its own build tag rather than `integration` because
// Garage costs a cluster bootstrap to start. It runs on changes to the store,
// not on every pull request.

// garageStore creates a bucket for this test and returns a Store scoped to it.
func garageStore(t *testing.T) *s3store.Store {
	t.Helper()
	store, _ := garagetest.NewStore(t)
	return store
}

// garagePut is a fixture helper: a failure here is setup going wrong, not the
// behavior under test.
func garagePut(t *testing.T, store *s3store.Store, key string, data []byte) {
	t.Helper()
	if err := store.Put(t.Context(), key, data); err != nil {
		t.Fatalf("Put %q: %v", key, err)
	}
}

// ─── Path-style addressing ──────────────────────────────────────────────────

// TestGarage_PathStyleAddressing_RoundTrip covers the addressing a self-hosted
// store is reached by. Every other test here depends on it, but only this one
// says so, and it checks the half the others cannot: that PathStyle selects
// addressing rather than being decoration.
//
// The same request with the flag off has to fail. Garage resolves a bucket
// from the request path unless an s3_api root_domain is configured, and the
// test container configures none, so a virtual-hosted request either cannot
// resolve bucket.host or is read as naming some other bucket entirely — the
// first path segment. Which of the two depends on how the host resolves
// *.localhost, and neither is the point: a success would mean both stores
// addressed the same bucket and the setting changed nothing.
func TestGarage_PathStyleAddressing_RoundTrip(t *testing.T) {
	store, bucket := garagetest.NewStore(t)

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
			garagePut(t, store, tc.key, tc.data)

			got, err := store.Get(t.Context(), tc.key)
			if err != nil {
				t.Fatalf("Get %q: %v", tc.key, err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Errorf("Get %q = %q, want %q", tc.key, got, tc.data)
			}
		})
	}

	t.Run("virtual-hosted addressing does not silently work", func(t *testing.T) {
		virtual, err := s3store.New(s3store.Config{
			Bucket:    bucket,
			Endpoint:  garagetest.Endpoint(t),
			Region:    garagetest.Region,
			PathStyle: false,
		})
		if err != nil {
			t.Fatalf("s3store.New: %v", err)
		}
		if _, err := virtual.Get(t.Context(), "reports/q3.md"); err == nil {
			t.Error("virtual-hosted addressing reached the same object as path-style — PathStyle is not selecting anything")
		}
	})
}

// ─── Listing completeness ───────────────────────────────────────────────────

// TestGarage_List_AcrossContinuationPages is the one that would silently halve
// a corpus. Enumeration completeness is what the source's retraction safety
// rests on: a listing that stops at the first page reads as "those objects are
// gone". Garage issues its own continuation tokens, so MinIO walking them
// correctly says nothing about this.
func TestGarage_List_AcrossContinuationPages(t *testing.T) {
	store := garageStore(t)
	const total = 1001 // S3 caps a listing response at 1000 keys.

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

	infos, err := store.Objects(t.Context(), "page/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != total {
		t.Errorf("enumerated %d objects, want %d", len(infos), total)
	}
}

func TestGarage_List_PrefixScoped(t *testing.T) {
	store := garageStore(t)
	for _, key := range []string{
		"reports/a.md", "reports/b.md", "reports-archive/c.md", "assets/d.png",
	} {
		garagePut(t, store, key, []byte(key))
	}

	got, err := store.List(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"reports/a.md", "reports/b.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("List = %v, want %v", got, want)
	}
}

// ─── Change detection metadata ──────────────────────────────────────────────

// TestGarage_Objects_CarryChangeMetadata proves the listing brings back what
// change detection compares. Change detection reads the ETag from the listing
// rather than issuing a HEAD per object, so an ETag that arrives quoted here,
// or a size the parse dropped, would make every pass look like a change and
// re-ingest the whole corpus every cycle.
func TestGarage_Objects_CarryChangeMetadata(t *testing.T) {
	store := garageStore(t)
	const key = "reports/q3.md"
	garagePut(t, store, key, []byte("first"))

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

	garagePut(t, store, key, []byte("second"))

	infos, err = store.Objects(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("Objects after overwrite: %v", err)
	}
	if infos[0].ETag == first.ETag {
		t.Errorf("ETag did not change when the content did: %q", infos[0].ETag)
	}
}

// TestGarage_Objects_MultipartETagIsCompositeAndOpaque confirms the assumption
// the design rests on (D6): an ETag is a change token and never a content
// hash. A multipart upload returns "<hash>-<partcount>", where hash is a digest
// over the parts' digests, not over the object. Code that treated the ETag as
// an MD5 would work on every small object and break on the large ones — the
// worst possible failure distribution.
//
// The upload goes through a raw client because the Store has no multipart API;
// what is under test is that a multipart object Garage already holds reads
// back sanely through the listing the source consumes.
func TestGarage_Objects_MultipartETagIsCompositeAndOpaque(t *testing.T) {
	store, bucket := garagetest.NewStore(t)
	client := garagetest.Client(t, credentials.NewStaticV4(garagetest.AccessKey, garagetest.SecretKey, ""))

	// Two parts: S3 requires every part but the last to be at least 5 MiB, so
	// this is the smallest object that is multipart at all.
	const partSize = 5 * 1024 * 1024
	data := bytes.Repeat([]byte("semsource"), (partSize+1024)/9)
	const key = "assets/large.bin"

	if _, err := client.PutObject(t.Context(), bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{PartSize: partSize}); err != nil {
		t.Fatalf("multipart PutObject: %v", err)
	}

	infos, err := store.Objects(t.Context(), "assets/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("Objects returned %d entries, want 1", len(infos))
	}
	info := infos[0]

	if !regexp.MustCompile(`^[0-9a-f]{32}-\d+$`).MatchString(info.ETag) {
		t.Errorf("ETag = %q, want the composite <hash>-<partcount> form", info.ETag)
	}
	sum := md5.Sum(data) //nolint:gosec // asserting the ETag is NOT this value
	if info.ETag == hex.EncodeToString(sum[:]) {
		t.Error("a multipart ETag equal to the content MD5 would make the opaque-token rule untestable")
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size, len(data))
	}

	// The object still reads back whole: a composite ETag describes how it was
	// written, not what can be fetched.
	got, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get a multipart object: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("Get returned %d bytes, want %d", len(got), len(data))
	}
}

// ─── Error shapes ───────────────────────────────────────────────────────────

// TestGarage_AuthFailure_IsAnErrorNotAnEmptyListing is the retraction-safety
// contract stated as a test. A store that answered a rejected credential with
// an empty result instead of an error would tell the source that every
// document in the corpus had been deleted, and the source would retract them.
//
// The bucket is seeded first, so an empty listing is a plausible wrong answer
// rather than an impossible one.
func TestGarage_AuthFailure_IsAnErrorNotAnEmptyListing(t *testing.T) {
	seeded, bucket := garagetest.NewStore(t)
	for _, key := range []string{"reports/a.md", "reports/b.md"} {
		garagePut(t, seeded, key, []byte(key))
	}

	rejected := garageStoreFor(t, bucket, garagetest.AccessKey,
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	t.Run("List", func(t *testing.T) {
		got, err := rejected.List(t.Context(), "reports/")
		if err == nil {
			t.Fatalf("a rejected credential listed %d keys instead of failing — this retracts the corpus", len(got))
		}
		if errors.Is(err, storage.ErrObjectNotFound) {
			t.Errorf("an auth failure must not classify as a missing object, got: %v", err)
		}
	})

	t.Run("Objects", func(t *testing.T) {
		got, err := rejected.Objects(t.Context(), "reports/")
		if err == nil {
			t.Fatalf("a rejected credential enumerated %d objects instead of failing", len(got))
		}
	})

	t.Run("Get", func(t *testing.T) {
		if _, err := rejected.Get(t.Context(), "reports/a.md"); err == nil {
			t.Error("a rejected credential fetched an object")
		}
	})
}

// TestGarage_Get_MissingKey covers the one classification the source acts on.
func TestGarage_Get_MissingKey(t *testing.T) {
	store := garageStore(t)

	if _, err := store.Get(t.Context(), "reports/never-written.md"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected storage.ErrObjectNotFound, got: %v", err)
	}
	r, err := store.Open(t.Context(), "reports/never-written.md")
	if err == nil {
		r.Close()
		t.Fatal("expected an error opening a key that holds no object")
	}
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("Open: expected storage.ErrObjectNotFound, got: %v", err)
	}
}

// TestGarage_MissingBucket_IsNotAMissingObject is the D11 disambiguation
// against a second implementation. A HEAD response carries no body, so a 404
// arrives with nothing in it to say what was missing and the client fills in
// NoSuchKey — meaning a typo in the bucket name looks exactly like a corpus
// that is legitimately empty. The store asks a second question to tell them
// apart, and whether that answer is trustworthy is a property of the server.
func TestGarage_MissingBucket_IsNotAMissingObject(t *testing.T) {
	store := garageStoreFor(t, "bucket-that-was-never-created", garagetest.AccessKey, garagetest.SecretKey)

	// Open is the HEAD path, where the ambiguity actually arises.
	r, err := store.Open(t.Context(), "reports/q3.md")
	if err == nil {
		r.Close()
		t.Fatal("expected an error")
	}
	if errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("Open: a missing bucket must not classify as a missing object, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bucket-that-was-never-created") {
		t.Errorf("error should name the bucket, got: %v", err)
	}

	if _, err := store.Get(t.Context(), "reports/q3.md"); errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("Get: a missing bucket must not classify as a missing object, got: %v", err)
	}
}

// ─── Deletion ───────────────────────────────────────────────────────────────

func TestGarage_Delete(t *testing.T) {
	store := garageStore(t)
	const key = "reports/q3.md"
	garagePut(t, store, key, []byte("content"))

	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(t.Context(), key); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Errorf("expected the object to be gone, got: %v", err)
	}
	if err := store.Delete(t.Context(), "reports/never-written.md"); err != nil {
		t.Errorf("deleting an absent key should be a no-op, got: %v", err)
	}
}

// garageStoreFor builds a Store against the shared container with explicit
// credentials, for the tests whose subject is the credential or the bucket
// name rather than the objects.
func garageStoreFor(t *testing.T, bucket, accessKey, secretKey string) *s3store.Store {
	t.Helper()

	endpoint := garagetest.Endpoint(t)
	t.Setenv(s3store.EnvAccessKeyID, accessKey)
	t.Setenv(s3store.EnvSecretAccessKey, secretKey)

	store, err := s3store.New(s3store.Config{
		Bucket:    bucket,
		Endpoint:  endpoint,
		Region:    garagetest.Region,
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	return store
}
