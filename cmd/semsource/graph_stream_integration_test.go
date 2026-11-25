//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	semconfig "github.com/c360studio/semengine/config"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semsource/config"
	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/natstest"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIntegration_GraphStreamKeepsPublishBeforeConsumerBinds publishes an
// entity to a fresh GRAPH stream built from graphStreamConfig before
// graph-ingest exists, then starts graph-ingest with its production config.
// Workqueue retention keeps the message until a consumer takes it, so the
// entity is applied after the bind, and graph-ingest's ack then removes it.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: The ingest transport stream refuses rather than evicts
func TestIntegration_GraphStreamKeepsPublishBeforeConsumerBinds(t *testing.T) {
	tc := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := semconfig.NewStreamsManager(tc.Client, logger).EnsureStreams(ctx, &semconfig.Config{
		Streams: graphStreamConfig(&config.Config{}),
	}); err != nil {
		t.Fatalf("EnsureStreams() error = %v", err)
	}

	const entityID = "acme.test-a1b2c3.repo.git.commit.early01"
	msg := message.NewBaseMessage(semsourcegraph.EntityType, &semsourcegraph.EntityPayload{
		ID:                  entityID,
		TripleData:          []message.Triple{{Subject: entityID, Predicate: source.GitCommitSubject, Object: "published before the bind"}},
		UpdatedAt:           time.Now().UTC(),
		IndexingProfileHint: semsourcegraph.IndexingProfileContent,
	}, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal entity message: %v", err)
	}
	if err := tc.Client.PublishToStream(ctx, "graph.ingest.entity", data); err != nil {
		t.Fatalf("PublishToStream() error = %v", err)
	}
	js, err := tc.Client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(ctx, "GRAPH")
	if err != nil {
		t.Fatal(err)
	}
	if got := stream.CachedInfo().Config.Retention; got != jetstream.WorkQueuePolicy {
		t.Fatalf("GRAPH retention = %v, want workqueue", got)
	}
	if n := entityMessages(ctx, t, stream); n != 1 {
		t.Fatalf("GRAPH holds %d graph.ingest.entity messages before the bind, want 1", n)
	}

	ingest := startProductionGraphIngest(ctx, t, tc)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := ingest.Stop(stopCtx); err != nil {
			t.Errorf("graph-ingest Stop() error = %v", err)
		}
	})

	kv, err := js.KeyValue(ctx, semgraph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open ENTITY_STATES: %v", err)
	}
	watcher, err := kv.Watch(ctx, entityID, jetstream.IgnoreDeletes())
	if err != nil {
		t.Fatalf("watch %s: %v", entityID, err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Errorf("stop watch: %v", err)
		}
	}()
	var stored semgraph.EntityState
	for applied := false; !applied; {
		select {
		case entry := <-watcher.Updates():
			if entry == nil {
				continue
			}
			if err := json.Unmarshal(entry.Value(), &stored); err != nil {
				t.Fatalf("decode %s: %v", entityID, err)
			}
			applied = true
		case <-ctx.Done():
			t.Fatalf("%s never applied: %v", entityID, ctx.Err())
		}
	}
	if stored.ID != entityID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, entityID)
	}

	waitForGraphIngestAck(ctx, t, stream)
	if n := entityMessages(ctx, t, stream); n != 0 {
		t.Fatalf("GRAPH holds %d graph.ingest.entity messages after graph-ingest applied the entity, want 0", n)
	}
}

// waitForGraphIngestAck returns once graph-ingest has acknowledged the
// stream's first message. graph-ingest acks after the ENTITY_STATES write,
// and under workqueue the ack removes the message.
func waitForGraphIngestAck(ctx context.Context, t *testing.T, stream jetstream.Stream) {
	t.Helper()
	consumer, err := stream.Consumer(ctx, "graph-ingest-graph-ingest-entity")
	if err != nil {
		t.Fatalf("graph-ingest consumer: %v", err)
	}
	for {
		info, err := consumer.Info(ctx)
		if err != nil {
			t.Fatalf("consumer info: %v", err)
		}
		if info.AckFloor.Stream >= 1 && info.NumAckPending == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("graph-ingest never acknowledged the entity: ack_floor=%d ack_pending=%d", info.AckFloor.Stream, info.NumAckPending)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func entityMessages(ctx context.Context, t *testing.T, stream jetstream.Stream) uint64 {
	t.Helper()
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter("graph.ingest.entity"))
	if err != nil {
		t.Fatalf("GRAPH info: %v", err)
	}
	return info.State.Subjects["graph.ingest.entity"]
}

// startProductionGraphIngest starts graph-ingest from the config
// buildSemstreamsConfig gives it.
func startProductionGraphIngest(ctx context.Context, t *testing.T, tc *natstest.Server) *graphingest.Component {
	t.Helper()
	ssCfg, err := buildSemstreamsConfig(&config.Config{Namespace: "acme"}, "acme")
	if err != nil {
		t.Fatalf("buildSemstreamsConfig() error = %v", err)
	}
	reg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatalf("buildPayloadRegistry() error = %v", err)
	}
	discovered, err := graphingest.CreateGraphIngest(ssCfg.Components["graph-ingest"].Config, component.Dependencies{
		Platform:        types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"},
		NATSClient:      tc.Client,
		PayloadRegistry: reg,
		MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphIngest() error = %v", err)
	}
	ingest := discovered.(*graphingest.Component)
	if err := ingest.Initialize(); err != nil {
		t.Fatalf("graph-ingest Initialize() error = %v", err)
	}
	if err := ingest.Start(ctx); err != nil {
		t.Fatalf("graph-ingest Start() error = %v", err)
	}
	return ingest
}
