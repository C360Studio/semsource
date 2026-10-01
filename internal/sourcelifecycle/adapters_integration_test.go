//go:build integration

package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	semconfig "github.com/c360studio/semstreams/config"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

func TestIntegrationJournalRetentionAndReceipts(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	authority := entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}
	j, err := OpenJournal(ctx, tc.Client, authority, "acme", 1)
	if err != nil {
		t.Fatal(err)
	}
	r := testRecord()
	rev, err := j.Create(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Phase = sourceintent.Pending
	if _, err := j.Update(ctx, r, rev); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(ctx, tc.Client, authority, "acme", 1)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := reopened.Get(ctx, r.Binding.Handle)
	if err != nil || got.Phase != sourceintent.Pending {
		t.Fatalf("reopened=%+v err=%v", got, err)
	}
	if _, err := OpenJournal(ctx, tc.Client, authority, "acme", 2); err == nil {
		t.Fatal("existing replica mismatch accepted")
	}
}
func TestIntegrationJournalRefusesMemoryBucket(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, err := tc.Client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: BucketName, Storage: jetstream.MemoryStorage, History: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournal(ctx, tc.Client, entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}, "acme", 1); err == nil {
		t.Fatal("memory journal accepted")
	}
}
func TestIntegrationTailIsCurrentRetainedOnly(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithStreams(natsclient.TestStreamConfig{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	status, err := semgraph.EnsureCatalogBucket(ctx, tc.Client, semgraph.BucketGraphStatus)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := status.Put(ctx, "graph-ingest", []byte(`{"state":"ready","ready":true,"bootstrap_complete":true}`)); err != nil {
		t.Fatal(err)
	}
	stream, err := tc.Client.GetStream(ctx, "GRAPH")
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Name: "graph-ingest-graph-ingest-entity", Durable: "graph-ingest-graph-ingest-entity", AckPolicy: jetstream.AckExplicitPolicy, DeliverPolicy: jetstream.DeliverAllPolicy, FilterSubject: "graph.ingest.entity", MaxDeliver: 3})
	if err != nil {
		t.Fatal(err)
	}
	scope := sourceintent.SourceScope{Inputs: []sourceintent.StreamInput{{Stream: "GRAPH", Consumer: "graph-ingest-graph-ingest-entity", Filters: []string{"graph.ingest.entity"}}}}
	observer := NATSTail{Client: tc.Client}
	evidence, err := observer.Settled(ctx, scope)
	if err != nil || evidence.Proof != sourceintent.TailProofCurrentRetained {
		t.Fatalf("proof=%+v err=%v", evidence, err)
	}
	if err := tc.Client.PublishToStream(ctx, "graph.ingest.entity", []byte("pending")); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Settled(ctx, scope); err == nil {
		t.Fatal("pending input appeared settled")
	}
	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for msg := range batch.Messages() {
		if err := msg.DoubleAck(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := observer.Settled(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := status.Put(ctx, "graph-ingest", []byte(`{"state":"degraded","failed_count":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Settled(ctx, scope); err == nil {
		t.Fatal("degraded input permitted projection")
	}
}
func TestIntegrationProjectionRetainsPartialFailure(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub, err := tc.Client.SubscribeForRequests(ctx, ProjectionSubject, func(context.Context, []byte) ([]byte, error) {
		return json.Marshal(ProjectionReply{Result: sourceintent.ProjectionResult{Enumerated: 4, Marked: 2}, Error: &sourceintent.Blocker{Code: sourceintent.CodeMutation, Message: "retry CAS", Retryable: true}})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sub.Drain(ctx); err != nil {
			t.Error(err)
		}
	}()
	p := NATSProjector{Client: tc.Client}
	result, err := p.ApplyRemoval(ctx, sourceintent.RemovalProjection{})
	var b *sourceintent.Blocker
	if !errors.As(err, &b) || b.Code != sourceintent.CodeMutation || result.Marked != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := p.ApplyReactivation(ctx, sourceintent.ReactivationProjection{}); err == nil {
		t.Fatal("reactivation hid partial failure")
	}
}

func TestIntegrationConfigReaderUsesDurableEnvelope(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	name, err := semconfig.BucketName("acme", "source")
	if err != nil {
		t.Fatal(err)
	}
	js, err := tc.Client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: name, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := ConfigReader(ctx, tc.Client, "acme", "source")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists, err := reader(ctx, "docs"); err != nil || exists {
		t.Fatalf("missing: %v %v", exists, err)
	}
	if _, err := kv.Put(ctx, "components.docs", []byte(`{"name":"doc-source","enabled":false,"config":{"paths":["/docs"]}}`)); err != nil {
		t.Fatal(err)
	}
	cc, exists, err := reader(ctx, "docs")
	if err != nil || !exists || cc.Enabled || cc.Name != "doc-source" {
		t.Fatalf("durable envelope: %+v %v %v", cc, exists, err)
	}
	if _, err := kv.Put(ctx, "components.docs", []byte(`{`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reader(ctx, "docs"); err == nil {
		t.Fatal("corrupt durable envelope accepted")
	}
	if _, err := ConfigReader(ctx, tc.Client, "bad.org", "source"); err == nil {
		t.Fatal("invalid declared authority accepted")
	}
	if _, err := ConfigReader(ctx, tc.Client, "acme", "absent"); err == nil {
		t.Fatal("missing bucket accepted")
	}
}
