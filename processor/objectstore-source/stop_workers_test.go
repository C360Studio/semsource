package objectstoresource

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
)

type stopTransport struct{}

func (stopTransport) PublishToStream(context.Context, string, []byte) error { return nil }
func (stopTransport) PublishToStreamWithMsgID(context.Context, string, []byte, string) error {
	return nil
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
