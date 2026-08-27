// Package s3store provides an S3-compatible implementation of storage.Store.
// Keys map to object keys under a single configured bucket. The endpoint,
// addressing style, and region are all explicit, so a self-hosted store
// (Garage, MinIO) is a first-class target rather than a deviation from AWS.
//
// Credentials never appear in configuration. They are read from the process
// environment once, at construction — see New.
package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/c360studio/semstreams/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Compile-time assertions that Store satisfies the storage interfaces. The
// streaming one is the stronger claim and implies the other, but both are
// asserted so a lost method names the contract it broke.
var (
	_ storage.Store           = (*Store)(nil)
	_ storage.StreamableStore = (*Store)(nil)
)

// Credential environment variables, consulted in the order listed.
//
// The SEMSOURCE_-prefixed names are the documented ones, matching
// SEMSOURCE_API_TOKEN and SEMSOURCE_GIT_TOKEN. The AWS_ names are honored as a
// fallback because a host already configured for the aws CLI, rclone, or mc
// has them set, and requiring the same secret under a second name invites the
// copy that goes stale.
const (
	EnvAccessKeyID     = "SEMSOURCE_S3_ACCESS_KEY_ID"
	EnvSecretAccessKey = "SEMSOURCE_S3_SECRET_ACCESS_KEY"
	EnvSessionToken    = "SEMSOURCE_S3_SESSION_TOKEN"

	envAWSAccessKeyID     = "AWS_ACCESS_KEY_ID"
	envAWSSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	envAWSSessionToken    = "AWS_SESSION_TOKEN"
)

// Store implements storage.StreamableStore against an S3-compatible endpoint.
// All operations are safe for concurrent use from multiple goroutines.
type Store struct {
	client   *minio.Client
	bucket   string
	endpoint string // as resolved, for error text and status lines
}

// New creates a Store for cfg, resolving credentials from the process
// environment at construction and holding them for the client's lifetime.
//
// Construction does not reach the network: an unreachable endpoint or a wrong
// credential surfaces on the first operation, not here, so a store that is
// temporarily down cannot stop the service from starting. Missing credentials
// do fail here, because that is a deployment mistake no retry fixes.
func New(cfg Config) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("s3store: %w", err)
	}

	raw := cfg.Endpoint
	if raw == "" {
		raw = DefaultEndpoint
	}
	ep, err := parseEndpoint(raw)
	if err != nil {
		return nil, fmt.Errorf("s3store: endpoint %q: %w", raw, err)
	}

	creds, err := credentialsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("s3store: %w", err)
	}

	lookup := minio.BucketLookupDNS
	if cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}

	client, err := minio.New(ep.host, &minio.Options{
		Creds:        creds,
		Secure:       ep.secure,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("s3store: client for endpoint %q: %w", raw, err)
	}

	return &Store{client: client, bucket: cfg.Bucket, endpoint: raw}, nil
}

// Bucket returns the bucket this store resolves keys under.
func (s *Store) Bucket() string { return s.bucket }

// Endpoint returns the resolved endpoint URL.
func (s *Store) Endpoint() string { return s.endpoint }

// String identifies the store by endpoint and bucket — the two values that
// belong in a log line or a status entry, and the only two there are to leak.
func (s *Store) String() string {
	return fmt.Sprintf("s3store(endpoint=%s bucket=%s)", s.endpoint, s.bucket)
}

// Close is a no-op for the S3 backend but satisfies the lifecycle patterns
// other storage implementations use.
func (s *Store) Close() error {
	return nil
}

// Put writes data to the given key, overwriting any object already there.
func (s *Store) Put(ctx context.Context, key string, data []byte) error {
	if err := checkKey(key); err != nil {
		return fmt.Errorf("s3store: Put: %w", err)
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{})
	if err != nil {
		return s.opErr("Put", key, classify(err))
	}
	return nil
}

// Get retrieves the data stored at key. When the key holds no object the error
// wraps storage.ErrObjectNotFound.
//
// It issues one GET rather than reusing Open, which resolves the object with a
// HEAD first. The ingest path fetches every changed object in a prefix, so the
// difference is one round trip or two per document.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, fmt.Errorf("s3store: Get: %w", err)
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, s.opErr("Get", key, classify(err))
	}
	defer obj.Close() //nolint:errcheck // read-only; a close error cannot corrupt anything

	// GetObject is lazy: the request goes out on the first read, so this is
	// where a missing key surfaces. The response carries an error body, which
	// is what lets classify take the code at its word.
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, s.opErr("Get", key, classify(err))
	}
	return data, nil
}

// Open returns a streaming reader for the object at key. The caller must close
// it. When the key holds no object the error wraps storage.ErrObjectNotFound.
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, fmt.Errorf("s3store: Open: %w", err)
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, s.opErr("Open", key, classify(err))
	}
	// Nothing has been requested yet — GetObject is lazy — so a missing key
	// would otherwise surface as a read error somewhere far from the call that
	// asked for it. Stat forces the request now, which is what makes the
	// not-found sentinel meaningful at the point of Open.
	if _, err := obj.Stat(); err != nil {
		obj.Close() //nolint:errcheck // the object is already being discarded
		return nil, s.opErr("Open", key, s.classifyStat(ctx, err))
	}
	return obj, nil
}

// ObjectInfo is one object as a listing described it: enough to decide whether
// it changed and whether it is worth fetching, without fetching it.
//
// ETag is a change token, never a content hash. For multipart uploads S3 and
// Garage both return a composite "<hash>-<partcount>" that is not the MD5 of
// the object, so anything that needs the content's hash must compute it from
// the bytes.
type ObjectInfo struct {
	Key          string
	ETag         string
	Size         int64
	LastModified time.Time
}

// Objects returns metadata for every object under the given prefix, in
// lexicographic order by key. An empty prefix matches every object in the
// bucket, and the returned slice is non-nil even when nothing matches.
//
// It either consumes the listing to its end or returns an error — never a
// partial answer. See List.
func (s *Store) Objects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	return s.objects(ctx, "Objects", prefix)
}

// List returns every key under the given prefix in lexicographic order. An
// empty prefix matches every key in the bucket. The returned slice is non-nil
// even when nothing matches.
//
// The listing is consumed to its end: the client walks continuation pages
// internally, and a failure part way through arrives as an error on the
// channel. That error is returned rather than the keys collected so far,
// because a partial listing that reads as a complete one is how a transient
// fault turns into a conclusion that objects no longer exist.
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	infos, err := s.objects(ctx, "List", prefix)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(infos))
	for _, info := range infos {
		keys = append(keys, info.Key)
	}
	return keys, nil
}

// objects is the single pagination implementation both listings share, so
// there is only one place where consuming every page can go wrong.
func (s *Store) objects(ctx context.Context, op, prefix string) ([]ObjectInfo, error) {
	infos := []ObjectInfo{}
	for info := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if info.Err != nil {
			return nil, s.opErr(op, prefix, classify(info.Err))
		}
		infos = append(infos, ObjectInfo{
			Key:          info.Key,
			ETag:         info.ETag,
			Size:         info.Size,
			LastModified: info.LastModified,
		})
	}

	// S3 lists lexicographically, but the interface promises the order and
	// this store's whole point is talking to implementations that are not S3.
	// Sorting an already-sorted slice is the cheap half of that bargain.
	sort.Slice(infos, func(i, j int) bool { return infos[i].Key < infos[j].Key })
	return infos, nil
}

// Delete removes the object at key. It is idempotent: deleting a key that
// holds no object is not an error.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return fmt.Errorf("s3store: Delete: %w", err)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return s.opErr("Delete", key, classify(err))
	}
	return nil
}

// opErr wraps a failure with what an operator needs to act on it: which store,
// which key, and the cause. A message that says only "access denied" makes
// whoever reads it guess which of several configured sources produced it.
func (s *Store) opErr(op, key string, err error) error {
	return fmt.Errorf("s3store %s bucket %q: %s %q: %w", s.endpoint, s.bucket, op, key, err)
}

// checkKey rejects keys the store cannot address at all.
func checkKey(key string) error {
	if key == "" {
		return errors.New("key must not be empty")
	}
	return nil
}

// classify translates a client error into the backend-agnostic sentinel
// callers match on, for operations whose error responses carry a body — the
// listing, where the store names the fault itself.
//
// Only a missing key maps to storage.ErrObjectNotFound. A missing bucket
// deliberately does not: it is a deployment fault every key in the store
// shares, and reporting it as "that one object is absent" is how a typo in the
// bucket name reads as a corpus that is legitimately empty.
func classify(err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) && resp.Code == minio.NoSuchKey {
		return fmt.Errorf("%w: %s", storage.ErrObjectNotFound, resp.Message)
	}
	return err
}

// classifyStat is classify for an error from resolving an object with HEAD,
// which cannot take the response at its word.
//
// A HEAD response carries no body, so a 404 arrives with nothing in it to say
// what was missing, and the client fills in NoSuchKey because an object is
// what it asked for. A wrong bucket produces exactly that answer for every key
// in it. Every other operation here reads an error body and needs none of
// this.
//
// One extra request settles which it was, on the error path only. When even
// that is inconclusive — a store that answers the bucket check with a denial
// rather than a verdict — the not-found sentinel still applies: it is what the
// response said, and inventing a bucket fault from a permission error would
// trade one wrong answer for another.
func (s *Store) classifyStat(ctx context.Context, err error) error {
	var resp minio.ErrorResponse
	if !errors.As(err, &resp) || resp.Code != minio.NoSuchKey {
		return classify(err)
	}
	if exists, bucketErr := s.client.BucketExists(ctx, s.bucket); bucketErr == nil && !exists {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return fmt.Errorf("%w: %s", storage.ErrObjectNotFound, resp.Message)
}

// credentialsFromEnv reads static credentials from the environment.
//
// Errors name the variables and never their values — this error text reaches
// logs and status surfaces.
func credentialsFromEnv() (*credentials.Credentials, error) {
	id := firstEnv(EnvAccessKeyID, envAWSAccessKeyID)
	secret := firstEnv(EnvSecretAccessKey, envAWSSecretAccessKey)
	token := firstEnv(EnvSessionToken, envAWSSessionToken)

	switch {
	case id == "" && secret == "":
		return nil, fmt.Errorf("no credentials in the environment: set %s and %s (or %s and %s)",
			EnvAccessKeyID, EnvSecretAccessKey, envAWSAccessKeyID, envAWSSecretAccessKey)
	case id == "":
		return nil, fmt.Errorf("a secret key is set but no access key: also set %s (or %s)",
			EnvAccessKeyID, envAWSAccessKeyID)
	case secret == "":
		return nil, fmt.Errorf("an access key is set but no secret key: also set %s (or %s)",
			EnvSecretAccessKey, envAWSSecretAccessKey)
	}

	return credentials.NewStaticV4(id, secret, token), nil
}

// firstEnv returns the value of the first name that is set to a non-empty
// value, or the empty string when none is.
func firstEnv(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}
