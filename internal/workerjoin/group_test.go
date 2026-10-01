package workerjoin

import (
	"context"
	"errors"
	"testing"
)

func TestWaitJoinsAcceptedWorkerAndNestedWork(t *testing.T) {
	var g Group
	entered, release, nested := make(chan struct{}), make(chan struct{}), make(chan struct{})
	g.Go(func() { close(entered); <-release; g.Go(func() { close(nested) }) })
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked worker wait=%v", err)
	}
	close(release)
	if err := g.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-nested:
	default:
		t.Fatal("Wait returned before nested worker")
	}
}
func TestEmptyGroupIsSettled(t *testing.T) {
	var g Group
	if err := g.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeJoinHonorsDeadlineAndCanRetry(t *testing.T) {
	var g Group
	ch := make(chan int)
	g.JoinOnStop(func(ctx context.Context) error { return Channel(ctx, ch) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("native join error=%v", err)
	}
	close(ch)
	if err := g.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
