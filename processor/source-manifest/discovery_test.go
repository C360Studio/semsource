package sourcemanifest

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
)

func discoveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.com", "GIT_ALLOW_PROTOCOL=file")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func discoveryRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	discoveryGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	discoveryGit(t, dir, "add", ".")
	discoveryGit(t, dir, "commit", "-m", "init")
	return dir
}

func (s *DiscoveryStatus) pending(name, change string) bool {
	for _, src := range s.Sources {
		for _, p := range src.Pending {
			if p.Name == name && p.Change == change {
				return true
			}
		}
	}
	return false
}

// treeOf lists every path under root, so a test can assert a check left the
// filesystem as it found it.
func treeOf(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	if err := filepath.WalkDir(root, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func branchWatch(repo string, admitted ...string) DiscoveryWatch {
	set := map[string]string{}
	for _, b := range admitted {
		set[b] = ""
	}
	return DiscoveryWatch{
		DiscoveryState: config.DiscoveryState{Source: repo, Kind: "branch", State: config.DiscoveryOK, Admitted: set},
		RepoPath:       repo, Patterns: []string{"feature/*"},
	}
}

// TestCheckDiscovery pins task 4.1: the check reports what a restart would add
// or retire against the boot set, types each failure (D4), never sets
// restart_required for a listing failure (Q2), and touches nothing on disk.
//
// spec: openspec/specs/source-discovery/spec.md#Requirement: Runtime discovery is read-only and reports what a restart would change
func TestCheckDiscovery(t *testing.T) {
	repo := discoveryRepo(t)
	discoveryGit(t, repo, "branch", "feature/a")
	discoveryGit(t, repo, "branch", "feature/b")
	parent := filepath.Dir(repo)
	before := treeOf(t, parent)
	ctx := context.Background()

	t.Run("pending against the boot set", func(t *testing.T) {
		got := checkDiscovery(ctx, []DiscoveryWatch{branchWatch(repo, "feature/a", "feature/gone")})
		want := []DiscoveryChange{
			{Kind: "branch", Name: "feature/b", Change: "added"},
			{Kind: "branch", Name: "feature/gone", Change: "removed"},
		}
		if !got.RestartRequired || !reflect.DeepEqual(got.Sources[0].Pending, want) {
			t.Fatalf("got %+v; want restart_required with %+v", got, want)
		}
	})
	t.Run("nothing pending", func(t *testing.T) {
		got := checkDiscovery(ctx, []DiscoveryWatch{branchWatch(repo, "feature/a", "feature/b")})
		if got.RestartRequired || len(got.Sources[0].Pending) != 0 || got.Sources[0].State != config.DiscoveryOK {
			t.Fatalf("got %+v; want ok with nothing pending", got)
		}
	})
	t.Run("capped", func(t *testing.T) {
		w := branchWatch(repo, "feature/a")
		w.MaxBranches = 1
		got := checkDiscovery(ctx, []DiscoveryWatch{w})
		src := got.Sources[0]
		if src.State != config.DiscoveryCapped || src.NotAdmitted != 1 || got.RestartRequired {
			t.Fatalf("got %+v; want capped, one not admitted, nothing pending", got)
		}
	})
	t.Run("awaiting checkout", func(t *testing.T) {
		got := checkDiscovery(ctx, []DiscoveryWatch{branchWatch(filepath.Join(parent, "absent"))})
		if got.Sources[0].State != config.DiscoveryAwaitingCheckout || got.RestartRequired {
			t.Fatalf("got %+v; want awaiting_checkout", got)
		}
	})
	t.Run("checkout arrived after an awaiting boot", func(t *testing.T) {
		w := branchWatch(repo)
		w.State = config.DiscoveryAwaitingCheckout
		got := checkDiscovery(ctx, []DiscoveryWatch{w})
		if !got.RestartRequired || !got.pending("feature/a", "added") || got.Sources[0].State != config.DiscoveryOK {
			t.Fatalf("got %+v; want the matching branches pending once the checkout exists", got)
		}
		w.Patterns = []string{"release/*"}
		if got := checkDiscovery(ctx, []DiscoveryWatch{w}); got.RestartRequired {
			t.Fatalf("got %+v; want no restart when nothing matches (the clone component stays)", got)
		}
	})
	t.Run("a branch boot could not check out is not pending", func(t *testing.T) {
		w := branchWatch(repo, "feature/b")
		w.MaxBranches, w.Skipped, w.Error = 1, []string{"feature/a"}, "no worktree could be created for feature/a"
		got := checkDiscovery(ctx, []DiscoveryWatch{w})
		src := got.Sources[0]
		if got.RestartRequired || len(src.Pending) != 0 || src.State != config.DiscoveryOK || src.Error != w.Error {
			t.Fatalf("got %+v; want feature/b still admitted, nothing pending, and the boot error shown", got)
		}
	})
	t.Run("submodule parent whose checkout arrived with no pins", func(t *testing.T) {
		w := DiscoveryWatch{RepoPath: repo, DiscoveryState: config.DiscoveryState{
			Source: repo, Kind: "submodule", State: config.DiscoveryAwaitingCheckout,
		}}
		if got := checkDiscovery(ctx, []DiscoveryWatch{w}); got.RestartRequired {
			t.Fatalf("got %+v; want no restart for a checkout without submodules", got)
		}
	})
	t.Run("boot listing failure reports state only", func(t *testing.T) {
		w := branchWatch(repo)
		w.State, w.Error, w.Admitted = config.DiscoveryListingFailed, "boot: for-each-ref failed", nil
		got := checkDiscovery(ctx, []DiscoveryWatch{w})
		src := got.Sources[0]
		if got.RestartRequired || len(src.Pending) != 0 || src.State != config.DiscoveryListingFailed || src.Error != w.Error {
			t.Fatalf("got %+v; want the boot failure with nothing pending", got)
		}
	})
	t.Run("listing failure does not require a restart", func(t *testing.T) {
		got := checkDiscovery(ctx, []DiscoveryWatch{branchWatch(t.TempDir(), "feature/a")})
		src := got.Sources[0]
		if src.State != config.DiscoveryListingFailed || src.Error == "" || got.RestartRequired || len(src.Pending) != 0 {
			t.Fatalf("got %+v; want listing_failed with an error and restart_required false", got)
		}
	})

	if after := treeOf(t, parent); !reflect.DeepEqual(before, after) {
		t.Fatalf("the check changed the filesystem:\nbefore %v\nafter  %v", before, after)
	}
	if wt := discoveryGit(t, repo, "worktree", "list", "--porcelain"); strings.Count(wt, "worktree ") != 1 {
		t.Fatalf("the check created a worktree:\n%s", wt)
	}
}

// TestCheckDiscovery_Submodules pins the submodule side: a pin added, moved,
// or removed since boot is pending as added, changed, or removed.
func TestCheckDiscovery_Submodules(t *testing.T) {
	sub := discoveryRepo(t)
	parent := discoveryRepo(t)
	discoveryGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", sub, "libs/sub")
	discoveryGit(t, parent, "commit", "-m", "add submodule")
	sha := discoveryGit(t, parent, "rev-parse", "HEAD:libs/sub")

	w := DiscoveryWatch{RepoPath: parent, DiscoveryState: config.DiscoveryState{
		Source: parent, Kind: "submodule", State: config.DiscoveryOK,
		Admitted: map[string]string{"libs/sub": strings.Repeat("0", len(sha)), "libs/gone": sha},
	}}
	got := checkDiscovery(context.Background(), []DiscoveryWatch{w})
	want := []DiscoveryChange{
		{Kind: "submodule", Name: "libs/gone", Change: "removed"},
		{Kind: "submodule", Name: "libs/sub", Change: "changed"},
	}
	if !got.RestartRequired || !reflect.DeepEqual(got.Sources[0].Pending, want) {
		t.Fatalf("got %+v; want %+v", got, want)
	}

	w.Admitted = map[string]string{"libs/sub": sha}
	if got := checkDiscovery(context.Background(), []DiscoveryWatch{w}); got.RestartRequired {
		t.Fatalf("got %+v; want nothing pending at the boot pin", got)
	}
}

// TestDiscoveryStatusBlock pins task 4.2: the running check's result is the
// discovery block on the served status (which /source-manifest/status and
// source_status both return), and the block is absent when no source
// discovers. The check holds no configuration store, so it cannot write one;
// TestCheckDiscovery proves it leaves the filesystem and worktrees alone.
//
// spec: openspec/specs/source-discovery/spec.md#Requirement: Runtime discovery is read-only and reports what a restart would change
func TestDiscoveryStatusBlock(t *testing.T) {
	repo := discoveryRepo(t)
	discoveryGit(t, repo, "branch", "feature/a")
	stored := make(chan struct{}, 8)
	c := &Component{
		logger:     slog.Default(),
		aggregator: newStatusAggregator(0),
		statusBuilt: func() {
			select {
			case stored <- struct{}{}:
			default:
			}
		},
	}
	c.config.Namespace = "acme"

	c.refreshStatus()
	<-stored
	var bare map[string]json.RawMessage
	if err := json.Unmarshal(c.statusData, &bare); err != nil {
		t.Fatal(err)
	}
	if _, ok := bare["discovery"]; ok {
		t.Fatalf("discovery present with no discovering source: %s", c.statusData)
	}

	cancel := c.startDiscoveryCheck(context.Background(), []DiscoveryWatch{branchWatch(repo)}, time.Hour)
	<-stored
	cancel()
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := c.workers.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Discovery struct {
			RestartRequired bool `json:"restart_required"`
			Sources         []struct {
				Source  string `json:"source"`
				Kind    string `json:"kind"`
				State   string `json:"state"`
				Pending []struct {
					Kind, Name, Change string
				} `json:"pending"`
			} `json:"sources"`
		} `json:"discovery"`
	}
	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	d := got.Discovery
	if !d.RestartRequired || len(d.Sources) != 1 || d.Sources[0].Source != repo || d.Sources[0].Kind != "branch" ||
		d.Sources[0].State != "ok" || len(d.Sources[0].Pending) != 1 ||
		d.Sources[0].Pending[0].Name != "feature/a" || d.Sources[0].Pending[0].Change != "added" {
		t.Fatalf("discovery block = %s", data)
	}
}
