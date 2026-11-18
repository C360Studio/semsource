//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// discoveryStatus is the slice of /source-manifest/status this test reads.
type discoveryStatus struct {
	Phase   string `json:"phase"`
	Sources []struct {
		InstanceName string `json:"instance_name"`
		EntityCount  int64  `json:"entity_count"`
	} `json:"sources"`
	Discovery *struct {
		RestartRequired bool `json:"restart_required"`
		Sources         []struct {
			Kind    string `json:"kind"`
			State   string `json:"state"`
			Pending []struct {
				Kind   string `json:"kind"`
				Name   string `json:"name"`
				Change string `json:"change"`
			} `json:"pending"`
		} `json:"sources"`
	} `json:"discovery"`
}

func (s discoveryStatus) pending(kind, name, change string) bool {
	if s.Discovery == nil {
		return false
	}
	for _, src := range s.Discovery.Sources {
		for _, p := range src.Pending {
			if p.Kind == kind && p.Name == name && p.Change == change {
				return true
			}
		}
	}
	return false
}

// ingested reports whether a status source whose instance name contains
// fragment has seeded entities.
func (s discoveryStatus) ingested(fragment string) bool {
	for _, src := range s.Sources {
		if strings.Contains(src.InstanceName, fragment) && src.EntityCount > 0 {
			return true
		}
	}
	return false
}

func (s discoveryStatus) hasSource(fragment string) bool {
	for _, src := range s.Sources {
		if strings.Contains(src.InstanceName, fragment) {
			return true
		}
	}
	return false
}

// pollDiscovery GETs the status until ok holds or the timeout passes.
func pollDiscovery(t *testing.T, httpPort int, timeout time.Duration, what string, ok func(discoveryStatus) bool) discoveryStatus {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/source-manifest/status", httpPort)
	deadline := time.Now().Add(timeout)
	var last discoveryStatus
	var raw string
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			var body json.RawMessage
			if json.NewDecoder(resp.Body).Decode(&body) == nil {
				raw = string(body)
				last = discoveryStatus{}
				_ = json.Unmarshal(body, &last)
				if ok(last) {
					resp.Body.Close()
					return last
				}
			}
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("status never showed %s within %s; last status: %s", what, timeout, raw)
	return last
}

func discoGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "protocol.file.allow=always"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// discoRepo creates a repository with one committed markdown file, alone in
// its own directory (a local multi-branch source puts its worktrees beside it).
func discoRepo(t *testing.T, name, doc string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	discoGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	discoGit(t, dir, "add", ".")
	discoGit(t, dir, "commit", "-m", "init")
	return dir
}

// TestE2E_DiscoveryAcrossRestarts drives the real binary through #216's
// contract (task 6.1): a branch and a submodule created while SemSource runs
// are reported as restart_required, are ingested after a restart, and a
// deleted branch's components are gone after the next restart, all on one
// retained store at an unchanged config_revision.
//
// spec: openspec/specs/source-discovery/spec.md#Requirement: Discovery takes effect at the boot that finds it
func TestE2E_DiscoveryAcrossRestarts(t *testing.T) {
	natsURL, cleanup := startNATS(t)
	defer cleanup()
	binPath := buildBinary(t)
	workDir := t.TempDir()
	httpPort, wsPort := freePort(t), freePort(t)

	repo := discoRepo(t, "repo", "# Branch fixture\n\nThe main line.\n")
	app := discoRepo(t, "app", "# App fixture\n\nThe parent of a submodule.\n")
	sub := discoRepo(t, "sub", "# Submodule fixture\n\nLinked into app.\n")

	cfg := map[string]any{
		"namespace": "e2ediscovery",
		"http_port": httpPort,
		"metrics":   map[string]any{"port": freePort(t)},
		"sources": []map[string]any{
			{"type": "repo", "path": repo, "branches": []string{"feature/*"}, "branch_poll_interval": "2s", "watch": false},
			{"type": "repo", "path": app, "watch": false},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, "semsource.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	boot := func() func() {
		stop := upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
		upgradeWaitForReady(t, httpPort, 180*time.Second)
		return stop
	}

	// Boot 1: nothing discovered yet, nothing pending.
	stop := boot()
	pollDiscovery(t, httpPort, 30*time.Second, "an empty discovery block", func(s discoveryStatus) bool {
		return s.Discovery != nil && !s.Discovery.RestartRequired && len(s.Discovery.Sources) == 2
	})

	discoGit(t, repo, "branch", "feature/a")
	discoGit(t, app, "submodule", "add", sub, "libs/sub")
	discoGit(t, app, "commit", "-m", "add submodule")
	pollDiscovery(t, httpPort, 60*time.Second, "the new branch and submodule pending", func(s discoveryStatus) bool {
		return s.Discovery != nil && s.Discovery.RestartRequired &&
			s.pending("branch", "feature/a", "added") && s.pending("submodule", "libs/sub", "added")
	})
	if hasNew := pollDiscovery(t, httpPort, time.Second, "status", func(discoveryStatus) bool { return true }); hasNew.hasSource("feature-a") || hasNew.hasSource("-via-") {
		t.Fatal("a discovered component started before a restart")
	}
	stop()

	// Boot 2: both are admitted and ingested; nothing is pending.
	stop = boot()
	pollDiscovery(t, httpPort, 60*time.Second, "the branch and submodule ingested", func(s discoveryStatus) bool {
		return s.ingested("feature-a") && s.ingested("-via-") && s.Discovery != nil && !s.Discovery.RestartRequired
	})

	worktree := filepath.Join(filepath.Dir(repo), ".semsource-worktrees", "feature-a")
	discoGit(t, repo, "worktree", "remove", "--force", worktree)
	discoGit(t, repo, "branch", "-D", "feature/a")
	pollDiscovery(t, httpPort, 60*time.Second, "the deleted branch pending removal", func(s discoveryStatus) bool {
		return s.Discovery != nil && s.Discovery.RestartRequired && s.pending("branch", "feature/a", "removed")
	})
	stop()

	// Boot 3: the deleted branch's components are gone; the submodule stays.
	stop = boot()
	defer stop()
	final := pollDiscovery(t, httpPort, 60*time.Second, "the submodule still ingested", func(s discoveryStatus) bool {
		return s.ingested("-via-") && s.Discovery != nil && !s.Discovery.RestartRequired
	})
	if final.hasSource("feature-a") {
		t.Fatalf("the deleted branch's components are still running: %+v", final.Sources)
	}
}
