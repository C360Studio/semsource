package docsource

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
)

// The verbatim body store is a STARTUP requirement, not a best-effort facet.
// Without it every passage loses its handle and its embedding, so doc_context
// returns empty bodies and the semantic index holds no document text — while the
// component reports itself healthy. Start therefore fails rather than coming up
// degraded, and these tests pin that.
//
// No live NATS is needed: objectstore.NewStoreWithConfig asks the client for its
// JetStream context first, and a client that was never connected returns
// "JetStream not initialized" from that call. That is the same failure a real
// deployment hits when JetStream is unavailable, reached without a broker. The
// client must be a non-nil zero value — a nil *natsclient.Client dereferences
// inside JetStream() and would panic instead of erroring.

// nopPublisher satisfies entitypub.NATSPublisher without a network. The drain
// loop never has anything to publish in these tests: Start fails before ingest.
type nopPublisher struct{}

func (nopPublisher) PublishToStream(context.Context, string, []byte) error { return nil }

// unstartedComponent builds a doc-source Component whose NATS client was never
// connected, so the body store cannot be created.
func unstartedComponent(t *testing.T, root string) *Component {
	t.Helper()

	pub, err := entitypub.New(nopPublisher{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("build entity publisher: %v", err)
	}
	return &Component{
		name:          "doc-source",
		config:        Config{Org: "acme", Paths: []string{root}, WatchEnabled: false},
		publisher:     pub,
		distinct:      entitypub.NewDistinctTracker(),
		natsClient:    &natsclient.Client{},
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		passageCounts: make(map[string]int),
	}
}

// TestComponentStart_FailsWhenBodyStoreUnavailable pins the hard failure. The
// component must not reach its ingest at all: coming up without the store would
// walk the whole corpus and publish entities whose bodies are unretrievable.
func TestComponentStart_FailsWhenBodyStoreUnavailable(t *testing.T) {
	c := unstartedComponent(t, t.TempDir())
	defer c.publisher.Stop()

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil with an unavailable body store, want an error; the component must not come up without it")
	}
	if !strings.Contains(err.Error(), graph.BodyStoreBucket) {
		t.Errorf("Start() error = %v, want it to name the bucket %q so an operator can see what is missing",
			err, graph.BodyStoreBucket)
	}
}

// TestComponentStart_StaysUnhealthyWhenBodyStoreUnavailable is the complement:
// a failed Start must leave the component visibly down. Reporting Healthy after
// returning an error is the silent-degradation shape the hard failure exists to
// remove — a supervisor polling health would see nothing wrong.
func TestComponentStart_StaysUnhealthyWhenBodyStoreUnavailable(t *testing.T) {
	c := unstartedComponent(t, t.TempDir())
	defer c.publisher.Stop()

	if err := c.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil with an unavailable body store, want an error")
	}

	if health := c.Health(); health.Healthy {
		t.Errorf("Health().Healthy = true after a failed Start, want false (status %q)", health.Status)
	}
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()
	if running {
		t.Error("component reports running = true after a failed Start, want false")
	}
	if published := c.entitiesPublished.Load(); published != 0 {
		t.Errorf("entities published after a failed Start = %d, want 0; Start must fail before it ingests anything",
			published)
	}
}
