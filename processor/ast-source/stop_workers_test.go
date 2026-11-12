package astsource

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
)

type stopTransport struct{}

func (stopTransport) PublishToStream(ctx context.Context, _ string, _ []byte) error {
	return ctx.Err()
}
func (stopTransport) PublishToStreamWithMsgID(ctx context.Context, _ string, _ []byte, _ string) error {
	return ctx.Err()
}

func TestStopJoinsProducerBeforePublisherSettlement(t *testing.T) {
	pub, err := entitypub.New(stopTransport{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(context.Background())
	canceled, release, produced := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	c := &Component{running: true, logger: slog.Default(), publisher: pub, cancelFuncs: []context.CancelFunc{func() { close(canceled) }}}
	c.workers.Go(func() {
		<-canceled
		<-release
		produced <- pub.Send(&graph.EntityPayload{ID: "acme.test.docs.web.doc.tail", IndexingProfileHint: graph.IndexingProfileContent})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- c.Stop(ctx) }()
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("source did not cancel producer")
	}
	select {
	case err := <-result:
		t.Fatalf("Stop returned while producer still active: %v", err)
	default:
	}
	close(release)
	if err := <-produced; err != nil {
		t.Fatalf("accepted producer tail rejected: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if pub.Published() != 1 || pub.Lost() != 0 {
		t.Fatalf("delivered=%d lost=%d", pub.Published(), pub.Lost())
	}
}

func TestStartContextCancelDoesNotDropEntitiesBeforeStop(t *testing.T) {
	pub, err := entitypub.New(stopTransport{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	startCtx, startCancel := context.WithCancel(context.Background())
	runCtx := context.WithoutCancel(startCtx)
	pub.Start(runCtx)
	c := &Component{running: true, logger: slog.Default(), publisher: pub}
	c.startProgressReporting(runCtx)
	seedEntered := make(chan struct{})
	seedRelease := make(chan struct{})
	c.seed.Start(runCtx, slog.Default(), func(ctx context.Context) error {
		close(seedEntered)
		<-seedRelease
		return ctx.Err()
	})
	<-seedEntered

	// Cancel startCtx (simulating SIGTERM cancelling StartAll's signalCtx before StopAll).
	startCancel()

	if err := pub.Send(&graph.EntityPayload{
		ID:                  "acme.test.docs.web.doc.after-sigterm",
		IndexingProfileHint: graph.IndexingProfileContent,
	}); err != nil {
		t.Fatalf("Send after startCtx cancel: %v", err)
	}
	close(seedRelease)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed after startCtx cancel: %v", err)
	}
	if pub.Published() != 1 || pub.Lost() != 0 {
		t.Fatalf("delivered=%d lost=%d, want 1 and 0", pub.Published(), pub.Lost())
	}
}
