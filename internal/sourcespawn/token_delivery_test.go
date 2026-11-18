package sourcespawn

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestGitComponentConfig_ResolvesTheBranchWithTheEnvironmentToken: the
// default-branch lookup for a remote with no branch must authenticate with
// the environment token, since the token is no longer passed in (#272).
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The git token is resolved from the environment, never persisted into configuration
func TestGitComponentConfig_ResolvesTheBranchWithTheEnvironmentToken(t *testing.T) {
	const probe = "probe-token-272"
	t.Setenv(config.EnvGitToken, probe)

	var mu sync.Mutex
	var auth []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		http.NotFound(w, r)
	}))
	defer remote.Close()

	_, comp, err := gitComponentConfig(t.Context(), config.SourceEntry{Type: "git", URL: remote.URL + "/acme/private.git"}, "acme",
		Options{Org: "acme", WorkspaceDir: t.TempDir()})
	if err != nil {
		t.Fatalf("gitComponentConfig: %v", err)
	}
	if _, ok := comp["git_token"]; ok {
		t.Fatal("component configuration carries git_token")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, got := range auth {
		if got == "bearer "+probe {
			return
		}
	}
	t.Fatalf("the default-branch lookup never sent the environment token; Authorization headers seen: %q", auth)
}
