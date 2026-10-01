package entitypub

import (
	"context"
	"testing"
)

type cancelOnPublish struct{ cancel context.CancelFunc }

func (p cancelOnPublish) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return p.PublishToStreamWithMsgID(ctx, subject, data, "")
}
func (p cancelOnPublish) PublishToStreamWithMsgID(ctx context.Context, _ string, _ []byte, _ string) error {
	p.cancel()
	return ctx.Err()
}

func TestCanceledAcceptedBatchRemainsAccounted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, _ := newTestPublisher(t, cancelOnPublish{cancel: cancel})
	for _, id := range []string{"one", "two", "three"} {
		if err := p.Send(payloadN(id)); err != nil {
			t.Fatal(err)
		}
	}
	p.drainBatch(ctx)
	if accounted := p.Published() + p.Lost() + int64(p.Pending()); accounted != 3 {
		t.Fatalf("accepted 3 entities, accounted for %d after cancellation: published=%d lost=%d pending=%d", accounted, p.Published(), p.Lost(), p.Pending())
	}
}
