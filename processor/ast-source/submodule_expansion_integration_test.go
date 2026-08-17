//go:build integration

package astsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semsource/internal/subwatch"
)

type recordingStore struct {
	mu   sync.Mutex
	puts map[string]types.ComponentConfig
}

func (r *recordingStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.puts[name] = cfg
	return nil
}

func (r *recordingStore) DeleteComponentFromKV(context.Context, string) error { return nil }

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "protocol.file.allow=always"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// TestIntegration_SubmoduleExpansionSeedsScopedEntities runs the full local
// chain: fixture-shaped dual-pin repo → recursive clone → subwatch expansion
// → the SPAWNED ast component config boots a real ast-source against test
// NATS and seeds entities. This is the seam none of the unit tiers cross:
// strict component decoding must accept exactly what expansion emits, and the
// walk must produce version-scoped entities from the submodule tree.
func TestIntegration_SubmoduleExpansionSeedsScopedEntities(t *testing.T) {
	// Fixture: sub v1 (Greet), v2 adds Farewell; parent pins root@v2, nested@v1.
	sub := filepath.Join(t.TempDir(), "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, sub, "init", "-b", "main")
	os.WriteFile(filepath.Join(sub, "greeter.go"), []byte("package greeter\n\nfunc Greet() string { return \"hi\" }\n"), 0o644)
	gitRun(t, sub, "add", ".")
	gitRun(t, sub, "commit", "-m", "v1")
	shaV1 := gitRun(t, sub, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(sub, "farewell.go"), []byte("package greeter\n\nfunc Farewell() string { return \"bye\" }\n"), 0o644)
	gitRun(t, sub, "add", ".")
	gitRun(t, sub, "commit", "-m", "v2")
	shaV2 := gitRun(t, sub, "rev-parse", "HEAD")

	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, parent, "init", "-b", "main")
	os.WriteFile(filepath.Join(parent, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	gitRun(t, parent, "add", ".")
	gitRun(t, parent, "commit", "-m", "init")
	gitRun(t, parent, "submodule", "add", sub, "sub")
	gitRun(t, parent, "submodule", "add", sub, "nested/sub")
	gitRun(t, filepath.Join(parent, "nested/sub"), "checkout", shaV1)
	gitRun(t, parent, "add", "nested/sub")
	gitRun(t, parent, "commit", "-m", "dual pin")

	clone := filepath.Join(t.TempDir(), "clone")
	gitRun(t, filepath.Dir(clone), "clone", "--recurse-submodules", parent, clone)

	// Expansion.
	store := &recordingStore{puts: map[string]types.ComponentConfig{}}
	w := subwatch.New(subwatch.Config{
		RepoPath:   clone,
		ParentSlug: "parentproj",
		Languages:  []string{"go"},
		Opts:       sourcespawn.Options{Org: "acme"},
		Store:      store,
	})
	w.Tick(context.Background())

	tc := natsclient.NewTestClient(t,
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity", "graph.ingest.batch"},
		}),
	)

	// Boot every spawned ast instance from its EXACT spawned bytes.
	var booted int
	for name, cc := range store.puts {
		if cc.Name != "ast-source" {
			continue
		}
		booted++
		comp, err := NewComponent(cc.Config, component.Dependencies{NATSClient: tc.Client})
		if err != nil {
			t.Fatalf("NewComponent from spawned config %q: %v", name, err)
		}
		c := comp.(*Component)
		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start %q: %v", name, err)
		}
		defer func() { _ = stopWithin(5*time.Second, c.Stop) }()

		deadline := time.Now().Add(20 * time.Second)
		for c.distinct.Count() == 0 && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if c.distinct.Count() == 0 {
			t.Errorf("%q seeded no entities from its submodule tree", name)
		}
	}
	if booted != 2 {
		t.Fatalf("booted %d ast instances, want 2 (one per pin)", booted)
	}

	// The two spawned instance names carry the two distinct version scopes.
	names := make([]string, 0, len(store.puts))
	for n := range store.puts {
		names = append(names, n)
	}
	joined := strings.Join(names, " ")
	for _, sha := range []string{shaV1[:12], shaV2[:12]} {
		if !strings.Contains(joined, sha) {
			t.Errorf("no spawned instance carries version scope %s (names: %v)", sha, names)
		}
	}
}
