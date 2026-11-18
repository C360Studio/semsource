package gitsource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
	srcconfig "github.com/c360studio/semsource/config"
)

// TestNewComponent_DeliversTheEnvironmentToken: the token is no longer in the
// component configuration, so the git source must still send it from the
// environment when it clones (#272). The remote records the Authorization
// header git sends; the clone itself is expected to fail.
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The git token is resolved from the environment, never persisted into configuration
func TestNewComponent_DeliversTheEnvironmentToken(t *testing.T) {
	const probe = "probe-token-272"
	t.Setenv(srcconfig.EnvGitToken, probe)

	var mu sync.Mutex
	var auth []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		http.NotFound(w, r)
	}))
	defer remote.Close()

	raw, err := json.Marshal(map[string]any{
		"org":           "acme",
		"repo_url":      remote.URL + "/acme/private.git",
		"branch":        "main",
		"workspace_dir": t.TempDir(),
		"instance_name": "git-source-private",
	})
	if err != nil {
		t.Fatal(err)
	}
	disc, err := NewComponent(raw, component.Dependencies{Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := disc.(*Component)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, err := c.handler.Ingest(ctx, c.sourceCfg); err == nil {
		t.Fatal("clone from a remote that serves nothing succeeded")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, got := range auth {
		if got == "bearer "+probe {
			return
		}
	}
	t.Fatalf("the remote never received the environment token; Authorization headers seen: %q", auth)
}
