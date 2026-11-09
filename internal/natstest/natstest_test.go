package natstest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
)

// fakeBroker answers MappedPort from a script: each call pops the next error,
// and an exhausted script publishes port 4222. keepAbsent leaves the port
// unpublished for the whole observation window.
type fakeBroker struct {
	hostErr      error
	portErrs     []error
	keepAbsent   bool
	blockAfter   int // when > 0, calls after this many block until their context ends
	portCalls    int
	onTerminate  func()
	terminateErr error
	terminated   int
}

func (f *fakeBroker) Host(context.Context) (string, error) { return "127.0.0.1", f.hostErr }

func (f *fakeBroker) MappedPort(ctx context.Context, _ nat.Port) (nat.Port, error) {
	f.portCalls++
	if f.blockAfter > 0 && f.portCalls > f.blockAfter {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if f.keepAbsent {
		return "", errAbsent
	}
	if len(f.portErrs) > 0 {
		err := f.portErrs[0]
		f.portErrs = f.portErrs[1:]
		return "", err
	}
	return "4222", nil
}

func (f *fakeBroker) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	f.terminated++
	if f.onTerminate != nil {
		f.onTerminate()
	}
	return f.terminateErr
}

var errAbsent = cerrdefs.ErrNotFound.WithMessage(`port "4222/tcp" not found`)

// launchSequence hands out the given brokers in order and counts launches.
func launchSequence(brokers ...*fakeBroker) (launcher, *int) {
	launches := 0
	return func(context.Context) (broker, error) {
		b := brokers[launches]
		launches++
		return b, nil
	}, &launches
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_TransientPortAbsenceRecoversWithoutReplacement(t *testing.T) {
	b := &fakeBroker{portErrs: []error{errAbsent, errAbsent}}
	launch, launches := launchSequence(b)

	_, url, err := start(t.Context(), launch, time.Second)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if url != "nats://127.0.0.1:4222" || *launches != 1 || b.terminated != 0 {
		t.Fatalf("url=%q launches=%d terminated=%d", url, *launches, b.terminated)
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_PersistentPortAbsenceReplacesOnceThenFails(t *testing.T) {
	first, second := &fakeBroker{keepAbsent: true}, &fakeBroker{keepAbsent: true}
	launch, launches := launchSequence(first, second)

	_, _, err := start(t.Context(), launch, 50*time.Millisecond)
	if err == nil {
		t.Fatal("start succeeded with the port never published")
	}
	if *launches != 2 || first.terminated != 1 || second.terminated != 1 {
		t.Fatalf("launches=%d terminated=(%d,%d), want 2 and (1,1)", *launches, first.terminated, second.terminated)
	}
	if !strings.Contains(err.Error(), "attempt 1") || !strings.Contains(err.Error(), "attempt 2") {
		t.Fatalf("error does not carry both attempts: %v", err)
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_PersistentAbsenceThenPublishedSucceedsOnReplacement(t *testing.T) {
	first, second := &fakeBroker{keepAbsent: true}, &fakeBroker{}
	launch, launches := launchSequence(first, second)

	got, _, err := start(t.Context(), launch, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if got != second || *launches != 2 || first.terminated != 1 {
		t.Fatalf("launches=%d first.terminated=%d", *launches, first.terminated)
	}
}

// The window's deadline can land inside a MappedPort call, which then returns
// the deadline error instead of not-found. The absence already observed still
// counts, so the container is replaced. synctest's fake clock makes the order
// exact: the first call sees absence, the 100 ms poll fires, and the 150 ms
// window ends inside the second call.
// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_WindowExpiringMidCallStillReplaces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := &fakeBroker{keepAbsent: true, blockAfter: 1}
		launch, launches := launchSequence(first, &fakeBroker{})

		if _, _, err := start(t.Context(), launch, 150*time.Millisecond); err != nil {
			t.Fatalf("start: %v", err)
		}
		if *launches != 2 || first.terminated != 1 || first.portCalls != 2 {
			t.Fatalf("launches=%d first.terminated=%d portCalls=%d, want 2, 1, 2",
				*launches, first.terminated, first.portCalls)
		}
	})
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_CancelledDuringCleanupCarriesCause(t *testing.T) {
	custom := errors.New("test deadline from caller")
	termErr := errors.New("docker: remove failed")
	for _, tc := range []struct {
		name    string
		cause   error
		termErr error
	}{
		{"canceled, clean removal", nil, nil},
		{"canceled, failed removal", nil, termErr},
		{"custom cause, clean removal", custom, nil},
		{"custom cause, failed removal", custom, termErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			first := &fakeBroker{keepAbsent: true, terminateErr: tc.termErr, onTerminate: func() { cancel(tc.cause) }}
			launch, launches := launchSequence(first, &fakeBroker{})

			_, _, err := start(ctx, launch, 50*time.Millisecond)
			want := []error{errAbsent, context.Canceled}
			if tc.cause != nil {
				want = []error{errAbsent, tc.cause}
			}
			if tc.termErr != nil {
				want = append(want, tc.termErr)
			}
			for _, w := range want {
				if !errors.Is(err, w) {
					t.Errorf("error %v does not carry %v", err, w)
				}
			}
			if *launches != 1 {
				t.Fatalf("launches = %d, want 1", *launches)
			}
		})
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_FailedTerminationStopsReplacement(t *testing.T) {
	termErr := errors.New("docker: remove failed")
	first := &fakeBroker{keepAbsent: true, terminateErr: termErr}
	launch, launches := launchSequence(first, &fakeBroker{})

	_, _, err := start(t.Context(), launch, 50*time.Millisecond)
	if !errors.Is(err, termErr) || !errors.Is(err, errAbsent) {
		t.Fatalf("error = %v, want both the port and the termination cause", err)
	}
	if *launches != 1 {
		t.Fatalf("launches = %d, want 1: a leaked broker must not be replaced", *launches)
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_NonAbsenceFailuresAreNotReplaced(t *testing.T) {
	inspectErr := errors.New("inspect: connection refused")
	hostErr := errors.New("docker host unknown")
	for name, b := range map[string]*fakeBroker{
		"inspect error": {portErrs: []error{inspectErr}},
		"host error":    {hostErr: hostErr},
	} {
		t.Run(name, func(t *testing.T) {
			launch, launches := launchSequence(b, &fakeBroker{})
			if _, _, err := start(t.Context(), launch, time.Second); err == nil {
				t.Fatal("start succeeded")
			}
			if *launches != 1 || b.terminated != 1 {
				t.Fatalf("launches=%d terminated=%d, want 1 and 1", *launches, b.terminated)
			}
		})
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_CancelledTestIsNotReplaced(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	b := &fakeBroker{keepAbsent: true}
	launches := 0
	launch := func(context.Context) (broker, error) {
		launches++
		cancel() // the test ends while the first container is starting
		return b, nil
	}

	_, _, err := start(ctx, launch, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to carry context.Canceled", err)
	}
	if launches != 1 || b.terminated != 1 {
		t.Fatalf("launches=%d terminated=%d, want 1 and 1", launches, b.terminated)
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_FailedLaunchRemovesReturnedContainer(t *testing.T) {
	b := &fakeBroker{}
	launchErr := errors.New("wait for log: context deadline exceeded")
	launch := func(context.Context) (broker, error) { return b, launchErr }

	if _, _, err := start(t.Context(), launch, time.Second); !errors.Is(err, launchErr) {
		t.Fatalf("error = %v", err)
	}
	if b.terminated != 1 {
		t.Fatalf("terminated = %d, want 1", b.terminated)
	}
}

// spec: openspec/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestStart_LaunchIsBoundedByStartTimeout(t *testing.T) {
	var deadline time.Time
	var ok bool
	launch := func(ctx context.Context) (broker, error) {
		deadline, ok = ctx.Deadline()
		return &fakeBroker{}, nil
	}

	before := time.Now()
	if _, _, err := start(context.Background(), launch, time.Second); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !ok || deadline.After(before.Add(startTimeout+time.Second)) {
		t.Fatalf("launch deadline = %v (set %v), want within startTimeout", deadline, ok)
	}
}
