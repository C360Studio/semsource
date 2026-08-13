// Package astsource: the repo-corpus tier of the CI determinism gate (#127).
//
// This tier archives the semsource repository itself (via `git archive HEAD`,
// never `git worktree` — the established e2e/scorecard convention: a .git dir
// in the corpus would be walked by handlers and grep alike, and archive gives
// a byte-identical tree on any machine) and runs three two-pass ingestion
// diffs over it in one process:
//
//   - AST across go/typescript/javascript/svelte at repo scale, plus the
//     hierarchy (repo/folder) entities BuildHierarchy derives from the parse
//     results — reusing the same runASTPass helper determinism_test.go uses
//     for the cheap fixture tier, so "how a pass is driven" cannot drift
//     between tiers.
//   - cfgfile.IngestEntityStates over the repo's own go.mod, ui/package.json,
//     and the three Dockerfiles.
//   - doc.IngestEntityStates over the repo's ~300 markdown files.
//
// No build tag: measured locally at ~5.5-6.8s wall clock for the whole
// package (git archive + all three two-pass diffs, -race included — see the
// PR description for the exact numbers), comfortably under the ~90s bar for
// running unconditionally in the standard `go test -race ./...` job rather
// than behind a tag with a separate CI job. Revisit if the corpus or the
// subsystems covered grow enough to change that math.
package astsource

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/cfgfile"
	dochandler "github.com/c360studio/semsource/handler/doc"
)

// repoDetOrg and repoDetProject are fixed identifiers for the repo-corpus
// watch path / source configs. Only routing/parsing determinism is under
// test here, not entity-ID content, so any fixed values work.
const (
	repoDetOrg     = "acme"
	repoDetProject = "semsource-repo"
)

// detSourceConfig adapts a single filesystem path to handler.SourceConfig for
// the cfgfile and doc handlers' IngestEntityStates calls. Neither handler's
// IngestEntityStates consults GetType() (only their Supports(), which this
// gate bypasses by calling IngestEntityStates directly — same as
// cfgfile's own TestIngestEntityStates_Deterministic and doc's
// docsHandler test helper do); the field is still set correctly for clarity.
type detSourceConfig struct {
	typ  string
	path string
}

func (s detSourceConfig) GetType() string             { return s.typ }
func (s detSourceConfig) GetPath() string             { return s.path }
func (s detSourceConfig) GetPaths() []string          { return nil }
func (s detSourceConfig) GetURL() string              { return "" }
func (s detSourceConfig) GetBranch() string           { return "" }
func (s detSourceConfig) IsWatchEnabled() bool        { return false }
func (s detSourceConfig) GetKeyframeMode() string     { return "" }
func (s detSourceConfig) GetKeyframeInterval() string { return "" }
func (s detSourceConfig) GetSceneThreshold() float64  { return 0 }

// detMemStore is a minimal in-memory storage.Store, satisfying the doc
// handler's mandatory body-store dependency (dochandler.ErrBodyStoreRequired).
// Design constraint #6 excludes storage bodies/ObjectStore from the
// determinism claim itself — StorageRef is dropped in fromHandlerState — so
// this only needs to make the walk succeed, not be production-representative.
type detMemStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newDetMemStore() *detMemStore { return &detMemStore{data: make(map[string][]byte)} }

func (s *detMemStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *detMemStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return d, nil
}

func (s *detMemStore) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *detMemStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// repoRoot resolves the top of the git checkout the test is running from.
// Run with no explicit working directory override, this always resolves to
// the checkout `go test` itself is executing in — the isolated worktree
// during local/agent development, the runner's checkout in CI — never a
// sibling checkout, so the archived corpus is always this exact revision.
func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// buildRepoArchiveCorpus exports the current HEAD commit's tree into a fresh
// temp directory via `git archive`, never `git worktree` (repo convention —
// see the file header). Extraction uses Go's archive/tar directly rather than
// shelling out to `tar`, so the corpus builder has no dependency beyond git
// itself.
func buildRepoArchiveCorpus(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)

	cmd := exec.Command("git", "archive", "HEAD")
	cmd.Dir = root
	archive, err := cmd.Output()
	if err != nil {
		t.Fatalf("git archive HEAD: %v", err)
	}

	dir := t.TempDir()
	if err := extractTar(bytes.NewReader(archive), dir); err != nil {
		t.Fatalf("extract archive into %s: %v", dir, err)
	}
	return dir
}

// extractTar writes every regular file and directory in a tar stream under
// dest, rejecting any entry whose name would resolve outside dest (a git
// archive of our own HEAD never produces one, but the check is cheap
// insurance against a corrupted or unexpected stream).
func extractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(filepath.Separator)) {
			return fmt.Errorf("tar entry %q escapes destination %s", hdr.Name, dest)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeTarFile(target, hdr.Mode, tr); err != nil {
				return err
			}
		}
	}
}

// writeTarFile materializes one regular file from an open tar entry reader.
func writeTarFile(target string, mode int64, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// runCfgfilePass drives cfgfile.ConfigHandler.IngestEntityStates — the real
// production entry point processor/cfgfile-source calls — over root, and
// returns every entity state in comparison-normalized form.
func runCfgfilePass(ctx context.Context, t *testing.T, root string) []detEntity {
	t.Helper()
	h := cfgfile.New(nil)
	cfg := detSourceConfig{typ: handler.SourceTypeConfig, path: root}
	states, err := h.IngestEntityStates(ctx, cfg, repoDetOrg)
	if err != nil {
		t.Fatalf("cfgfile IngestEntityStates: %v", err)
	}
	out := make([]detEntity, 0, len(states))
	for _, s := range states {
		out = append(out, fromHandlerState(s))
	}
	return out
}

// runDocPass drives doc.Handler.IngestEntityStates — the real production
// entry point processor/doc-source calls — over root, and returns every
// entity state in comparison-normalized form. Each pass gets its own
// detMemStore: the body store's content-addressed keys are deterministic by
// construction, but a fresh store per pass keeps this test's own state from
// being a hidden channel between passes.
func runDocPass(ctx context.Context, t *testing.T, root string) []detEntity {
	t.Helper()
	h := dochandler.New(dochandler.WithBodyStore(newDetMemStore(), "objectstore"))
	cfg := detSourceConfig{typ: "docs", path: root}
	states, err := h.IngestEntityStates(ctx, cfg, repoDetOrg)
	if err != nil {
		t.Fatalf("doc IngestEntityStates: %v", err)
	}
	out := make([]detEntity, 0, len(states))
	for _, s := range states {
		out = append(out, fromHandlerState(s))
	}
	return out
}

// TestDeterminism_RepoCorpus is the repo-corpus tier of the CI determinism
// gate (#127): archive this repository's own HEAD once, then run
// AST-plus-hierarchy, cfgfile, and doc ingestion twice each over the identical
// corpus, asserting (via assertDeterministic, determinism_support_test.go)
// that every pass produces the same entity/triple set. Runs unconditionally
// in `go test ./...` — see the file header for the measured runtime that
// justifies no build tag.
func TestDeterminism_RepoCorpus(t *testing.T) {
	corpus := buildRepoArchiveCorpus(t)
	ctx := context.Background()

	watchPaths := []WatchPathConfig{{
		Path:      corpus,
		Org:       repoDetOrg,
		Project:   repoDetProject,
		Languages: []string{"go", "typescript", "javascript", "svelte"},
	}}
	astPass1 := runASTPass(ctx, t, watchPaths)
	astPass2 := runASTPass(ctx, t, watchPaths)
	if len(astPass1) == 0 {
		t.Fatal("repo AST corpus produced zero entities — corpus or routing is broken, not just non-deterministic")
	}
	assertDeterministic(t, "repo corpus AST (go/typescript/javascript/svelte) + hierarchy", astPass1, astPass2)

	cfgPass1 := runCfgfilePass(ctx, t, corpus)
	cfgPass2 := runCfgfilePass(ctx, t, corpus)
	if len(cfgPass1) == 0 {
		t.Fatal("repo cfgfile corpus produced zero entities")
	}
	assertDeterministic(t, "repo corpus cfgfile (go.mod/package.json/Dockerfile)", cfgPass1, cfgPass2)

	docPass1 := runDocPass(ctx, t, corpus)
	docPass2 := runDocPass(ctx, t, corpus)
	if len(docPass1) == 0 {
		t.Fatal("repo doc corpus produced zero entities")
	}
	assertDeterministic(t, "repo corpus doc (markdown)", docPass1, docPass2)
}
