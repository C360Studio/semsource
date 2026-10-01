//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"
)

func TestIntegrationReadDesiredConfigRetainedAuthority(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	want := types.ComponentConfig{
		Name: "doc-source", Type: types.ComponentTypeProcessor, Enabled: false,
		Config: json.RawMessage(`{"paths":["/retained/docs"],"project":"retained","watch":true}`),
	}
	// Use the actual owner to write a complete disabled envelope and sanitize its
	// handle. The manager is stopped before observing read-only behavior.
	kv := desiredReadFixture(ctx, t, tc.Client, "acme", "reader", "retained docs", want)
	before := desiredReadSnapshot(ctx, t, kv)
	read, err := readDesiredConfig(ctx, tc.Client, "acme", "reader")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("exact_disabled_envelope", func(t *testing.T) {
		got, found, err := read(ctx, "retained docs")
		if err != nil || !found || got.Name != want.Name || got.Type != want.Type ||
			got.Enabled != want.Enabled || !bytes.Equal(got.Config, want.Config) {
			t.Fatalf("read = %+v, found=%t, err=%v; want %+v", got, found, err, want)
		}
	})
	t.Run("missing_key", func(t *testing.T) {
		got, found, err := read(ctx, "not-present")
		if err != nil || found || !reflect.DeepEqual(got, types.ComponentConfig{}) {
			t.Fatalf("missing key = %+v, found=%t, err=%v", got, found, err)
		}
	})
	if after := desiredReadSnapshot(ctx, t, kv); !reflect.DeepEqual(before, after) {
		t.Fatal("successful/missing reads changed retained configuration")
	}

	// Malformed data is a deliberate fixture in this test-owned bucket, not a
	// repair performed by the reader or a simulated read failure.
	if _, err := kv.Put(ctx, "components.malformed", []byte(`{"name":`)); err != nil {
		t.Fatal(err)
	}
	before = desiredReadSnapshot(ctx, t, kv)
	if _, found, err := read(ctx, "malformed"); err == nil || found {
		t.Fatalf("malformed record granted authority: found=%t, err=%v", found, err)
	}
	if after := desiredReadSnapshot(ctx, t, kv); !reflect.DeepEqual(before, after) {
		t.Fatal("malformed read changed retained configuration")
	}
	desiredReadNoLegacy(ctx, t, tc.Client)
}

func TestIntegrationReadDesiredConfigNamespaceIsolation(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for _, owner := range []struct{ org, stem, project string }{
		{"acme", "reader", "first"},
		{"other", "reader", "second"},
		{"acme", "separate", "third"},
	} {
		want := types.ComponentConfig{
			Name: "doc-source", Type: types.ComponentTypeProcessor, Enabled: true,
			Config: json.RawMessage(`{"project":"` + owner.project + `"}`),
		}
		kv := desiredReadFixture(ctx, t, tc.Client, owner.org, owner.stem, "same-handle", want)
		before := desiredReadSnapshot(ctx, t, kv)
		read, err := readDesiredConfig(ctx, tc.Client, owner.org, owner.stem)
		if err != nil {
			t.Fatal(err)
		}
		got, found, err := read(ctx, "same-handle")
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s/%s read = %+v, found=%t, err=%v; want %+v", owner.org, owner.stem, got, found, err, want)
		}
		if after := desiredReadSnapshot(ctx, t, kv); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s/%s read changed retained configuration", owner.org, owner.stem)
		}
	}
	desiredReadNoLegacy(ctx, t, tc.Client)
}

func TestIntegrationReadDesiredConfigUnavailableStorage(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if read, err := readDesiredConfig(ctx, tc.Client, "acme", "missing"); err == nil || read != nil {
		t.Fatalf("missing authority yielded reader: err=%v", err)
	}
	name, err := semconfig.BucketName("acme", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tc.Client.GetKeyValueBucket(ctx, name); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("reader provisioned missing configuration bucket: %v", err)
	}
	if read, err := readDesiredConfig(ctx, tc.Client, "", "reader"); err == nil || read != nil {
		t.Fatalf("invalid namespace yielded reader: err=%v", err)
	}
	want := types.ComponentConfig{
		Name: "doc-source", Type: types.ComponentTypeProcessor, Enabled: false,
		Config: json.RawMessage(`{"project":"retained"}`),
	}
	kv := desiredReadFixture(ctx, t, tc.Client, "acme", "reader", "retained", want)
	before := desiredReadSnapshot(ctx, t, kv)
	// Close a separate real connection so the original client can independently
	// prove that failing reads leave both configuration and legacy storage alone.
	client, err := natsclient.NewClient(tc.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := client.Close(cleanupCtx); err != nil {
			t.Error(err)
		}
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := readDesiredConfig(ctx, client, "acme", "reader")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := read(ctx, "retained"); err == nil || found {
		t.Fatalf("closed connection granted authority: found=%t, err=%v", found, err)
	}
	if read, err := readDesiredConfig(ctx, client, "acme", "reader"); err == nil || read != nil {
		t.Fatalf("closed connection opened authority reader: err=%v", err)
	}
	if after := desiredReadSnapshot(ctx, t, kv); !reflect.DeepEqual(before, after) {
		t.Fatal("failed reads changed retained configuration")
	}
	desiredReadNoLegacy(ctx, t, tc.Client)
}

func desiredReadFixture(ctx context.Context, t *testing.T, client *natsclient.Client, org, stem, handle string, cc types.ComponentConfig) jetstream.KeyValue {
	t.Helper()
	manager, err := semconfig.NewConfigManager(&semconfig.Config{
		Version: "1.0.0", Platform: semconfig.PlatformConfig{Org: org, ID: stem},
		Components: semconfig.ComponentConfigs{},
	}, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Stop(3 * time.Second); err != nil {
			t.Error(err)
		}
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := manager.PutComponentToKV(ctx, handle, cc); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	name, err := semconfig.BucketName(org, stem)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := client.GetKeyValueBucket(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return kv
}

type desiredReadState struct {
	values  uint64
	entries map[string]struct {
		revision uint64
		value    string
	}
}

func desiredReadSnapshot(ctx context.Context, t *testing.T, kv jetstream.KeyValue) desiredReadState {
	t.Helper()
	status, err := kv.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state := desiredReadState{values: status.Values(), entries: map[string]struct {
		revision uint64
		value    string
	}{}}
	keys, err := kv.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		state.entries[key] = struct {
			revision uint64
			value    string
		}{entry.Revision(), string(entry.Value())}
	}
	return state
}

func desiredReadNoLegacy(ctx context.Context, t *testing.T, client *natsclient.Client) {
	t.Helper()
	if _, err := client.GetKeyValueBucket(ctx, legacyLifecycleBucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("desired read created legacy storage: %v", err)
	}
}
