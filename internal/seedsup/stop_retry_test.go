package seedsup

import (
	"context"
	"testing"
	"time"
)

func TestStopTimeoutRetainsSeedForRetry(t *testing.T) {
	var supervisor Supervisor
	entered, release := make(chan struct{}), make(chan struct{})
	supervisor.Start(context.Background(), nil, func(context.Context) error {
		close(entered)
		<-release
		return nil
	})
	<-entered
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		supervisor.Stop(ctx, nil)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for attempt := range 2 {
		supervisor.Stop(ctx, nil)
		if !supervisor.Running() {
			t.Fatalf("attempt %d forgot an unjoined seed after deadline", attempt)
		}
	}
	close(release)
	joinCtx, joinCancel := context.WithTimeout(context.Background(), time.Second)
	defer joinCancel()
	supervisor.Stop(joinCtx, nil)
	if supervisor.Running() {
		t.Fatal("completed seed still running after successful retry")
	}
}
