package miniotest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

type testRunner func() int

func (run testRunner) Run() int { return run() }

func TestRunTestsReportsTeardownFailure(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(string(rune('0'+exitCode)), func(t *testing.T) {
			previous := terminate
			t.Cleanup(func() { terminate = previous })
			ran, cleaned := false, false
			terminate = func(ctx context.Context) error {
				if !ran {
					t.Fatal("cleanup ran before test execution")
				}
				assertCleanupContext(ctx, t)
				cleaned = true
				return errors.New("cannot remove fixture")
			}
			got := RunTests(testRunner(func() int { ran = true; return exitCode }))
			want := exitCode
			if want == 0 {
				want = 1
			}
			if got != want || !cleaned {
				t.Fatalf("exit=%d cleaned=%t, want exit=%d and cleanup", got, cleaned, want)
			}
		})
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

func TestRunTestsAttemptsEveryCleanupAndPreservesFailures(t *testing.T) {
	cases := []struct {
		name                   string
		testExit               int
		minioFails, extraFails bool
		want                   int
	}{
		{"all_success", 0, false, false, 0},
		{"minio_failure", 0, true, false, 1},
		{"extra_failure", 0, false, true, 1},
		{"both_fail", 0, true, true, 1},
		{"test_failure", 7, true, true, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := terminate
			t.Cleanup(func() { terminate = previous })
			var events []string
			terminate = func(context.Context) error {
				events = append(events, "MinIO")
				if tc.minioFails {
					return errors.New("MinIO cleanup failed")
				}
				return nil
			}
			extra := func() error {
				events = append(events, "Garage")
				if tc.extraFails {
					return errors.New("Garage cleanup failed")
				}
				return nil
			}
			got := RunTests(testRunner(func() int { events = append(events, "tests"); return tc.testExit }), extra)
			if got != tc.want || !slices.Equal(events, []string{"tests", "MinIO", "Garage"}) {
				t.Fatalf("exit=%d events=%v, want exit=%d and every cleanup after tests", got, events, tc.want)
			}
		})
	}
}
