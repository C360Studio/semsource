package urlhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
)

type watchSourceConfig struct {
	url string
}

func (c watchSourceConfig) GetType() string             { return handler.SourceTypeURL }
func (c watchSourceConfig) GetPath() string             { return "" }
func (c watchSourceConfig) GetPaths() []string          { return nil }
func (c watchSourceConfig) GetURL() string              { return c.url }
func (c watchSourceConfig) GetBranch() string           { return "" }
func (c watchSourceConfig) IsWatchEnabled() bool        { return true }
func (c watchSourceConfig) GetKeyframeMode() string     { return "" }
func (c watchSourceConfig) GetKeyframeInterval() string { return "" }
func (c watchSourceConfig) GetSceneThreshold() float64  { return 0 }
func (c watchSourceConfig) GetPollInterval() string     { return "1h" }

func TestWatchCreateUsesOnlyTypedState(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("initial content"))
	}))
	defer srv.Close()

	h := NewWithClient(nil, srv.Client())
	h.org = "acme"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events, err := h.Watch(ctx, watchSourceConfig{url: srv.URL})
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	select {
	case event := <-events:
		if event.Operation != handler.OperationCreate || event.Path != srv.URL {
			t.Fatalf("create signal changed: %+v", event)
		}
		if len(event.EntityStates) != 1 {
			t.Fatalf("EntityStates count = %d, want 1", len(event.EntityStates))
		}
		if len(event.Entities) != 0 {
			t.Fatalf("RawEntity count = %d, want 0", len(event.Entities))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for URL watch event")
	}
}
