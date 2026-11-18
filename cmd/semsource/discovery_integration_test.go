//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/service"

	"github.com/c360studio/semsource/internal/natstest"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// TestIntegrationDiscoveryBootSeam proves the seam boot-time discovery uses
// (design D1): a component put into the config manager after arbitration and
// before the service manager is built is in the snapshot the component
// manager constructs from (semengine service/component_manager.go:188), on a
// retained store at an equal version where the file itself would not win.
//
// spec: openspec/specs/source-discovery/spec.md#Requirement: Discovery takes effect at the boot that finds it
func TestIntegrationDiscoveryBootSeam(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", guideSource))
	bootConfigFile(ctx, t, broker.URL, path) // first boot writes the file to the bucket

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg, _, err := loadAndExpandConfig(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	nc, ssCfg, err := setupNATS(ctx, broker.URL, cfg, logger, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nc.Close(ctx) }()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configMgr.Stop(ctx) }()

	added := configMgr.GetConfig().Get().Components["doc-source-guide"]
	if err := configMgr.PutComponentToKV(ctx, "doc-source-discovered", added); err != nil {
		t.Fatal(err)
	}
	registry, err := prepareComponentRegistry(ctx, cfg, configMgr, nc)
	if err != nil {
		t.Fatal(err)
	}
	payloadReg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manager, err := createServiceManager(nc, registry, payloadReg, configMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, ok := manager.GetService("component-manager")
	if !ok {
		t.Fatal("no component-manager service")
	}
	cm, ok := svc.(*service.ComponentManager)
	if !ok {
		t.Fatalf("component-manager is %T", svc)
	}
	if _, ok := cm.GetComponentStatus()["doc-source-discovered"]; !ok {
		t.Fatal("a component put before the service manager is built was not created for this boot")
	}
}

// bootWithDiscovery runs the boot path through discovery reconciliation and
// returns the components the service manager would be built from, and what
// the runtime check would compare against.
func bootWithDiscovery(ctx context.Context, t *testing.T, natsURL, path string) (semconfig.ComponentConfigs, []sourcemanifest.DiscoveryWatch) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg, expand, err := loadAndExpandConfig(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	nc, ssCfg, err := setupNATS(ctx, natsURL, cfg, logger, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nc.Close(ctx) }()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configMgr.Stop(ctx) }()
	watches, err := admitDiscovered(ctx, cfg, expand, ssCfg.Components, configMgr, logger)
	if err != nil {
		t.Fatal(err)
	}
	return configMgr.GetConfig().Get().Components, watches
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.com", "GIT_ALLOW_PROTOCOL=file")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "README.md"), "# fixture\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "init")
	return dir
}

func hasPrefixed(components semconfig.ComponentConfigs, want string) bool {
	for name, cc := range components {
		if cc.Enabled && strings.Contains(name, want) {
			return true
		}
	}
	return false
}

// TestIntegrationDiscoveryAcrossRestarts is the #216 regression on a retained
// store at an equal version: a branch created between boots is admitted, a
// deleted one leaves the composition, and a materialized submodule expands at
// boot, none of which needs a config_revision bump.
//
// spec: openspec/specs/source-discovery/spec.md#Requirement: Discovery takes effect at the boot that finds it
func TestIntegrationDiscoveryAcrossRestarts(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	repo := newGitRepo(t)
	gitIn(t, repo, "branch", "feature/a")
	sub := newGitRepo(t)
	app := newGitRepo(t)
	gitIn(t, app, "-c", "protocol.file.allow=always", "submodule", "add", sub, "libs/sub")
	gitIn(t, app, "commit", "-m", "add submodule")

	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", `[
		{"type": "repo", "path": "`+repo+`", "branches": ["feature/*"]},
		{"type": "repo", "path": "`+app+`"}]`))

	first, watches := bootWithDiscovery(ctx, t, broker.URL, path)
	if !hasPrefixed(first, "feature-a") {
		t.Fatal("first boot did not admit feature/a")
	}
	// The runtime check compares against exactly what this boot admitted.
	if len(watches) != 2 || !maps.Equal(watches[0].Admitted, map[string]string{"feature/a": ""}) ||
		watches[1].Kind != "submodule" || len(watches[1].Admitted) != 1 || watches[1].Admitted["libs/sub"] == "" {
		t.Fatalf("boot set = %+v; want feature/a and the libs/sub pin", watches)
	}
	if !hasPrefixed(first, "-via-") {
		t.Fatal("first boot did not expand the submodule")
	}

	gitIn(t, repo, "branch", "feature/b")
	second, _ := bootWithDiscovery(ctx, t, broker.URL, path)
	if !hasPrefixed(second, "feature-b") {
		t.Fatal("a branch created between boots was not admitted on the retained store")
	}

	gitIn(t, repo, "worktree", "remove", "--force", filepath.Join(filepath.Dir(repo), ".semsource-worktrees", "feature-a"))
	gitIn(t, repo, "branch", "-D", "feature/a")
	third, _ := bootWithDiscovery(ctx, t, broker.URL, path)
	if hasPrefixed(third, "feature-a") {
		t.Fatal("a deleted branch's components were still admitted")
	}
	if !hasPrefixed(third, "feature-b") || !hasPrefixed(third, "-via-") {
		t.Fatal("unrelated discovered components were lost")
	}
}

// TestIntegrationDiscoveryAcrossFileWinsBoot: a boot where the file wins
// arbitration (a config_revision bump) rewrites the next-boot manifest from
// the file and so starts the ownership record over. A branch deleted before
// that boot must still leave for good: the prune that follows the push
// (pruneRetainedComponents) deletes every bucket component the file lacks, so
// the next, bucket-wins boot has no stale key to run. Discovered children the
// file never generates (submodules) are pruned and re-admitted the same boot.
func TestIntegrationDiscoveryAcrossFileWinsBoot(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	repo := newGitRepo(t)
	gitIn(t, repo, "branch", "feature/a")
	gitIn(t, repo, "branch", "feature/b")
	sub := newGitRepo(t)
	app := newGitRepo(t)
	gitIn(t, app, "-c", "protocol.file.allow=always", "submodule", "add", sub, "libs/sub")
	gitIn(t, app, "commit", "-m", "add submodule")
	sources := `[
		{"type": "repo", "path": "` + repo + `", "branches": ["feature/*"]},
		{"type": "repo", "path": "` + app + `"}]`
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", sources))

	if first, _ := bootWithDiscovery(ctx, t, broker.URL, path); !hasPrefixed(first, "feature-a") || !hasPrefixed(first, "-via-") {
		t.Fatal("first boot did not admit feature/a and the submodule")
	}

	gitIn(t, repo, "worktree", "remove", "--force", filepath.Join(filepath.Dir(repo), ".semsource-worktrees", "feature-a"))
	gitIn(t, repo, "branch", "-D", "feature/a")
	writeFile(t, path, revisionFile(`"config_revision": 1,`, sources))
	second, _ := bootWithDiscovery(ctx, t, broker.URL, path) // the file wins
	if hasPrefixed(second, "feature-a") || !hasPrefixed(second, "feature-b") || !hasPrefixed(second, "-via-") {
		t.Fatal("file-wins boot: want feature/b and the submodule admitted, feature/a gone")
	}

	third, _ := bootWithDiscovery(ctx, t, broker.URL, path) // the bucket wins
	if hasPrefixed(third, "feature-a") {
		t.Fatal("a branch deleted before a file-wins boot came back from the bucket")
	}
	if !hasPrefixed(third, "feature-b") || !hasPrefixed(third, "-via-") {
		t.Fatal("bucket-wins boot after a file-wins boot lost discovered components")
	}
}
