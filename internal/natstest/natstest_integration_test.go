//go:build integration

package natstest

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// spec: openspec/changes/nats-test-fixture/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestNew_CoreOnlyBrokerHasNoJetStream(t *testing.T) {
	srv := New(t)

	js, err := srv.Client.JetStream()
	if err != nil {
		t.Fatalf("jetstream handle: %v", err)
	}
	if _, err := js.AccountInfo(t.Context()); !errors.Is(err, jetstream.ErrJetStreamNotEnabled) {
		t.Fatalf("AccountInfo error = %v, want ErrJetStreamNotEnabled", err)
	}
}

// spec: openspec/changes/nats-test-fixture/specs/integration-fixture-reproducibility/spec.md#Requirement: NATS integration fixture is SemSource-owned and bounded
func TestNew_StreamsAreCreatedBounded(t *testing.T) {
	srv := New(t, WithStreams(Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}))

	js, err := srv.Client.JetStream()
	if err != nil {
		t.Fatalf("jetstream handle: %v", err)
	}
	stream, err := js.Stream(t.Context(), "GRAPH")
	if err != nil {
		t.Fatalf("stream GRAPH: %v", err)
	}
	cfg := stream.CachedInfo().Config
	if cfg.MaxAge != streamMaxAge || cfg.MaxBytes != streamMaxBytes || cfg.Discard != jetstream.DiscardOld {
		t.Fatalf("stream bounds = (%v, %d, %v), want (%v, %d, DiscardOld)",
			cfg.MaxAge, cfg.MaxBytes, cfg.Discard, streamMaxAge, streamMaxBytes)
	}
	if got := cfg.Subjects; len(got) != 1 || got[0] != "graph.ingest.entity" {
		t.Fatalf("subjects = %v", got)
	}
	if srv.URL == "" {
		t.Fatal("URL is empty")
	}
}
