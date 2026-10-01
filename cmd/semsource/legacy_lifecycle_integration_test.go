//go:build integration

package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

func TestIntegrationLegacyLifecycleCompatibility(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := checkLegacyLifecycle(ctx, tc.Client); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.Client.GetKeyValueBucket(ctx, legacyLifecycleBucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("absent probe created bucket: %v", err)
	}
	bucket, err := tc.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: legacyLifecycleBucket, History: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := checkLegacyLifecycle(ctx, tc.Client); err != nil {
		t.Fatal(err)
	}
	reset := func(t *testing.T) {
		t.Helper()
		if err := tc.Client.DeleteKeyValueBucket(ctx, legacyLifecycleBucket); err != nil {
			t.Fatal(err)
		}
		var err error
		bucket, err = tc.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: legacyLifecycleBucket, History: 1})
		if err != nil {
			t.Fatal(err)
		}
		status, err := bucket.Status(ctx)
		if err != nil || status.Values() != 0 {
			t.Fatalf("fixture not empty: %v", err)
		}
		if err := checkLegacyLifecycle(ctx, tc.Client); err != nil {
			t.Fatal(err)
		}
	}
	for _, sample := range []struct{ key, value string }{
		{"unknown", `{"effect":{"outcome":"unknown"}}`},
		{"terminal", `{"effect":{"outcome":"verified"}}`},
		{"foreign", `{"authority":{"org":"other","platform":"other"}}`},
		{"corrupt", `not-json`},
	} {
		t.Run(sample.key, func(t *testing.T) {
			reset(t)
			if _, err := bucket.Put(ctx, sample.key, []byte(sample.value)); err != nil {
				t.Fatal(err)
			}
			before, err := bucket.Status(ctx)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := bucket.Get(ctx, sample.key)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkLegacyLifecycle(ctx, tc.Client); err == nil {
				t.Fatal("retained state admitted")
			}
			after, err := bucket.Status(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got, err := bucket.Get(ctx, sample.key)
			if err != nil {
				t.Fatal(err)
			}
			if before.Values() != after.Values() || got.Revision() != entry.Revision() || string(got.Value()) != sample.value {
				t.Fatal("refusal changed retained state")
			}
		})
	}
	reset(t)
	if _, err := bucket.Put(ctx, "tombstone", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := bucket.Delete(ctx, "tombstone"); err != nil {
		t.Fatal(err)
	}
	before, err := bucket.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkLegacyLifecycle(ctx, tc.Client); err == nil {
		t.Fatal("retained tombstone admitted")
	}
	after, err := bucket.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Values() != after.Values() {
		t.Fatal("tombstone refusal changed storage")
	}
	if err := tc.Client.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := checkLegacyLifecycle(ctx, tc.Client); err == nil {
		t.Fatal("disconnected storage treated as absent")
	}
}

func TestRootRefusesBeforeProvisioning(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	bucket, err := tc.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: legacyLifecycleBucket})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bucket.Put(ctx, "unknown", []byte("retained")); err != nil {
		t.Fatal(err)
	}
	// setupNATS owns ordinary stream provisioning. A refusal here must precede it.
	nc, _, err := setupNATS(ctx, tc.URL, &config.Config{Namespace: "acme"}, slog.Default(), 0, nil)
	if err == nil || nc != nil {
		t.Fatal("root admitted incompatible state")
	}
	if _, err := tc.Client.GetStream(ctx, "GRAPH"); !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Fatalf("root provisioned GRAPH before guard: %v", err)
	}
	entry, err := bucket.Get(ctx, "unknown")
	if err != nil || string(entry.Value()) != "retained" {
		t.Fatalf("root changed legacy record: %v", err)
	}
}
