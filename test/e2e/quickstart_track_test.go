//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These tests execute docs/QUICKSTART.md's marked command blocks verbatim
// (see quickstart_runner.go) — the document is the only copy of the
// commands; this file holds only the per-step assertions (design D2).
//
// Both tracks run one engine at a time: the documented configs carry no
// metrics section, so each engine binds the default 9091 metrics port.
// Go runs tests in a package sequentially, which is exactly what we need.

// quickstartStatus is the enriched /source-manifest/status payload the
// quickstart documents: aggregate + per-source phases, index/embedding
// readiness objects, and per-path submodule states.
type quickstartStatus struct {
	Namespace     string `json:"namespace"`
	Phase         string `json:"phase"`
	TotalEntities int64  `json:"total_entities"`
	Sources       []struct {
		InstanceName string `json:"instance_name"`
		SourceType   string `json:"source_type"`
		Phase        string `json:"phase"`
		EntityCount  int64  `json:"entity_count"`
		ErrorCount   int64  `json:"error_count"`
		Submodules   []struct {
			Path  string `json:"path"`
			SHA   string `json:"sha,omitempty"`
			State string `json:"state"`
		} `json:"submodules,omitempty"`
	} `json:"sources"`
	Index struct {
		Available bool `json:"available"`
		Ready     bool `json:"ready"`
	} `json:"index"`
	Embedding struct {
		Available bool `json:"available"`
		Ready     bool `json:"ready"`
	} `json:"embedding"`
}

// fetchQuickstartStatus GETs and decodes the documented status endpoint.
func fetchQuickstartStatus(t *testing.T, httpPort int) (quickstartStatus, error) {
	t.Helper()
	var s quickstartStatus
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/source-manifest/status", httpPort))
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, fmt.Errorf("status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return s, err
	}
	return s, nil
}

// waitForQuickstartReady polls until the aggregate phase is ready AND the
// structural index has caught up — the gate the document tells users
// structural queries depend on. The timeout is the test's own generous
// constant; the document's wait guidance stays qualitative.
func waitForQuickstartReady(t *testing.T, httpPort int, timeout time.Duration) quickstartStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last quickstartStatus
	var lastErr error
	for time.Now().Before(deadline) {
		s, err := fetchQuickstartStatus(t, httpPort)
		if err == nil {
			last = s
			if s.Phase == "ready" && s.Index.Ready {
				return s
			}
			if s.Phase == "degraded" {
				t.Fatalf("status went degraded before ready: %+v", s)
			}
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("never reached phase=ready with index.ready within %s; last=%+v lastErr=%v",
		timeout, last, lastErr)
	return last
}

// entityIDPattern extracts entity identifiers from a fusion response body —
// fusion nodes carry them as opaque "handle" fields.
var entityIDPattern = regexp.MustCompile(`"handle"\s*:\s*"([^"]+)"`)

// idsContaining returns the distinct entity IDs in a raw response whose
// lowercase form contains the needle.
func idsContaining(raw, needle string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range entityIDPattern.FindAllStringSubmatch(raw, -1) {
		id := m[1]
		if strings.Contains(strings.ToLower(id), needle) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// TestE2E_QuickstartSingleTrack drives the single-repository track of
// docs/QUICKSTART.md verbatim against a local clone of the public
// semdev-test fixture: init --quick, run, documented readiness wait, and the
// documented first queries returning Classify content. The scratch env
// contains nothing but the documented prerequisites — a needed undocumented
// step fails here as a document defect.
func TestE2E_QuickstartSingleTrack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network e2e in short mode")
	}
	natsURL, cleanup := startNATS(t)
	defer cleanup()

	steps := []quickstartStep{
		{
			name: "clone the repository",
			assert: func(t *testing.T, env *quickstartEnv) {
				if _, err := os.Stat(filepath.Join(env.cwd(), "semdev-test", "health.go")); err != nil {
					t.Errorf("documented clone did not produce semdev-test/health.go: %v", err)
				}
			},
		},
		{
			name: "init --quick",
			assert: func(t *testing.T, env *quickstartEnv) {
				if !strings.Contains(env.lastOutput(), "Config written") {
					t.Errorf("init --quick output missing success message:\n%s", env.lastOutput())
				}
				data, err := os.ReadFile(filepath.Join(env.cwd(), "semsource.json"))
				if err != nil {
					t.Fatalf("documented init produced no semsource.json: %v", err)
				}
				var cfg struct {
					Namespace string `json:"namespace"`
					Sources   []struct {
						Type string `json:"type"`
					} `json:"sources"`
				}
				if err := json.Unmarshal(data, &cfg); err != nil {
					t.Fatalf("generated config does not parse: %v", err)
				}
				// The doc states the namespace derives from the git
				// remote's owner (C360Studio → c360studio).
				if cfg.Namespace != "c360studio" {
					t.Errorf("namespace = %q, want c360studio (as documented)", cfg.Namespace)
				}
				types := map[string]bool{}
				for _, s := range cfg.Sources {
					types[s.Type] = true
				}
				for _, want := range []string{"ast", "docs", "config", "git"} {
					if !types[want] {
						t.Errorf("generated config missing documented source type %q", want)
					}
				}
			},
		},
		{
			name:       "start the engine",
			background: true,
		},
		{
			name:     "watch readiness",
			retryFor: 4 * time.Minute,
			retryUntil: func(out string) bool {
				return strings.Contains(out, `"phase":"ready"`)
			},
			assert: func(t *testing.T, env *quickstartEnv) {
				s := waitForQuickstartReady(t, env.httpPort, 3*time.Minute)
				if s.Namespace != "c360studio" {
					t.Errorf("status namespace = %q, want c360studio", s.Namespace)
				}
				if s.TotalEntities == 0 {
					t.Errorf("ready with zero entities: %+v", s)
				}
				for _, src := range s.Sources {
					if src.ErrorCount > 0 {
						t.Errorf("source %s reports %d errors at ready", src.InstanceName, src.ErrorCount)
					}
				}
			},
		},
		{
			name:     "first query: code-context/context Classify",
			retryFor: 2 * time.Minute,
			retryUntil: func(out string) bool {
				return strings.Contains(out, `"name":"Classify"`)
			},
			assert: func(t *testing.T, env *quickstartEnv) {
				out := env.lastOutput()
				if !strings.Contains(out, "func Classify") {
					t.Errorf("documented context query returned no verbatim Classify body:\n%.2000s", out)
				}
			},
		},
		{
			name:     "search by meaning",
			retryFor: 2 * time.Minute,
			retryUntil: func(out string) bool {
				return strings.Contains(out, "Classify")
			},
			assert: nil,
		},
	}

	runQuickstartTrack(t, "single", natsURL, steps)
}

// TestE2E_QuickstartMultiTrack drives the multi-repository track verbatim:
// remote semdev-test (which dual-pins semdev-test-sub) + a local
// semdev-test-sub clone registered with explicit project/version at the same
// pin. Asserts per-source readiness, submodule loudness, a query answering
// from each source's scope, and the dedup contract: one merged Farewell
// entity, not per-source forks (ADR-0012).
func TestE2E_QuickstartMultiTrack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network e2e in short mode")
	}
	natsURL, cleanup := startNATS(t)
	defer cleanup()

	steps := []quickstartStep{
		{
			name: "workspace and pinned local clone",
			assert: func(t *testing.T, env *quickstartEnv) {
				head, err := os.ReadFile(filepath.Join(env.cwd(), "semdev-test-sub", ".git", "HEAD"))
				if err != nil {
					t.Fatalf("documented clone missing: %v", err)
				}
				if got := strings.TrimSpace(string(head)); !strings.HasPrefix(got, fixtureShaV2) {
					t.Errorf("local clone HEAD = %q, want the documented pin %s", got, fixtureShaV2)
				}
			},
		},
		{
			name: "write the config",
			assert: func(t *testing.T, env *quickstartEnv) {
				data, err := os.ReadFile(filepath.Join(env.cwd(), "semsource.json"))
				if err != nil {
					t.Fatalf("documented heredoc produced no semsource.json: %v", err)
				}
				var cfg struct {
					Sources []json.RawMessage `json:"sources"`
				}
				if err := json.Unmarshal(data, &cfg); err != nil {
					t.Fatalf("documented config does not parse: %v", err)
				}
				if len(cfg.Sources) != 2 {
					t.Errorf("documented config has %d sources, want 2", len(cfg.Sources))
				}
			},
		},
		{
			name: "validate",
			// Exit 0 is the contract; validate's wording is its own.
		},
		{
			name:       "start the engine",
			background: true,
		},
		{
			name:     "per-source readiness and submodule loudness",
			retryFor: 6 * time.Minute,
			retryUntil: func(out string) bool {
				// The documented signals: aggregate ready and both pins
				// materialized (expansion lands after the clone tick).
				return strings.Contains(out, `"phase":"ready"`) &&
					strings.Contains(out, fixtureShaV1) &&
					strings.Contains(out, fixtureShaV2)
			},
			assert: func(t *testing.T, env *quickstartEnv) {
				s := waitForQuickstartReady(t, env.httpPort, 3*time.Minute)
				if s.Namespace != "quickstart" {
					t.Errorf("status namespace = %q, want quickstart (from the documented config)", s.Namespace)
				}
				// The repo source expands to git+ast+docs+config instances,
				// plus the standalone local ast source.
				if len(s.Sources) < 5 {
					t.Errorf("status sources = %d, want >= 5 (repo expansion + local source): %+v",
						len(s.Sources), s.Sources)
				}
				states := map[string]string{}
				shas := map[string]string{}
				for _, src := range s.Sources {
					for _, sm := range src.Submodules {
						states[sm.Path] = sm.State
						shas[sm.Path] = sm.SHA
					}
				}
				for path, wantSHA := range map[string]string{
					"semdev-test-sub":        fixtureShaV2,
					"nested/semdev-test-sub": fixtureShaV1,
				} {
					if states[path] != "materialized" {
						t.Errorf("submodule %s state = %q, want materialized (all: %v)", path, states[path], states)
					}
					if shas[path] != wantSHA {
						t.Errorf("submodule %s sha = %q, want %s", path, shas[path], wantSHA)
					}
				}
			},
		},
		{
			name:     "query the parent scope: Classify",
			retryFor: 3 * time.Minute,
			retryUntil: func(out string) bool {
				return strings.Contains(out, `"name":"Classify"`)
			},
			assert: func(t *testing.T, env *quickstartEnv) {
				out := env.lastOutput()
				if !strings.Contains(out, "func Classify") {
					t.Errorf("parent-scope query returned no verbatim body:\n%.2000s", out)
				}
				// Attribution: Classify belongs to the parent repository's
				// own identity scope, not the submodule's.
				ids := idsContaining(out, "classify")
				if len(ids) == 0 {
					t.Errorf("no Classify entity handle in response:\n%.2000s", out)
				}
				for _, id := range ids {
					lower := strings.ToLower(id)
					if !strings.Contains(lower, "github-com-c360studio-semdev-test") ||
						strings.Contains(lower, subProjectSlug) {
						t.Errorf("Classify handle %q is not scoped to the parent repository", id)
					}
				}
			},
		},
		{
			name:     "query the submodule scope: Farewell (dedup)",
			retryFor: 4 * time.Minute,
			retryUntil: func(out string) bool {
				return strings.Contains(out, `"name":"Farewell"`)
			},
			assert: func(t *testing.T, env *quickstartEnv) {
				out := env.lastOutput()
				ids := idsContaining(out, "farewell")
				if len(ids) == 0 {
					t.Fatalf("no Farewell entity id in the documented query response:\n%.2000s", out)
				}
				// One merged entity, not per-source forks: every Farewell id
				// carries the submodule's canonical project scope and the
				// shared pin, and exactly one distinct id exists.
				for _, id := range ids {
					lower := strings.ToLower(id)
					if !strings.Contains(lower, subProjectSlug) {
						t.Errorf("Farewell id %q lacks the canonical submodule project scope %s — a per-source fork", id, subProjectSlug)
					}
					if !strings.Contains(lower, fixtureShaV2) {
						t.Errorf("Farewell id %q lacks the shared pin %s", id, fixtureShaV2)
					}
				}
				if len(ids) > 1 {
					t.Errorf("dedup: %d distinct Farewell entities, want 1 merged: %v", len(ids), ids)
				}
			},
		},
	}

	runQuickstartTrack(t, "multi", natsURL, steps)
}
