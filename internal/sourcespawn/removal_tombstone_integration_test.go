//go:build integration

package sourcespawn

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
)

func TestIntegrationRemovedFileSourceStaysDisabledAfterConfigRestart(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const handle = "url-source-example-com"
	start := func() *semconfig.Manager {
		cfg := newFakeStore().GetConfig().Get()
		cfg.Version = "3.0.0"
		cfg.Components[handle] = types.ComponentConfig{Name: "url-source", Type: "processor", Enabled: true, Config: json.RawMessage(`{"urls":["https://example.com"]}`)}
		mgr, err := semconfig.NewConfigManager(cfg, tc.Client, slog.Default())
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.Start(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := mgr.Stop(time.Second); err != nil {
				t.Error(err)
			}
		})
		return mgr
	}
	first := start()
	if err := Remove(ctx, handle, first); err != nil {
		t.Fatal(err)
	}
	if err := first.Stop(time.Second); err != nil {
		t.Fatal(err)
	}
	second := start() // The original file still declares this source enabled.
	if source, exists := second.GetConfig().Get().Components[handle]; !exists || source.Enabled {
		t.Fatalf("original file source resurrected after removal: %+v exists=%v", source, exists)
	}
	added, err := Add(ctx, config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}, second, Options{Org: "test"})
	if err != nil || len(added) != 1 || !added[0].Created {
		t.Fatalf("re-add=%+v err=%v", added, err)
	}
	if err := second.Stop(time.Second); err != nil {
		t.Fatal(err)
	}
	third := start()
	if !third.GetConfig().Get().Components[handle].Enabled {
		t.Fatal("re-added source did not reactivate at next boot")
	}
}
