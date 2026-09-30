package garagetest

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

type fixtureContainer struct {
	testcontainers.Container
	stop         func(context.Context) error
	bootstrapErr error
}

func (c *fixtureContainer) Terminate(ctx context.Context, _ ...testcontainers.TerminateOption) error {
	return c.stop(ctx)
}
func (c *fixtureContainer) Exec(context.Context, []string, ...tcexec.ProcessOption) (int, io.Reader, error) {
	return 0, nil, c.bootstrapErr
}

func TestFailedStartCleansContainerWithIndependentContext(t *testing.T) {
	for _, stage := range []string{"create", "bootstrap"} {
		for _, cleanupFails := range []bool{false, true} {
			t.Run(stage+map[bool]string{false: "/clean", true: "/cleanup_error"}[cleanupFails], func(t *testing.T) {
				startupFailure, cleanupFailure := errors.New("startup failed"), errors.New("cleanup failed")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				container := &fixtureContainer{bootstrapErr: startupFailure, stop: func(cleanupCtx context.Context) error {
					assertCleanupContext(cleanupCtx, t)
					calls++
					if cleanupFails && calls == 1 {
						return cleanupFailure
					}
					return nil
				}}
				endpoint, stop, err := startWithCreator(ctx, func(context.Context, testcontainers.GenericContainerRequest) (testcontainers.Container, error) {
					cancel()
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

func TestTerminateRetainsFailedCleanupAndForgetsSuccess(t *testing.T) {
	previous := terminate
	t.Cleanup(func() { terminate = previous })
	failure := errors.New("cannot remove fixture")
	calls := 0
	terminate = func(ctx context.Context) error {
		assertCleanupContext(ctx, t)
		calls++
		if calls == 1 {
			return failure
		}
		return nil
	}
	if err := Terminate(); !errors.Is(err, failure) {
		t.Fatalf("first cleanup=%v", err)
	}
	if err := Terminate(); err != nil {
		t.Fatalf("retry cleanup=%v", err)
	}
	if err := Terminate(); err != nil {
		t.Fatalf("already cleaned=%v", err)
	}
	if calls != 2 {
		t.Fatalf("cleanup calls=%d, want failed attempt plus successful retry", calls)
	}
}

func assertCleanupContext(ctx context.Context, t *testing.T) {
	t.Helper()
	if ctx.Err() != nil {
		t.Fatalf("cleanup received canceled context: %v", ctx.Err())
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 30*time.Second {
		t.Fatalf("cleanup requires a live deadline of at most 30s: %v, bounded=%t", deadline, ok)
	}
}
