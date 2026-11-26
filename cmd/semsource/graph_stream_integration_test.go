//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	semconfig "github.com/c360studio/semengine/config"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

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
	publishEntity(ctx, t, tc.Client, entityID)
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

	ingest := startProductionGraphIngest(ctx, t, tc.Client)
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := ingest.Stop(stopCtx); err != nil {
			t.Errorf("graph-ingest Stop() error = %v", err)
		}
	})

	stored := waitForApplied(ctx, t, js, entityID)
	if stored.ID != entityID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, entityID)
	}

	waitForGraphIngestAck(ctx, t, stream)
	if n := entityMessages(ctx, t, stream); n != 0 {
		t.Fatalf("GRAPH holds %d graph.ingest.entity messages after graph-ingest applied the entity, want 0", n)
	}
}

// TestIntegration_GraphStreamPostureAcrossNATSRestart publishes entities to
// a GRAPH stream built from graphStreamConfig with graph-ingest not running,
// then stops and starts the same NATS container. Under the durable default
// (file storage on the container's filesystem) the messages are still in the
// stream after the restart, and graph-ingest, started on a fresh connection,
// applies them. Under the ephemeral posture (memory storage) the restart
// loses them: the contrast shows the restart actually discarded server memory.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: The ingest transport stream refuses rather than evicts
func TestIntegration_GraphStreamPostureAcrossNATSRestart(t *testing.T) {
	tests := []struct {
		name    string
		posture string
		survive bool
	}{
		{name: "durable default", posture: "", survive: true},
		{name: "ephemeral", posture: config.GraphStreamEphemeral, survive: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			streams := graphStreamConfig(&config.Config{GraphStream: config.GraphStreamConfig{Posture: tt.posture}})
			broker := startRestartableNATS(ctx, t)

			client := broker.connect(ctx, t)
			if err := semconfig.NewStreamsManager(client, logger).EnsureStreams(ctx, &semconfig.Config{Streams: streams}); err != nil {
				t.Fatalf("EnsureStreams() error = %v", err)
			}
			entityIDs := []string{
				"acme.test-a1b2c3.repo.git.commit.restart01",
				"acme.test-a1b2c3.repo.git.commit.restart02",
				"acme.test-a1b2c3.repo.git.commit.restart03",
			}
			for _, id := range entityIDs {
				publishEntity(ctx, t, client, id)
			}
			closeNATSClient(t, client)

			broker.restart(ctx, t)
			client = broker.connect(ctx, t)
			t.Cleanup(func() { closeNATSClient(t, client) })
			js, err := client.JetStream()
			if err != nil {
				t.Fatal(err)
			}

			if !tt.survive {
				stream, err := js.Stream(ctx, "GRAPH")
				switch {
				case errors.Is(err, jetstream.ErrStreamNotFound):
				case err != nil:
					t.Fatalf("GRAPH after restart: %v", err)
				default:
					if n := entityMessages(ctx, t, stream); n != 0 {
						t.Fatalf("ephemeral GRAPH holds %d graph.ingest.entity messages after the restart, want 0", n)
					}
				}
				return
			}

			stream, err := js.Stream(ctx, "GRAPH")
			if err != nil {
				t.Fatalf("GRAPH after restart: %v", err)
			}
			if got := stream.CachedInfo().Config.Storage; got != jetstream.FileStorage {
				t.Fatalf("GRAPH storage = %v, want file", got)
			}
			if n := entityMessages(ctx, t, stream); n != uint64(len(entityIDs)) {
				t.Fatalf("GRAPH holds %d graph.ingest.entity messages after the restart, want %d", n, len(entityIDs))
			}
			// Boot declares the stream again; the retained one must not drift.
			if err := semconfig.NewStreamsManager(client, logger).EnsureStreams(ctx, &semconfig.Config{Streams: streams}); err != nil {
				t.Fatalf("EnsureStreams() after restart error = %v", err)
			}

			ingest := startProductionGraphIngest(ctx, t, client)
			t.Cleanup(func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				if err := ingest.Stop(stopCtx); err != nil {
					t.Errorf("graph-ingest Stop() error = %v", err)
				}
			})
			for _, id := range entityIDs {
				if stored := waitForApplied(ctx, t, js, id); stored.ID != id {
					t.Fatalf("stored ID = %q, want %q", stored.ID, id)
				}
			}
		})
	}
}

// restartableNATS is a JetStream broker whose store directory lives on the
// container's own filesystem (not a tmpfs), so a stop and start keeps file
// storage and drops memory storage, as a NATS process restart does.
type restartableNATS struct {
	container testcontainers.Container
}

func startRestartableNATS(ctx context.Context, t *testing.T) *restartableNATS {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        natstest.Image,
			ExposedPorts: []string{"4222/tcp"},
			Cmd:          []string{"--port", "4222", "--js", "--store_dir", "/var/lib/nats"},
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if container != nil {
		t.Cleanup(func() {
			termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer termCancel()
			if err := container.Terminate(termCtx); err != nil {
				t.Errorf("terminate NATS: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("start NATS: %v", err)
	}
	return &restartableNATS{container: container}
}

// restart stops the container (SIGTERM, then the NATS process exits) and
// starts the same container again.
func (b *restartableNATS) restart(ctx context.Context, t *testing.T) {
	t.Helper()
	stopTimeout := 10 * time.Second
	if err := b.container.Stop(ctx, &stopTimeout); err != nil {
		t.Fatalf("stop NATS: %v", err)
	}
	if err := b.container.Start(ctx); err != nil {
		t.Fatalf("start NATS again: %v", err)
	}
}

// connect retries until the broker accepts a client: after a restart the
// container's earlier "Server is ready" log line says nothing, and the host
// port binding can appear after the container reports running (gh#736).
func (b *restartableNATS) connect(ctx context.Context, t *testing.T) *natsclient.Client {
	t.Helper()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var lastErr error
	for {
		client, err := b.tryConnect(ctx)
		if err == nil {
			return client
		}
		lastErr = err
		select {
		case <-ctx.Done():
			t.Fatalf("connect NATS: %v (last error: %v)", ctx.Err(), lastErr)
		case <-tick.C:
		}
	}
}

func (b *restartableNATS) tryConnect(ctx context.Context) (*natsclient.Client, error) {
	host, err := b.container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("host: %w", err)
	}
	port, err := b.container.MappedPort(ctx, "4222/tcp")
	if err != nil {
		return nil, fmt.Errorf("mapped port: %w", err)
	}
	client, err := natsclient.NewClient(fmt.Sprintf("nats://%s:%s", host, port.Port()),
		natsclient.WithTimeout(5*time.Second),
		natsclient.WithMaxReconnects(0),
	)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Connect(connectCtx); err != nil {
		closeErr := client.Close(connectCtx)
		return nil, errors.Join(fmt.Errorf("connect: %w", err), closeErr)
	}
	if _, err := client.JetStream(); err != nil {
		return nil, errors.Join(fmt.Errorf("jetstream: %w", err), client.Close(connectCtx))
	}
	return client, nil
}

func closeNATSClient(t *testing.T, client *natsclient.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Errorf("close NATS client: %v", err)
	}
}

// publishEntity publishes one entity message to graph.ingest.entity and
// returns once the stream has acknowledged it.
func publishEntity(ctx context.Context, t *testing.T, client *natsclient.Client, entityID string) {
	t.Helper()
	msg := message.NewBaseMessage(semsourcegraph.EntityType, &semsourcegraph.EntityPayload{
		ID:                  entityID,
		TripleData:          []message.Triple{{Subject: entityID, Predicate: source.GitCommitSubject, Object: "published before graph-ingest ran"}},
		UpdatedAt:           time.Now().UTC(),
		IndexingProfileHint: semsourcegraph.IndexingProfileContent,
	}, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal entity message: %v", err)
	}
	if err := client.PublishToStream(ctx, "graph.ingest.entity", data); err != nil {
		t.Fatalf("PublishToStream() error = %v", err)
	}
}

// waitForApplied returns entityID's ENTITY_STATES value once graph-ingest has
// written it.
func waitForApplied(ctx context.Context, t *testing.T, js jetstream.JetStream, entityID string) semgraph.EntityState {
	t.Helper()
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
	for {
		select {
		case entry := <-watcher.Updates():
			if entry == nil {
				continue
			}
			var stored semgraph.EntityState
			if err := json.Unmarshal(entry.Value(), &stored); err != nil {
				t.Fatalf("decode %s: %v", entityID, err)
			}
			return stored
		case <-ctx.Done():
			t.Fatalf("%s never applied: %v", entityID, ctx.Err())
		}
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
func startProductionGraphIngest(ctx context.Context, t *testing.T, client *natsclient.Client) *graphingest.Component {
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
		NATSClient:      client,
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
