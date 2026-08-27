// Package garagetest provisions a Garage server for compatibility tests, the
// way internal/miniotest provisions MinIO: one container, started on first
// use, torn down with the process.
//
// It exists because MinIO passing does not prove Garage works, and Garage is
// what the first adopter runs (#202). The two providers are deliberate
// siblings — same surface, different server — so a suite can be pointed at
// either without learning a second vocabulary.
//
// Garage does not containerize in one step, which is why this is a separate
// package rather than a parameter to miniotest. A Garage daemon starts with no
// cluster layout, and until one is applied it can store nothing while its S3
// port answers HTTP 403 — not a refused connection. A wait strategy keyed on
// the port, or on any HTTP response from it, therefore passes well before the
// store can serve a request, and the failures that follow read as client bugs.
// See start for the sequence that closes that window.
package garagetest

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/c360studio/semsource/storage/s3store"
)

// Image pins the Garage server the compatibility tests run against. Pinned to
// a patch version rather than a moving tag: this suite's whole purpose is to
// say which server was proven, and "latest" names no server at all.
const Image = "dxflrs/garage:v2.3.0"

// Container credentials, imported into the cluster verbatim at bootstrap.
//
// Garage mints random keys with `key create` and prints them, but it also
// accepts `key import`, which lets these be constants here instead of two
// values parsed out of CLI output. The key ID's shape is Garage's: the GK
// prefix followed by 24 hex characters.
const (
	AccessKey = "GK00000000000000000000dead"
	SecretKey = "0000000000000000000000000000000000000000000000000000000000000042"
)

// Region is Garage's own default region name, set to match the s3_region in
// the container's configuration. Garage verifies the region in the request
// signature, so a mismatch here fails to authenticate rather than being
// ignored the way a self-hosted store often ignores the value.
const Region = "garage"

// Ports Garage listens on. The admin port is 3903; 3902 is the bucket-website
// port, and community examples that name it for the admin API are wrong.
const (
	s3Port    = "3900/tcp"
	adminPort = "3903/tcp"
)

// The daemon configuration. Single node, so replication_factor is 1 and the
// rpc_secret only has to exist — it authenticates node-to-node traffic in a
// cluster of one.
const configTOML = `
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1

rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "1799bccfd7411eddcf9ebd316bc1f5287ad12a68094e1c6ac6abde7e6feae1ec"

[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"

[admin]
api_bind_addr = "[::]:3903"
admin_token = "test-admin-token"
`

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

// Endpoint starts the shared Garage server if it is not already running and
// returns its S3 endpoint URL.
func Endpoint(t *testing.T) string {
	t.Helper()

	once.Do(func() { endpoint, terminate, startErr = start() })
	if startErr != nil {
		t.Fatalf("start Garage: %v", startErr)
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

// NewBucket creates a bucket named after the running test and returns its
// name.
//
// The bucket is created through the S3 API rather than with `garage bucket
// create`, so setup exercises the same protocol the tests do. That is what the
// --create-bucket permission granted at bootstrap is for.
func NewBucket(t *testing.T) string {
	t.Helper()

	name := bucketName(t.Name())
	if err := adminClient(t).MakeBucket(t.Context(), name, minio.MakeBucketOptions{Region: Region}); err != nil {
		t.Fatalf("create bucket %q: %v", name, err)
	}
	return name
}

// NewStore creates a bucket and returns a Store scoped to it, with credentials
// placed in the test's environment.
//
// Path-style addressing is on because the endpoint is an IP address: there is
// no wildcard DNS to make bucket.host resolve, and every request would fail
// without it. It is also what a self-hosted Garage deployment uses.
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

// Client returns a raw S3 client for the container, for the few assertions
// that need to reach past the Store — writing a multipart upload the Store has
// no API for, or presenting credentials the Store would refuse to construct
// with.
func Client(t *testing.T, creds *credentials.Credentials) *minio.Client {
	t.Helper()

	client, err := minio.New(strings.TrimPrefix(Endpoint(t), "http://"), &minio.Options{
		Creds:        creds,
		Region:       Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatalf("s3 client: %v", err)
	}
	return client
}

// adminClient is the client bucket creation runs through.
func adminClient(t *testing.T) *minio.Client {
	t.Helper()
	return Client(t, credentials.NewStaticV4(AccessKey, SecretKey, ""))
}

// bucketName turns a test name into a legal bucket name: DNS labels are
// lowercase, have no underscores, and run 3-63 characters. The counter keeps
// names unique when one test creates several.
//
// The same mapping as miniotest's, and deliberately a separate copy: these
// packages share a surface, not an implementation, and importing one from the
// other to save twenty lines would make the MinIO provider a dependency of
// every Garage run.
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

// start brings up a Garage node that can actually serve S3, in the three
// phases its bootstrap requires.
//
// No single wait strategy can express this, because the middle phase is an
// imperative step: the cluster layout has to be assigned and applied, and the
// daemon has to be accepting commands before it can be. So the container's own
// WaitingFor covers only phase one, and the readiness that matters is polled
// after the layout lands.
func start() (string, func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        Image,
			ExposedPorts: []string{s3Port, adminPort},
			Files: []testcontainers.ContainerFile{{
				Reader:            strings.NewReader(configTOML),
				ContainerFilePath: "/etc/garage.toml",
				FileMode:          0o644,
			}},
			// Phase 1: the daemon takes commands. Deliberately not the health
			// endpoint, which is legitimately 503 until a layout exists, and
			// deliberately not the S3 port, which answers 403 in that state.
			WaitingFor: wait.ForExec([]string{"/garage", "node", "id", "-q"}).
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		return "", nil, fmt.Errorf("start container: %w", err)
	}
	stop := func(ctx context.Context) error { return container.Terminate(ctx) }

	// Phase 2: give the single node a role, so the cluster has somewhere to
	// put data, then create the credentials the tests authenticate with.
	if err := bootstrap(ctx, container); err != nil {
		return "", stop, err
	}

	// Phase 3: the honest readiness gate. /health on the admin port is
	// unauthenticated and tracks layout state exactly — 503 before the apply
	// above, 200 once the node is fully operational.
	ready := wait.ForHTTP("/health").
		WithPort(adminPort).
		WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
		WithStartupTimeout(time.Minute)
	if err := ready.WaitUntilReady(ctx, container); err != nil {
		return "", stop, fmt.Errorf("garage never became operational: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return "", stop, fmt.Errorf("container host: %w", err)
	}
	port, err := container.MappedPort(ctx, s3Port)
	if err != nil {
		return "", stop, fmt.Errorf("mapped port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), stop, nil
}

// bootstrap runs the cluster and credential setup Garage has no environment
// variables for.
func bootstrap(ctx context.Context, container testcontainers.Container) error {
	// `node id -q` prints "<hex>@<addr>"; the layout commands want the hex.
	id, err := run(ctx, container, "/garage", "node", "id", "-q")
	if err != nil {
		return fmt.Errorf("read the node id: %w", err)
	}
	nodeID, _, _ := strings.Cut(strings.TrimSpace(id), "@")
	if nodeID == "" {
		return fmt.Errorf("no node id in %q", id)
	}

	steps := [][]string{
		{"/garage", "layout", "assign", nodeID, "-z", "local", "-c", "1G"},
		{"/garage", "layout", "apply", "--version", "1"},
		{"/garage", "key", "import", "--yes", "-n", "semsource-test", AccessKey, SecretKey},
		// Lets the tests create their buckets over S3 rather than by reaching
		// into the container.
		{"/garage", "key", "allow", "--create-bucket", AccessKey},
	}
	for _, step := range steps {
		if _, err := run(ctx, container, step...); err != nil {
			return fmt.Errorf("bootstrap %q: %w", strings.Join(step[1:], " "), err)
		}
	}
	return nil
}

// run executes a command in the container and returns its combined output,
// treating a non-zero exit as an error with that output attached — Garage
// reports what it disliked on stdout, so dropping it leaves only "exit 1".
//
// Multiplexed demultiplexes Docker's stream framing. Without it the output
// arrives with an 8-byte header per frame, which is invisible in a log line
// and turns the node id into an argument Garage rejects.
func run(ctx context.Context, container testcontainers.Container, cmd ...string) (string, error) {
	code, reader, err := container.Exec(ctx, cmd, tcexec.Multiplexed())
	if err != nil {
		return "", err
	}
	out := new(strings.Builder)
	if reader != nil {
		if _, copyErr := io.Copy(out, reader); copyErr != nil {
			return "", fmt.Errorf("read command output: %w", copyErr)
		}
	}
	if code != 0 {
		return out.String(), fmt.Errorf("exit %d: %s", code, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}
