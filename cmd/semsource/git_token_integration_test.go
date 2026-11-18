//go:build integration

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semsource/internal/natstest"
)

// TestIntegrationNewGenerationScrubsRetainedGitToken: a store written by a
// build that put the token in git-source configurations holds it in the
// bucket. The first boot at the next generation must leave the latest
// revision of every component key without it (#272).
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The git token is resolved from the environment, never persisted into configuration
func TestIntegrationNewGenerationScrubsRetainedGitToken(t *testing.T) {
	const probe = "probe-token-272"
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", `[{"type": "git", "url": "https://github.com/acme/private", "branch": "main"}]`))
	first := bootConfigFile(ctx, t, broker.URL, path)

	var gitKey string
	for name := range first.effective.Components {
		if strings.HasPrefix(name, "git-source") {
			gitKey = "components." + name
		}
	}
	if gitKey == "" {
		t.Fatal("the boot generated no git-source component")
	}

	// Rewrite the store as the previous generation left it: an older version
	// and a git-source key that carries the token.
	nc, err := natsclient.NewClient(broker.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nc.Close(ctx) }()
	bucket, err := semconfig.BucketName("acme", "revision")
	if err != nil {
		t.Fatal(err)
	}
	kv, err := nc.GetKeyValueBucket(ctx, bucket)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := kv.Get(ctx, gitKey)
	if err != nil {
		t.Fatal(err)
	}
	var comp map[string]json.RawMessage
	if err := json.Unmarshal(entry.Value(), &comp); err != nil {
		t.Fatal(err)
	}
	var inner map[string]any
	if err := json.Unmarshal(comp["config"], &inner); err != nil {
		t.Fatal(err)
	}
	inner["git_token"] = probe
	comp["config"] = mustJSON(t, inner)
	if _, err := kv.Put(ctx, gitKey, mustJSON(t, comp)); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(ctx, "version", mustJSON(t, "4.0.0")); err != nil {
		t.Fatal(err)
	}

	bootConfigFile(ctx, t, broker.URL, path)

	keys, err := kv.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(entry.Value()), probe) {
			t.Errorf("%s still carries the token after a boot at generation %d: %s", key, compositionGeneration, entry.Value())
		}
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
