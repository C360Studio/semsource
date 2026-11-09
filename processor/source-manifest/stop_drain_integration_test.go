//go:build integration

package sourcemanifest

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/natstest"
)

// spec: openspec/changes/nats-test-fixture/specs/semstreams-governance/spec.md#Requirement: Component Stop keeps a subscription until its callbacks drain
func TestIntegrationStopRetainsActiveSubscriptionOnDeadline(t *testing.T) {
	tc := natstest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	sub, err := tc.Client.SubscribeForRequests(ctx, "semsource.test.drain", func(context.Context, []byte) ([]byte, error) { close(entered); <-release; return []byte(`{}`), nil })
	if err != nil {
		t.Fatal(err)
	}
	c := &Component{running: true, logger: slog.Default(), querySub: sub}
	requested := make(chan struct{})
	go func() {
		defer close(requested)
		_, _ = tc.Client.Request(ctx, "semsource.test.drain", nil, 10*time.Second)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("handler did not start")
	}
	expired, expire := context.WithCancel(context.Background())
	expire()
	if err := c.Stop(expired); err == nil {
		close(release)
		t.Fatal("Stop reported success with an active callback")
	}
	if c.querySub == nil {
		close(release)
		t.Fatal("Stop lost the subscription needed to retry join")
	}
	close(release)
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if c.running || c.querySub != nil {
		t.Fatal("successful drain retained running state")
	}
	select {
	case <-requested:
	case <-ctx.Done():
		t.Fatal("request did not complete")
	}
}
