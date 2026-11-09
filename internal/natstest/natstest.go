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
	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/go-connections/nat"
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

	container, url, err := start(t.Context(), dockerLauncher(cfg), portWindow)
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

// broker is the part of a testcontainers.Container this package uses, so the
// setup and cleanup paths can be tested without Docker.
type broker interface {
	Host(ctx context.Context) (string, error)
	MappedPort(ctx context.Context, port nat.Port) (nat.Port, error)
	Terminate(ctx context.Context, opts ...testcontainers.TerminateOption) error
}

// launcher creates and starts one broker container. A failed start may still
// return a container to remove.
type launcher func(ctx context.Context) (broker, error)

// portWindow bounds how long a started container's client port may stay
// unpublished before the container is replaced: Docker can report a container
// running before its port binding is visible (gh#736).
const portWindow = 10 * time.Second

func dockerLauncher(cfg config) launcher {
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
	return func(ctx context.Context) (broker, error) {
		container, err := testcontainers.GenericContainer(ctx, req)
		if container == nil {
			// Keep a nil Container a nil broker, so terminate skips it.
			return nil, err
		}
		return container, err
	}
}

// start launches a broker and resolves its client URL. Each attempt, pull and
// readiness included, is bounded by startTimeout. A container is replaced
// once, and only when its client port was observed absent for the whole
// window, the container was removed cleanly, and the test is still running.
func start(ctx context.Context, launch launcher, window time.Duration) (broker, string, error) {
	var failures error
	for attempt := 1; attempt <= 2; attempt++ {
		container, url, portMissing, err := attemptStart(ctx, launch, window)
		if err == nil {
			return container, url, nil
		}
		err = fmt.Errorf("attempt %d: %w", attempt, err)
		termErr := terminate(container)
		failures = errors.Join(failures, err, termErr)
		if ctx.Err() != nil {
			// The test ended, possibly during cleanup: say so, and stop.
			return nil, "", errors.Join(failures, context.Cause(ctx))
		}
		// A broker that could not be removed must not be hidden behind a
		// successful replacement.
		if termErr != nil || !portMissing {
			break
		}
	}
	return nil, "", failures
}

// attemptStart reports portMissing only when the container started and its
// host resolved, but the client port stayed unpublished for the window.
func attemptStart(ctx context.Context, launch launcher, window time.Duration) (broker, string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	container, err := launch(ctx)
	if err != nil {
		return container, "", false, fmt.Errorf("start container: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return container, "", false, fmt.Errorf("container host: %w", err)
	}

	port, absent, err := observePort(ctx, container, window)
	if err != nil {
		return container, "", absent, err
	}
	return container, fmt.Sprintf("nats://%s:%s", host, port.Port()), false, nil
}

// observePort polls the client port until it is published or the window
// ends. absent is true only when the port was seen unpublished and the window
// itself ran out; an error from the attempt's own context (startTimeout or the
// test ending) carries that cause and is never treated as absence.
func observePort(ctx context.Context, container broker, window time.Duration) (nat.Port, bool, error) {
	observe, stop := context.WithTimeout(ctx, window)
	defer stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	var lastAbsent error
	for {
		port, err := container.MappedPort(observe, clientPort)
		switch {
		case err == nil:
			return port, false, nil
		case ctx.Err() != nil:
			return "", false, fmt.Errorf("mapped port %s: %w", clientPort, errors.Join(context.Cause(ctx), err))
		case cerrdefs.IsNotFound(err):
			lastAbsent = err
		case observe.Err() == nil:
			return "", false, fmt.Errorf("mapped port %s: %w", clientPort, err)
		}
		// A call cut short by the window's own deadline falls through here.

		if observe.Err() == nil {
			select {
			case <-tick.C:
				continue
			case <-observe.Done():
			}
		}
		if ctx.Err() != nil {
			return "", false, fmt.Errorf("mapped port %s: %w", clientPort, errors.Join(context.Cause(ctx), lastAbsent))
		}
		if lastAbsent == nil {
			return "", false, fmt.Errorf("mapped port %s: no observation within %s", clientPort, window)
		}
		return "", true, fmt.Errorf("mapped port %s not published within %s: %w", clientPort, window, lastAbsent)
	}
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
func cleanup(client *natsclient.Client, container broker) error {
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

func terminate(container broker) error {
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
