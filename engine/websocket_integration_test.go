//go:build integration

package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/engine"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/normalizer"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/natsclient"
	wsoutput "github.com/c360studio/semstreams/output/websocket"
	"github.com/gorilla/websocket"
)

// natsEmitter publishes federation.Event values to a NATS subject so the
// semstreams WebSocket output can forward them to connected clients.
type natsEmitter struct {
	client  *natsclient.Client
	subject string
}

func (e *natsEmitter) Emit(ctx context.Context, event *federation.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return e.client.Publish(ctx, e.subject, data)
}

// freePort returns an available TCP port on loopback by briefly listening on
// port 0 and immediately closing the listener.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// TestWebSocketIntegration_SEEDEvent is an end-to-end test that verifies the
// complete pipeline:
//
//  1. NATS container starts via testcontainers (handled by natsclient.NewTestClient).
//  2. A semstreams WebSocket output subscribes to NATS and serves clients.
//  3. The semsource engine ingests a markdown doc and emits a SEED event via
//     natsEmitter, which publishes to NATS.
//  4. The WebSocket output forwards the message to the connected gorilla client.
//  5. The test asserts the received envelope wraps a valid SEED event with
//     entities and the expected namespace.
func TestWebSocketIntegration_SEEDEvent(t *testing.T) {
	ctx := context.Background()

	// --- NATS -----------------------------------------------------------
	// NewTestClient starts a real NATS container and registers t.Cleanup to
	// drain and terminate it. WithFastStartup shortens the connection timeout
	// (2 s) and container startup timeout (10 s) — appropriate for a
	// same-machine Docker daemon.
	tc := natsclient.NewTestClient(t, natsclient.WithFastStartup())

	// --- WebSocket output -----------------------------------------------
	subject := "semsource.graph"
	wsPort := freePort(t)

	wsOut := wsoutput.NewOutput(wsPort, "/ws", []string{subject}, tc.Client)
	if err := wsOut.Initialize(); err != nil {
		t.Fatalf("initialize ws output: %v", err)
	}
	if err := wsOut.Start(ctx); err != nil {
		t.Fatalf("start ws output: %v", err)
	}
	t.Cleanup(func() { wsOut.Stop(5 * time.Second) })

	// Give the HTTP server a moment to finish binding its port before the
	// gorilla client dials. 200 ms is consistent with the semstreams integration
	// test suite's own convention for this step.
	time.Sleep(200 * time.Millisecond)

	// --- WebSocket client -----------------------------------------------
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", wsPort)
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial: %v", err)
	}
	t.Cleanup(func() { wsConn.Close() })

	// --- Doc source and engine ------------------------------------------
	docDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(docDir, "hello.md"),
		[]byte("# Hello\nWorld"),
		0644,
	); err != nil {
		t.Fatalf("write hello.md: %v", err)
	}

	cfg := &config.Config{
		Namespace: "testorg",
		Sources: []config.SourceEntry{
			{Type: "docs", Path: docDir},
		},
		Flow: config.FlowConfig{
			Outputs: []config.OutputConfig{
				{Name: "ws", Type: "network", Subject: wsURL},
			},
			DeliveryMode: "at-most-once",
			AckTimeout:   "5s",
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	emitter := &natsEmitter{client: tc.Client, subject: subject}
	norm := normalizer.New(normalizer.Config{Org: "testorg"})

	eng := engine.NewEngine(cfg, emitter, logger, engine.WithNormalizer(norm))
	eng.RegisterHandler(dochandler.New())

	engineCtx, engineCancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		engineCancel()
		eng.Stop()
	})

	if err := eng.Start(engineCtx); err != nil {
		t.Fatalf("engine start: %v", err)
	}

	// --- Receive and validate -------------------------------------------
	// A 10-second deadline covers NATS round-trip latency and any scheduling
	// jitter on the CI host.
	if err := wsConn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	_, msgData, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("WS read: %v", err)
	}

	// The WebSocket output wraps every NATS message in a MessageEnvelope.
	var envelope wsoutput.MessageEnvelope
	if err := json.Unmarshal(msgData, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if envelope.Type != "data" {
		t.Fatalf("envelope.Type = %q, want %q", envelope.Type, "data")
	}

	// The envelope payload is the raw JSON-encoded federation.Event published
	// by the natsEmitter.
	var event federation.Event
	if err := json.Unmarshal(envelope.Payload, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if event.Type != federation.EventTypeSEED {
		t.Errorf("event.Type = %q, want %q", event.Type, federation.EventTypeSEED)
	}

	if len(event.Entities) == 0 {
		t.Error("SEED event has no entities; expected at least one from hello.md")
	}

	if event.Namespace != "testorg" {
		t.Errorf("event.Namespace = %q, want %q", event.Namespace, "testorg")
	}

	t.Logf("received SEED event with %d entities, namespace=%q",
		len(event.Entities), event.Namespace)
}
