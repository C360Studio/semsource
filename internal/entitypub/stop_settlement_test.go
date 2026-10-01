package entitypub

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/pkg/buffer"
)

type notifyingBuffer struct {
	buffer.Buffer[*graph.EntityPayload]
	sealed chan struct{}
}

func (b *notifyingBuffer) Close() error { err := b.Buffer.Close(); close(b.sealed); return err }

type settlementPublisher struct {
	entered chan context.Context
	release chan struct{}
}

func (p *settlementPublisher) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return p.PublishToStreamWithMsgID(ctx, subject, data, "")
}
func (p *settlementPublisher) PublishToStreamWithMsgID(ctx context.Context, _ string, _ []byte, _ string) error {
	select {
	case p.entered <- ctx:
	default:
	}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestStopSettlesAcceptedBatch(t *testing.T) {
	transport := &settlementPublisher{entered: make(chan context.Context, 1), release: make(chan struct{})}
	const total = defaultBatchSize + 2
	p, _ := newTestPublisher(t, transport, WithCapacity(total))
	sealed := make(chan struct{})
	p.buf = &notifyingBuffer{Buffer: p.buf, sealed: sealed}
	for i := 0; i < total; i++ {
		if err := p.Send(payloadN(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
	p.Start(context.Background())
	publishCtx := <-transport.entered
	if p.Pending() == 0 {
		t.Fatal("fixture requires queued work beyond the active batch")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(stopCtx) }()
	select {
	case <-sealed:
	case <-stopCtx.Done():
		t.Fatal("Stop did not seal admission")
	}
	// A successful stop is impossible while the broker still owns this publish.
	select {
	case err := <-stopped:
		t.Fatalf("stop returned before broker settlement: %v", err)
	default:
	}
	if err := publishCtx.Err(); err != nil {
		t.Fatalf("stop canceled accepted work: %v", err)
	}
	close(transport.release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if p.Published() != total || p.Lost() != 0 || p.Pending() != 0 {
		t.Fatalf("settlement: delivered=%d lost=%d pending=%d", p.Published(), p.Lost(), p.Pending())
	}
	if err := p.Send(payloadN("late")); err == nil {
		t.Fatal("stop accepted a late producer")
	}
}
func TestStopDeadlineReportsIncompleteSettlement(t *testing.T) {
	transport := &settlementPublisher{entered: make(chan context.Context, 1), release: make(chan struct{})}
	p, _ := newTestPublisher(t, transport)
	if err := p.Send(payloadN("one")); err != nil {
		t.Fatal(err)
	}
	p.Start(context.Background())
	<-transport.entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error=%v", err)
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("canceled publish never joined")
	}
	if p.Failed() != 1 {
		t.Fatalf("canceled delivery not counted: %d", p.Failed())
	}
}
