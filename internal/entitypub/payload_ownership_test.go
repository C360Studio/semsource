package entitypub

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
)

type wirePublisher struct{ messages chan []byte }

func (p wirePublisher) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return p.PublishToStreamWithMsgID(ctx, subject, data, "")
}
func (p wirePublisher) PublishToStreamWithMsgID(_ context.Context, _ string, data []byte, _ string) error {
	p.messages <- append([]byte(nil), data...)
	return nil
}

func TestAcceptedPayloadOwnsQueuedWireData(t *testing.T) {
	transport := wirePublisher{messages: make(chan []byte, 1)}
	p, _ := newTestPublisher(t, transport)
	payload := payloadN("owned")
	expires := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	number := json.Number("9007199254740993")
	payload.TripleData = []message.Triple{{
		Subject: payload.ID, Predicate: source.DocChunkCount, Object: &number,
		Source: "ownership-test", Timestamp: payload.UpdatedAt, Confidence: 1, ExpiresAt: &expires,
	}}
	payload.Storage = &message.StorageReference{StorageInstance: "files", Key: "original", ContentType: "text/plain", Size: 23}
	want, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Send(payload); err != nil {
		t.Fatal(err)
	}
	// Reusing producer-owned memory after admission must not alter eventual
	// wire data, including nested references and integers beyond float64 precision.
	payload.ID = "org.semsource.golang.sys.function.changed"
	payload.IndexingProfileHint = graph.IndexingProfileControl
	payload.UpdatedAt = payload.UpdatedAt.Add(time.Hour)
	payload.Storage.Key = "changed"
	payload.TripleData[0].Source = "changed"
	number = "9007199254740995"
	expires = expires.Add(time.Hour)
	p.Start(context.Background())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := p.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(<-transport.messages, &wire); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wire.Payload, want) {
		t.Fatalf("accepted payload changed before delivery:\n got %s\nwant %s", wire.Payload, want)
	}
	if p.Published() != 1 || p.Lost() != 0 || p.Pending() != 0 {
		t.Fatalf("delivery accounting changed: published=%d lost=%d pending=%d", p.Published(), p.Lost(), p.Pending())
	}
}

func TestUnrepresentablePayloadRejectedBeforeAdmission(t *testing.T) {
	for _, name := range []string{"nil", "invalid object"} {
		t.Run(name, func(t *testing.T) {
			transport := wirePublisher{messages: make(chan []byte, 1)}
			p, _ := newTestPublisher(t, transport)
			var payload *graph.EntityPayload
			if name == "invalid object" {
				payload = payloadN("invalid")
				payload.TripleData = []message.Triple{{Subject: payload.ID, Predicate: source.DocChunkCount, Object: make(chan int)}}
			}
			if err := p.Send(payload); err == nil {
				t.Fatal("accepted a payload that cannot be owned as wire data")
			}
			if p.Pending() != 0 || p.Published() != 0 || p.Dropped() != 1 || p.Failed() != 0 {
				t.Fatalf("rejection accounting: pending=%d published=%d dropped=%d failed=%d", p.Pending(), p.Published(), p.Dropped(), p.Failed())
			}
		})
	}
}
