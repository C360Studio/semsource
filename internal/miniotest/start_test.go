package miniotest

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type fixtureContainer struct {
	testcontainers.Container
	stop             func(context.Context) error
	hostErr, portErr error
}

func (c *fixtureContainer) Terminate(ctx context.Context, _ ...testcontainers.TerminateOption) error {
	return c.stop(ctx)
}
func (c *fixtureContainer) Host(context.Context) (string, error) { return "localhost", c.hostErr }
func (c *fixtureContainer) MappedPort(context.Context, nat.Port) (nat.Port, error) {
	return "9000/tcp", c.portErr
}

func TestFailedStartCleansContainerWithIndependentContext(t *testing.T) {
	for _, stage := range []string{"create", "host", "port"} {
		for _, cleanupFails := range []bool{false, true} {
			t.Run(stage+map[bool]string{false: "/clean", true: "/cleanup_error"}[cleanupFails], func(t *testing.T) {
				startupFailure, cleanupFailure := errors.New("startup failed"), errors.New("cleanup failed")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				container := &fixtureContainer{stop: func(cleanupCtx context.Context) error {
					assertCleanupContext(cleanupCtx, t)
					calls++
					if cleanupFails && calls == 1 {
						return cleanupFailure
					}
					return nil
				}}
				if stage == "host" {
					container.hostErr = startupFailure
				}
				if stage == "port" {
					container.portErr = startupFailure
				}
				endpoint, stop, err := startWithCreator(ctx, func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
					cancel() // Cleanup must outlive the failed startup budget.
					if stage == "create" {
						return container, startupFailure
					}
					return container, nil
				})
				if endpoint != "" || !errors.Is(err, startupFailure) || calls != 1 {
					t.Fatalf("endpoint=%q err=%v cleanup calls=%d", endpoint, err, calls)
				}
				if cleanupFails {
					if !errors.Is(err, cleanupFailure) || stop == nil {
						t.Fatalf("lost cleanup error or retry handle: %v", err)
					}
					retryCtx, retryCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer retryCancel()
					if err := stop(retryCtx); err != nil || calls != 2 {
						t.Fatalf("cleanup retry=%v calls=%d", err, calls)
					}
				} else if stop != nil {
					t.Fatal("successful failed-start cleanup retained a terminated container")
				}
			})
		}
	}
}

func TestStartWithoutContainerPreservesCreationError(t *testing.T) {
	failure := errors.New("cannot create container")
	endpoint, stop, err := startWithCreator(context.Background(), func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
		return nil, failure
	})
	if endpoint != "" || stop != nil || !errors.Is(err, failure) {
		t.Fatalf("start=%q stop=%t err=%v", endpoint, stop != nil, err)
	}
}

func TestStartUsesPinnedReleaseAndLiveHealthGate(t *testing.T) {
	const pinnedRelease = "quay.io/thanos/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
	calls := 0
	container := &fixtureContainer{stop: func(context.Context) error { calls++; return nil }}
	endpoint, stop, err := startWithCreator(context.Background(), func(_ context.Context, req testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
		if req.Image != pinnedRelease || !req.Started {
			t.Fatalf("fixture acquisition: image=%q started=%t", req.Image, req.Started)
		}
		health, ok := req.WaitingFor.(*wait.HTTPStrategy)
		if !ok {
			t.Fatalf("readiness must wait for the HTTP health endpoint: %T", req.WaitingFor)
		}
		if health.Path != "/minio/health/live" || health.Port != "9000/tcp" || health.Timeout() == nil || *health.Timeout() != 2*time.Minute || !health.StatusCodeMatcher(http.StatusOK) || health.StatusCodeMatcher(http.StatusServiceUnavailable) {
			t.Fatalf("readiness gate drifted: %+v", health)
		}
		return container, nil
	})
	if err != nil || endpoint != "http://localhost:9000" || stop == nil || calls != 0 {
		t.Fatalf("successful start endpoint=%q owner=%t cleanup calls=%d err=%v", endpoint, stop != nil, calls, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := stop(ctx); err != nil || calls != 1 {
		t.Fatalf("successful ownership cleanup=%v calls=%d", err, calls)
	}
}
