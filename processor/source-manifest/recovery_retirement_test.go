package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/types"
)

func TestDesiredRemovalIndependentOfGraph(t *testing.T) {
	store := newDesiredStore(t)
	// No graph client: desired management must remain usable while it is unavailable.
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	added := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/docs"}}}, cfg)
	if added.Error != nil || len(added.Components) != 1 {
		t.Fatalf("add failed without graph: %+v", added)
	}
	assertProjectionUnavailable(t, added)
	handle := added.Components[0].InstanceName
	removed := c.removeSource(context.Background(), handle, "test", cfg)
	if removed.Error != nil || !removed.Removed || !removed.DesiredChanged || !removed.RestartRequired || removed.RuntimeChanged || store.components[handle].Enabled {
		t.Fatalf("desired disable failed without graph: %+v", removed)
	}
	assertProjectionUnavailable(t, removed)
}

func TestDesiredReadbackDoesNotInventCommit(t *testing.T) {
	for _, mode := range []string{"unavailable", "different_config"} {
		t.Run(mode, func(t *testing.T) {
			store := &partialDesiredStore{desiredStore: newDesiredStore(t), retained: map[string]types.ComponentConfig{}}
			store.retained["source-manifest"] = store.components["source-manifest"]
			cfg := IngestHandlerConfig{Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
			cfg.ReadDesired = func(_ context.Context, name string) (types.ComponentConfig, bool, error) {
				cc, ok := store.retained[name]
				if name != "source-manifest" && store.failSource && ok {
					if mode == "unavailable" {
						return types.ComponentConfig{}, false, errors.New("retained config unreadable")
					}
					cc.Config = json.RawMessage(`{"urls":["https://other.example"]}`)
				}
				return cc, ok, nil
			}
			store.failSource = true
			c := &Component{logger: slog.Default()}
			reply := c.addSource(context.Background(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}}, cfg)
			if reply.Error == nil || reply.DesiredChanged || reply.RestartRequired || len(reply.Components) != 0 {
				t.Fatalf("unproved commit accepted: %+v", reply)
			}
			if mode == "unavailable" && !strings.Contains(reply.Error.Message, "retained config unreadable") {
				t.Fatalf("lost read failure: %+v", reply)
			}
		})
	}
}

type heldDesiredStore struct {
	*desiredStore
	entered chan struct{}
	release chan struct{}
}

func (s *heldDesiredStore) PutComponentToKV(ctx context.Context, name string, cc types.ComponentConfig) error {
	if name == "url-source-first-example" {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.desiredStore.PutComponentToKV(ctx, name, cc)
}

func TestCanceledQueuedDesiredRequestMakesNoWrites(t *testing.T) {
	store := &heldDesiredStore{desiredStore: newDesiredStore(t), entered: make(chan struct{}), release: make(chan struct{})}
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	first := make(chan *AddReply, 1)
	go func() {
		first <- c.addSource(t.Context(), AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://first.example"}}}, cfg)
	}()
	<-store.entered
	ctx, cancel := context.WithCancel(t.Context())
	second := make(chan *AddReply, 1)
	go func() {
		second <- c.addSource(ctx, AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://second.example"}}}, cfg)
	}()
	cancel()
	if got := <-second; got.Error == nil || got.DesiredChanged {
		t.Fatalf("queued cancel: %+v", got)
	}
	close(store.release)
	if got := <-first; got.Error != nil || !got.DesiredChanged {
		t.Fatalf("admitted request: %+v", got)
	}
	if enabledSourceComponentCount(store.components) != 1 {
		t.Fatalf("queued caller wrote source: %+v", store.components)
	}
}

type partialDesiredStore struct {
	*desiredStore
	retained   map[string]types.ComponentConfig
	failSource bool
}

func (s *partialDesiredStore) PutComponentToKV(ctx context.Context, name string, cc types.ComponentConfig) error {
	if name == "source-manifest" && s.failManifest {
		return errors.New("manifest failed")
	}
	s.retained[name] = cc
	if name != "source-manifest" && s.failSource {
		return errors.New("config committed; memory application failed")
	}
	return s.desiredStore.PutComponentToKV(ctx, name, cc)
}

func TestDesiredPartialCommitReceipt(t *testing.T) {
	for _, source := range []config.SourceEntry{{Type: "url", URLs: []string{"https://example.com/docs"}}, {Type: "repo", Path: "/tmp/repo", Branch: "main"}} {
		t.Run(source.Type, func(t *testing.T) {
			store := &partialDesiredStore{desiredStore: newDesiredStore(t), retained: map[string]types.ComponentConfig{}}
			store.retained["source-manifest"] = store.components["source-manifest"]
			cfg := IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme", WorkspaceDir: "/tmp/work"}, ReadDesired: func(_ context.Context, name string) (types.ComponentConfig, bool, error) {
				cc, ok := store.retained[name]
				return cc, ok, nil
			}}
			c := &Component{logger: slog.Default()}
			store.failSource = true
			added := c.addSource(context.Background(), AddRequest{Source: source}, cfg)
			if added.Error == nil || len(added.Components) != 1 || !added.DesiredChanged || !added.RestartRequired {
				t.Fatalf("partial add lost committed source: %+v", added)
			}
			assertProjectionUnavailable(t, added)
			handle := added.Components[0].InstanceName
			store.failSource = false
			// The request can read the committed source even when the manager's memory never added it.
			store.components[handle] = store.retained[handle]
			store.failSource = true
			removed := c.removeSource(context.Background(), handle, "test", cfg)
			if removed.Error == nil || !removed.Removed || !removed.DesiredChanged || !removed.RestartRequired || store.retained[handle].Enabled {
				t.Fatalf("partial disable lost committed facts: %+v", removed)
			}
			assertProjectionUnavailable(t, removed)
			store.failSource = false
			repeated := c.removeSource(context.Background(), handle, "test", cfg)
			if repeated.Error == nil || repeated.Error.Code != CodeNotFound || repeated.DesiredChanged {
				t.Fatalf("successful manifest repeated remove: %+v", repeated)
			}
		})
	}
}

func TestCanceledAdmissionMakesNoWrites(t *testing.T) {
	store := newDesiredStore(t)
	c := &Component{logger: slog.Default()}
	cfg := IngestHandlerConfig{Store: store, Spawn: sourcespawn.Options{Org: "acme"}}
	before, _ := json.Marshal(store.components)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if reply := c.addSource(ctx, AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}}, cfg); reply.Error == nil || reply.DesiredChanged {
		t.Fatal(reply)
	}
	if reply := c.removeSource(ctx, "docs", "", cfg); reply.Error == nil || reply.DesiredChanged {
		t.Fatal(reply)
	}
	after, _ := json.Marshal(store.components)
	if string(before) != string(after) {
		t.Fatal("canceled admission wrote desired config")
	}
}

func assertProjectionUnavailable(t *testing.T, reply any) {
	t.Helper()
	raw, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["projection_status"] != "unavailable" || wire["generation"] != nil || wire["projection_phase"] != nil {
		t.Fatalf("reply implies private projection: %s", raw)
	}
}

func TestSourceProjectionUnavailable(t *testing.T) {
	cfg := &IngestHandlerConfig{Namespace: "acme", Store: newDesiredStore(t), APIToken: "token"}
	mux := newHTTPComponent(t, cfg, nil)
	path := "/source-manifest/sources/unknown/lifecycle"
	if got := doJSON(t, mux, http.MethodGet, path, nil, nil); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status: %d", got.Code)
	}
	got := doJSON(t, mux, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer token"})
	if got.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	var reply struct {
		Error IngestError `json:"error"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Error.Code != "SOURCE_LIFECYCLE_UNAVAILABLE" {
		t.Fatalf("reply=%+v", reply)
	}
}
