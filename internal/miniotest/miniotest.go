// Package miniotest provisions a MinIO server for integration tests, the way
// natsclient.NewTestClient provisions NATS: one container, started on first
// use, torn down with the process.
//
// MinIO rather than Garage because it containerizes in one step with env-var
// credentials and a live S3 API, so it can gate every PR without adding flake
// surface. That is a deliberately narrower claim than compatibility — MinIO
// passing does not prove Garage works, and closing that gap is tracked in #202.
package miniotest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/c360studio/semsource/storage/s3store"
)

// Image pins the S3 server integration tests run against. It has one home
// here; every suite that needs a bucket reads it from this package rather than
// keeping a copy that drifts.
const Image = "minio/minio:RELEASE.2025-09-07T16-13-09Z"

// Container credentials. MinIO requires a root password of at least eight
// characters and refuses to start otherwise.
const (
	AccessKey = "semsource-test"
	SecretKey = "semsource-test-secret"
)

// Region is set explicitly on every client so no bucket-location lookup
// precedes each request.
const Region = "us-east-1"

// The server is shared across a package's tests and started on first use, so a
// run that touches no bucket pays nothing for it. Each test gets its own
// bucket instead of its own container.
var (
	once      sync.Once
	endpoint  string
	startErr  error
	terminate func(context.Context) error

	bucketCounter atomic.Int32
)

// Endpoint starts the shared MinIO server if it is not already running and
// returns its endpoint URL.
func Endpoint(t *testing.T) string {
	t.Helper()

	once.Do(func() { endpoint, terminate, startErr = start() })
	if startErr != nil {
		t.Fatalf("start MinIO: %v", startErr)
	}
	return endpoint
}

// Terminate stops the shared server. Call it from TestMain after m.Run; it is
// a no-op when no test asked for a bucket.
func Terminate() error {
	if terminate == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return terminate(ctx)
}

// RunTests wraps m.Run so a package gets container teardown with one line in
// TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(miniotest.RunTests(m)) }
//
// It wraps rather than exits so the teardown runs through a normal return
// path — os.Exit would skip it.
func RunTests(m interface{ Run() int }) int {
	code := m.Run()
	if err := Terminate(); err != nil {
		fmt.Printf("terminate the MinIO container: %v\n", err)
	}
	return code
}

// NewBucket creates a bucket named after the running test and returns its
// name.
//
// The bucket is created through the S3 API rather than by reaching into the
// container, so setup exercises the same protocol the tests do.
func NewBucket(t *testing.T) string {
	t.Helper()

	name := bucketName(t.Name())
	client, err := minio.New(strings.TrimPrefix(Endpoint(t), "http://"), &minio.Options{
		Creds:        credentials.NewStaticV4(AccessKey, SecretKey, ""),
		Region:       Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatalf("admin client: %v", err)
	}
	if err := client.MakeBucket(t.Context(), name, minio.MakeBucketOptions{Region: Region}); err != nil {
		t.Fatalf("create bucket %q: %v", name, err)
	}
	return name
}

// NewStore creates a bucket and returns a Store scoped to it, with credentials
// placed in the test's environment.
//
// Path-style addressing is on because the endpoint is an IP address: there is
// no wildcard DNS to make bucket.host resolve, and every request would fail
// without it.
func NewStore(t *testing.T) (*s3store.Store, string) {
	t.Helper()

	bucket := NewBucket(t)
	SetCredentials(t)

	store, err := s3store.New(s3store.Config{
		Bucket:    bucket,
		Endpoint:  Endpoint(t),
		Region:    Region,
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("s3store.New: %v", err)
	}
	return store, bucket
}

// SetCredentials puts the container's credentials in the test's environment,
// which is the only place the store reads them from.
func SetCredentials(t *testing.T) {
	t.Helper()
	t.Setenv(s3store.EnvAccessKeyID, AccessKey)
	t.Setenv(s3store.EnvSecretAccessKey, SecretKey)
}

// bucketName turns a test name into a legal bucket name: DNS labels are
// lowercase, have no underscores, and run 3-63 characters. The counter keeps
// names unique when one test creates several.
func bucketName(testName string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, testName)

	if len(mapped) > 40 {
		mapped = mapped[:40]
	}
	return fmt.Sprintf("%s-%d", strings.Trim(mapped, "-"), bucketCounter.Add(1))
}

func start() (string, func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        Image,
			ExposedPorts: []string{"9000/tcp"},
			Env: map[string]string{
				"MINIO_ROOT_USER":     AccessKey,
				"MINIO_ROOT_PASSWORD": SecretKey,
			},
			Cmd: []string{"server", "/data"},
			// The health endpoint is the honest readiness signal: a mapped
			// port accepts connections before the S3 API answers on it, and
			// waiting on the port alone yields intermittent failures that read
			// as client bugs.
			WaitingFor: wait.ForHTTP("/minio/health/live").
				WithPort("9000/tcp").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		return "", nil, fmt.Errorf("start container: %w", err)
	}
	stop := func(ctx context.Context) error { return container.Terminate(ctx) }

	host, err := container.Host(ctx)
	if err != nil {
		return "", stop, fmt.Errorf("container host: %w", err)
	}
	port, err := container.MappedPort(ctx, "9000/tcp")
	if err != nil {
		return "", stop, fmt.Errorf("mapped port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), stop, nil
}
