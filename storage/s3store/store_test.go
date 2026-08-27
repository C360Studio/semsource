package s3store_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semstreams/storage"

	"github.com/c360studio/semsource/storage/s3store"
)

// Example credentials from the AWS documentation. They are recognizable as
// fake, which matters in a file whose job is asserting they never come out.
const (
	testAccessKeyID     = "AKIAIOSFODNN7EXAMPLE"
	testSecretAccessKey = "wJalrXUtnFEMI-K7MDENG-bPxRfiCYEXAMPLEKEY"
)

// setTestCredentials puts credentials in the environment and clears every name
// New would otherwise fall back to, so a developer's real AWS keys cannot
// change what these tests exercise.
func setTestCredentials(t *testing.T) {
	t.Helper()
	t.Setenv(s3store.EnvAccessKeyID, testAccessKeyID)
	t.Setenv(s3store.EnvSecretAccessKey, testSecretAccessKey)
	t.Setenv(s3store.EnvSessionToken, "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
}

// fakeS3 stands in for the object store, routing bucket-level requests apart
// from object-level ones because the client uses the first to make sense of
// the second: a HEAD on an object that 404s says nothing about *what* was
// missing, so the store asks whether the bucket is there before deciding.
type fakeS3 struct {
	// bucketMissing answers the existence check with a 404, standing in for a
	// bucket that is not there — a typo in the name, or a deployment that was
	// never provisioned.
	bucketMissing bool

	// object answers everything else: object requests and listings.
	object http.HandlerFunc
}

func (f fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path-style addressing puts the bucket first: "/artifacts/" is the
	// bucket, "/artifacts/report.md" an object in it.
	bucketLevel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/artifacts"), "/") == ""
	if bucketLevel && r.Method == http.MethodHead {
		if f.bucketMissing {
			s3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	f.object(w, r)
}

// newFakeStore points a Store at an in-process fake.
//
// A fake server rather than a container because these cases are about how the
// client behaves when a *response* goes a particular way — a 404, a 403, a
// listing that fails on its second page. Provoking those against a real server
// means arranging state; here they are the fixture. What the fake cannot prove
// is that the client speaks S3 on the wire, which is what the MinIO-backed
// integration test is for.
func newFakeStore(t *testing.T, fake fakeS3) *s3store.Store {
	t.Helper()
	setTestCredentials(t)

	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	store, err := s3store.New(s3store.Config{
		Bucket:   "artifacts",
		Endpoint: srv.URL,
		// An explicit region keeps the client from issuing a bucket-location
		// lookup before every request, so each test sees only its own calls.
		Region:    "us-east-1",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	return store
}

// s3Error writes an S3 XML error response.
func s3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<Error><Code>%s</Code><Message>%s</Message>`+
		`<Resource>/artifacts</Resource><RequestId>test</RequestId></Error>`, code, message)
}

// listPage writes a ListObjectsV2 response holding one key, truncated or not.
func listPage(w http.ResponseWriter, key, continuation string) {
	truncated := continuation != ""
	next := ""
	if truncated {
		next = `<NextContinuationToken>` + continuation + `</NextContinuationToken>`
	}
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
		`<Name>artifacts</Name><Prefix></Prefix><KeyCount>1</KeyCount><MaxKeys>1000</MaxKeys>`+
		`<IsTruncated>%t</IsTruncated>%s`+
		`<Contents><Key>%s</Key><LastModified>2026-01-01T00:00:00.000Z</LastModified>`+
		`<ETag>&#34;d41d8cd98f00b204e9800998ecf8427e&#34;</ETag><Size>3</Size>`+
		`<StorageClass>STANDARD</StorageClass></Contents>`+
		`</ListBucketResult>`, truncated, next, key)
}

// ─── Construction ───────────────────────────────────────────────────────────

func TestNew_RequiresCredentials(t *testing.T) {
	for _, name := range []string{
		s3store.EnvAccessKeyID, s3store.EnvSecretAccessKey, s3store.EnvSessionToken,
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
	} {
		t.Setenv(name, "")
	}

	_, err := s3store.New(s3store.Config{Bucket: "artifacts"})
	if err == nil {
		t.Fatal("expected an error when the environment holds no credentials")
	}
	// The message has to say which variables to set, since nothing in the
	// configuration document hints at them.
	for _, name := range []string{s3store.EnvAccessKeyID, s3store.EnvSecretAccessKey} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error should name %s, got: %v", name, err)
		}
	}
}

func TestNew_RequiresBothHalvesOfACredential(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		secret string
	}{
		{"access key without secret", testAccessKeyID, ""},
		{"secret without access key", "", testSecretAccessKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(s3store.EnvAccessKeyID, tc.id)
			t.Setenv(s3store.EnvSecretAccessKey, tc.secret)
			t.Setenv("AWS_ACCESS_KEY_ID", "")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "")

			_, err := s3store.New(s3store.Config{Bucket: "artifacts"})
			if err == nil {
				t.Fatal("expected an error for a half-configured credential")
			}
			if strings.Contains(err.Error(), testSecretAccessKey) {
				t.Errorf("error leaked the secret: %v", err)
			}
		})
	}
}

func TestNew_RejectsInvalidConfig(t *testing.T) {
	setTestCredentials(t)

	if _, err := s3store.New(s3store.Config{}); err == nil {
		t.Error("expected an error for a config with no bucket")
	}
	if _, err := s3store.New(s3store.Config{Bucket: "artifacts", Endpoint: "localhost:9000"}); err == nil {
		t.Error("expected an error for an unparseable endpoint")
	}
}

// TestNew_ResolvesCredentialsFromEnvironment checks which key actually signs
// the request, since that is the only externally visible evidence of which
// environment variable won.
func TestNew_ResolvesCredentialsFromEnvironment(t *testing.T) {
	const awsAccessKeyID = "AKIAI44QH8DHBEXAMPLE"

	cases := []struct {
		name   string
		env    map[string]string
		signer string
	}{
		{
			name: "the semsource names are preferred",
			env: map[string]string{
				s3store.EnvAccessKeyID:     testAccessKeyID,
				s3store.EnvSecretAccessKey: testSecretAccessKey,
				"AWS_ACCESS_KEY_ID":        awsAccessKeyID,
				"AWS_SECRET_ACCESS_KEY":    "aws-secret",
			},
			signer: testAccessKeyID,
		},
		{
			name: "the aws names are the fallback",
			env: map[string]string{
				s3store.EnvAccessKeyID:     "",
				s3store.EnvSecretAccessKey: "",
				"AWS_ACCESS_KEY_ID":        awsAccessKeyID,
				"AWS_SECRET_ACCESS_KEY":    "aws-secret",
			},
			signer: awsAccessKeyID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for name, value := range tc.env {
				t.Setenv(name, value)
			}

			var authorization atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authorization.Store(r.Header.Get("Authorization"))
				listPage(w, "report.md", "")
			}))
			t.Cleanup(srv.Close)

			store, err := s3store.New(s3store.Config{
				Bucket:    "artifacts",
				Endpoint:  srv.URL,
				Region:    "us-east-1",
				PathStyle: true,
			})
			if err != nil {
				t.Fatalf("s3store.New: %v", err)
			}
			if _, err := store.List(t.Context(), ""); err != nil {
				t.Fatalf("List: %v", err)
			}

			got, _ := authorization.Load().(string)
			if !strings.Contains(got, "Credential="+tc.signer+"/") {
				t.Errorf("request signed with the wrong key\n  authorization: %s\n  want credential: %s", got, tc.signer)
			}
			if strings.Contains(got, testSecretAccessKey) {
				t.Error("the secret key reached the wire verbatim")
			}
		})
	}
}

// ─── Error classification ───────────────────────────────────────────────────

// TestMissingKeyIsNotFound covers both routes to the not-found sentinel. Get
// reads the code out of the error body; Open resolves the object with a HEAD,
// whose response has no body at all — Go's server drops it for HEAD exactly as
// a real store does — so it reaches the same verdict a different way.
func TestMissingKeyIsNotFound(t *testing.T) {
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		s3Error(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
	}})

	ctx := t.Context()
	_, getErr := store.Get(ctx, "missing.md")
	if !errors.Is(getErr, storage.ErrObjectNotFound) {
		t.Errorf("Get: expected storage.ErrObjectNotFound, got: %v", getErr)
	}

	r, openErr := store.Open(ctx, "missing.md")
	if openErr == nil {
		r.Close() //nolint:errcheck // only reached when the test is already failing
		t.Fatal("Open: expected an error")
	}
	if !errors.Is(openErr, storage.ErrObjectNotFound) {
		t.Errorf("Open: expected storage.ErrObjectNotFound, got: %v", openErr)
	}
}

// TestMissingBucketIsNotNotFound keeps a deployment fault from reading as an
// absent document. Every key in the store is missing when the bucket is wrong,
// and answering "no such object" for each of them is how a typo in the bucket
// name turns into a corpus that looks legitimately empty.
//
// The two routes fail differently and both have to hold: Get sees NoSuchBucket
// in the error body, while Open's HEAD gets a bare 404 that the client reports
// as NoSuchKey — the case the extra bucket check exists for.
func TestMissingBucketIsNotNotFound(t *testing.T) {
	store := newFakeStore(t, fakeS3{
		bucketMissing: true,
		object: func(w http.ResponseWriter, _ *http.Request) {
			s3Error(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
		},
	})

	ctx := t.Context()
	_, getErr := store.Get(ctx, "report.md")
	if getErr == nil {
		t.Fatal("Get: expected an error")
	}
	if errors.Is(getErr, storage.ErrObjectNotFound) {
		t.Errorf("Get: a missing bucket must not classify as a missing object, got: %v", getErr)
	}

	r, openErr := store.Open(ctx, "report.md")
	if openErr == nil {
		r.Close() //nolint:errcheck // only reached when the test is already failing
		t.Fatal("Open: expected an error")
	}
	if errors.Is(openErr, storage.ErrObjectNotFound) {
		t.Errorf("Open: a missing bucket must not classify as a missing object, got: %v", openErr)
	}
	if !strings.Contains(openErr.Error(), "artifacts") {
		t.Errorf("Open error should name the bucket, got: %v", openErr)
	}
}

func TestOperationErrorsCarryNoSecret(t *testing.T) {
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		s3Error(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
	}})

	ctx := t.Context()
	_, getErr := store.Get(ctx, "report.md")
	_, listErr := store.List(ctx, "")
	putErr := store.Put(ctx, "report.md", []byte("hi"))
	deleteErr := store.Delete(ctx, "report.md")

	for name, err := range map[string]error{
		"Get": getErr, "List": listErr, "Put": putErr, "Delete": deleteErr,
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if strings.Contains(err.Error(), testSecretAccessKey) {
			t.Errorf("%s error leaked the secret key: %v", name, err)
		}
	}
}

// ─── Listing ────────────────────────────────────────────────────────────────

func TestList_ConsumesEveryPage(t *testing.T) {
	var calls atomic.Int32
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			listPage(w, "reports/a.md", "page-2")
			return
		}
		listPage(w, "reports/b.md", "")
	}})

	keys, err := store.List(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"reports/a.md", "reports/b.md"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

// TestList_FailurePartWayThroughReturnsNoKeys is the store-level half of the
// completeness contract: a listing that dies on its second page yields an
// error, never the keys the first page happened to carry. A caller handed
// those partial keys would conclude that everything absent from them had been
// deleted.
func TestList_FailurePartWayThroughReturnsNoKeys(t *testing.T) {
	var calls atomic.Int32
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			listPage(w, "reports/a.md", "page-2")
			return
		}
		s3Error(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
	}})

	keys, err := store.List(t.Context(), "reports/")
	if err == nil {
		t.Fatal("expected an error when the listing fails part way through")
	}
	if keys != nil {
		t.Errorf("a failed listing must return no keys, got: %v", keys)
	}
}

func TestList_NoMatchesReturnsEmptyNotNil(t *testing.T) {
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
			`<Name>artifacts</Name><Prefix>nothing/</Prefix><KeyCount>0</KeyCount>`+
			`<MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`)
	}})

	keys, err := store.List(t.Context(), "nothing/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if keys == nil {
		t.Fatal("expected an empty slice, got nil")
	}
	if len(keys) != 0 {
		t.Errorf("keys = %v, want empty", keys)
	}
}

// ─── Keys and identification ────────────────────────────────────────────────

func TestOperationsRejectEmptyKey(t *testing.T) {
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		t.Error("an empty key must not reach the store")
		s3Error(w, http.StatusBadRequest, "InvalidRequest", "unreachable")
	}})

	ctx := t.Context()
	if err := store.Put(ctx, "", []byte("hi")); err == nil {
		t.Error("Put: expected an error for an empty key")
	}
	if _, err := store.Get(ctx, ""); err == nil {
		t.Error("Get: expected an error for an empty key")
	}
	if _, err := store.Open(ctx, ""); err == nil {
		t.Error("Open: expected an error for an empty key")
	}
	if err := store.Delete(ctx, ""); err == nil {
		t.Error("Delete: expected an error for an empty key")
	}
}

func TestString_IdentifiesByEndpointAndBucket(t *testing.T) {
	setTestCredentials(t)

	store, err := s3store.New(s3store.Config{
		Bucket:    "artifacts",
		Endpoint:  "https://garage.internal:3900",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}

	rendered := store.String()
	for _, want := range []string{"garage.internal:3900", "artifacts"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("String() = %q, should contain %q", rendered, want)
		}
	}
	if strings.Contains(rendered, testSecretAccessKey) || strings.Contains(rendered, testAccessKeyID) {
		t.Errorf("String() leaked a credential: %s", rendered)
	}
	if store.Bucket() != "artifacts" {
		t.Errorf("Bucket() = %q, want %q", store.Bucket(), "artifacts")
	}
	if store.Endpoint() != "https://garage.internal:3900" {
		t.Errorf("Endpoint() = %q", store.Endpoint())
	}
}

// TestNew_DefaultEndpointApplies pins the documented default: a config with no
// endpoint is legal and resolves to AWS.
func TestNew_DefaultEndpointApplies(t *testing.T) {
	setTestCredentials(t)

	store, err := s3store.New(s3store.Config{Bucket: "artifacts"})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	if store.Endpoint() != s3store.DefaultEndpoint {
		t.Errorf("Endpoint() = %q, want %q", store.Endpoint(), s3store.DefaultEndpoint)
	}
}

// TestObjects_CarryChangeMetadata pins what the listing has to bring back.
// Change detection compares these values between passes, so an object whose
// ETag or size failed to survive the parse would re-ingest on every pass.
func TestObjects_CarryChangeMetadata(t *testing.T) {
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		listPage(w, "reports/a.md", "")
	}})

	infos, err := store.Objects(t.Context(), "reports/")
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("Objects returned %d entries, want 1", len(infos))
	}

	got := infos[0]
	if got.Key != "reports/a.md" {
		t.Errorf("Key = %q", got.Key)
	}
	// The client strips the quotes S3 wraps an ETag in; a quoted value here
	// would compare unequal against every later pass.
	if got.ETag != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("ETag = %q, want the unquoted value", got.ETag)
	}
	if got.Size != 3 {
		t.Errorf("Size = %d, want 3", got.Size)
	}
	if got.LastModified.IsZero() {
		t.Error("LastModified is zero — it is the fallback change token when a store sends no ETag")
	}
}

func TestObjects_FailurePartWayThroughReturnsNothing(t *testing.T) {
	var calls atomic.Int32
	store := newFakeStore(t, fakeS3{object: func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			listPage(w, "reports/a.md", "page-2")
			return
		}
		s3Error(w, http.StatusForbidden, "AccessDenied", "Access Denied.")
	}})

	infos, err := store.Objects(t.Context(), "reports/")
	if err == nil {
		t.Fatal("expected an error when the listing fails part way through")
	}
	if infos != nil {
		t.Errorf("a failed listing must return no objects, got: %v", infos)
	}
}
