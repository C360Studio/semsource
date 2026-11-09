// Package natstest provisions a NATS broker for integration tests and hands
// back a connected natsclient.Client.
//
// SemSource owns this fixture: SemStreams' natsclient.NewTestClient is not
// carried into SemEngine, and SemEngine ruled its own harness repo policy, not
// consumer surface (semengine#568). Only the natsclient import path should
// change at the port. The surface is exactly what the integration suites use
// (#228): JetStream on or off, pre-created bounded streams, the client and the
// broker URL.
//
// shortcut: one broker per test, as natsclient.NewTestClient did; SemEngine
// shares one per package and empties it between tests. Switch when integration
// lane time matters more than isolation by construction.
package natstest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Image is the NATS server integration tests run against. It is the image the
// SETUP 03A lane pins (2.14.4, the frozen pin's server line); SemEngine pins
// nats:2.15.0-alpine, so this moves with the port.
const Image = "nats:2.14.4-alpine@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66"

const (
	clientPort     = "4222/tcp"
	startTimeout   = 30 * time.Second
	clientTimeout  = 5 * time.Second
	cleanupTimeout = 30 * time.Second

	// Every stream this package creates is bounded, so a runaway test fails
	// loudly instead of filling the account, and the suites keep exercising
	// the bounds contract production streams carry.
	streamMaxAge   = time.Hour
	streamMaxBytes = 64 << 20
)

// Stream is a JetStream stream created before the test runs.
type Stream struct {
	Name     string
	Subjects []string
}

type config struct {
	jetstream bool
	streams   []Stream
}

// Option configures the broker.
type Option func(*config)

// WithJetStream enables JetStream (and so KV and object stores).
func WithJetStream() Option {
	return func(c *config) { c.jetstream = true }
}

// WithStreams enables JetStream and creates the given streams.
func WithStreams(streams ...Stream) Option {
	return func(c *config) {
		c.jetstream = true
		c.streams = append(c.streams, streams...)
	}
}

// Server is a running broker and a client connected to it.
type Server struct {
	Client *natsclient.Client
	URL    string
}

// New starts a broker for this test, connects a client, creates the requested
// streams, and registers cleanup. It fails the test on any setup error.
func New(t testing.TB, opts ...Option) *Server {
	t.Helper()

	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	container, url, err := start(t.Context(), cfg)
	if err != nil {
		t.Fatalf("start NATS: %v", err)
	}
	client, err := connect(t.Context(), url)
	if err != nil {
		t.Fatalf("connect NATS: %v", errors.Join(err, terminate(container)))
	}
	t.Cleanup(func() {
		if err := cleanup(client, container); err != nil {
			t.Errorf("clean up NATS: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), clientTimeout)
	defer cancel()
	for _, s := range cfg.streams {
		if _, err := client.EnsureStream(ctx, jetstream.StreamConfig{
			Name:     s.Name,
			Subjects: s.Subjects,
			MaxAge:   streamMaxAge,
			MaxBytes: streamMaxBytes,
			Discard:  jetstream.DiscardOld,
		}); err != nil {
			t.Fatalf("create stream %s: %v", s.Name, err)
		}
	}

	return &Server{Client: client, URL: url}
}

// start runs the container and resolves its client URL. A container whose
// mapped port cannot be read (Docker reporting it running before the port
// binding is visible, gh#736) is replaced once.
func start(ctx context.Context, cfg config) (testcontainers.Container, string, error) {
	args := []string{"--port", "4222"}
	if cfg.jetstream {
		args = append(args, "--js")
	}
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        Image,
			ExposedPorts: []string{clientPort},
			Cmd:          args,
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(startTimeout),
		},
		Started: true,
	}

	var failures error
	for attempt := 1; attempt <= 2; attempt++ {
		container, err := testcontainers.GenericContainer(ctx, req)
		if err != nil {
			// A failed start may still return a container to remove.
			return nil, "", errors.Join(failures, err, terminate(container))
		}
		url, err := clientURL(ctx, container)
		if err == nil {
			return container, url, nil
		}
		failures = errors.Join(failures, fmt.Errorf("attempt %d: %w", attempt, err), terminate(container))
	}
	return nil, "", failures
}

func clientURL(ctx context.Context, container testcontainers.Container) (string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("container host: %w", err)
	}
	port, err := container.MappedPort(ctx, clientPort)
	if err != nil {
		return "", fmt.Errorf("mapped port %s: %w", clientPort, err)
	}
	return fmt.Sprintf("nats://%s:%s", host, port.Port()), nil
}

// connect builds the client the way production does (NewClient, Connect),
// with no reconnects and no health ticker so a dead broker fails fast.
func connect(ctx context.Context, url string) (*natsclient.Client, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(clientTimeout),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, clientTimeout)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("connect: %w", err), closeClient(client))
	}
	if err := client.WaitForConnection(connectCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("connection not ready: %w", err), closeClient(client))
	}
	return client, nil
}

// cleanup closes the client before removing the broker, so the client's
// drain does not race a vanished server. Each step gets a fresh deadline: the
// test context is already done when cleanup runs.
func cleanup(client *natsclient.Client, container testcontainers.Container) error {
	return errors.Join(closeClient(client), terminate(container))
}

func closeClient(client *natsclient.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		return fmt.Errorf("close client: %w", err)
	}
	return nil
}

func terminate(container testcontainers.Container) error {
	if container == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if err := container.Terminate(ctx); err != nil {
		return fmt.Errorf("terminate container: %w", err)
	}
	return nil
}
