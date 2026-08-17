//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The public fixture: C360Studio/semdev-test dual-pins semdev-test-sub —
// root path @ v2 (has greeter.Farewell), nested/semdev-test-sub @ v1 (no
// Farewell). Pins are append-only by fixture contract (see its README).
const (
	fixtureRepoURL = "https://github.com/C360Studio/semdev-test"
	subProjectSlug = "github-com-c360studio-semdev-test-sub"
	fixtureShaV1   = "b191a7bf4013" // nested pin, no Farewell
	fixtureShaV2   = "b1256521ee39" // root pin, has Farewell
)

// submoduleSourceStatus extends the harness sourceStatus with the submodule
// loudness field.
type submoduleSourceStatus struct {
	InstanceName string `json:"instance_name"`
	SourceType   string `json:"source_type"`
	Submodules   []struct {
		Path  string `json:"path"`
		SHA   string `json:"sha,omitempty"`
		State string `json:"state"`
	} `json:"submodules,omitempty"`
}

// TestE2E_SubmoduleIngest clones the real fixture from GitHub and asserts the
// three contract pillars end-to-end: completeness (both pins' trees ingest),
// identity (per-pin version scopes; the Farewell marker separates them; no
// submodule code under the parent scope), and loudness (per-path submodule
// states on the status surface).
func TestE2E_SubmoduleIngest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network e2e in short mode")
	}

	natsURL, cleanup := startNATS(t)
	defer cleanup()

	binPath := buildBinary(t)
	workDir := t.TempDir()
	workspaceDir := t.TempDir()
	httpPort := freePort(t)

	cfg := map[string]any{
		"namespace":     "e2eacme",
		"workspace_dir": workspaceDir,
		"http_port":     httpPort,
		"sources": []map[string]any{{
			"type":      "repo",
			"url":       fixtureRepoURL,
			"languages": []string{"go"},
			"watch":     false,
			// Drives the subwatch tick cadence: the first tick precedes the
			// clone, so expansion lands on a later tick — keep them frequent.
			"poll_interval": "5s",
		}},
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workDir, "semsource.json")
	if err := os.WriteFile(configPath, cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "GRAPH",
		Subjects: []string{"graph.ingest.>"},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	cons, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	runCtx, runCancel := context.WithTimeout(ctx, 300*time.Second)
	defer runCancel()
	cmd := exec.CommandContext(runCtx, binPath, "run",
		"--config", configPath,
		"--log-level", "debug",
		"--nats-url", natsURL,
	)
	cmd.Dir = workDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start semsource: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	go logPipe(t, "stderr", stderr)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.Logf("[stdout] %s", scanner.Text())
		}
	}()

	// Collect entity IDs until both version scopes are represented and the
	// stream goes quiet, or timeout.
	var mu sync.Mutex
	var ids []string
	collectCtx, collectCancel := context.WithTimeout(ctx, 240*time.Second)
	defer collectCancel()
	const stability = 8 * time.Second
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		lastActivity := time.Now()
		for collectCtx.Err() == nil {
			mu.Lock()
			joined := strings.Join(ids, " ")
			bothScopes := strings.Contains(joined, fixtureShaV1) && strings.Contains(joined, fixtureShaV2)
			mu.Unlock()
			if bothScopes && time.Since(lastActivity) >= stability {
				return
			}
			msgs, err := cons.Fetch(100, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				continue
			}
			for msg := range msgs.Messages() {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(msg.Data(), &raw); err == nil {
					payloadJSON, ok := raw["payload"]
					if !ok {
						payloadJSON = raw["data"]
					}
					var p entityPayload
					if json.Unmarshal(payloadJSON, &p) == nil && p.ID != "" {
						mu.Lock()
						ids = append(ids, p.ID)
						lastActivity = time.Now()
						mu.Unlock()
					}
				}
				msg.Ack()
			}
		}
	}()
	select {
	case <-fetchDone:
	case <-collectCtx.Done():
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ids) == 0 {
		t.Fatal("no entities collected")
	}

	// Identity: both pins present, in distinct version scopes.
	var v1Farewell, v2Farewell, v1Greet, v2Greet bool
	var parentScopedSubCode []string
	for _, id := range ids {
		lower := strings.ToLower(id)
		inSub := strings.Contains(lower, subProjectSlug)
		switch {
		case inSub && strings.Contains(lower, fixtureShaV1):
			if strings.Contains(lower, "farewell") {
				v1Farewell = true
			}
			if strings.Contains(lower, "greet") {
				v1Greet = true
			}
		case inSub && strings.Contains(lower, fixtureShaV2):
			if strings.Contains(lower, "farewell") {
				v2Farewell = true
			}
			if strings.Contains(lower, "greet") {
				v2Greet = true
			}
		case !inSub && (strings.Contains(lower, "farewell") || strings.Contains(lower, "greeter")):
			// Submodule code attributed outside the submodule scope.
			parentScopedSubCode = append(parentScopedSubCode, id)
		}
	}
	if !v1Greet || !v2Greet {
		t.Errorf("completeness: Greet missing from a pin (v1=%v v2=%v)", v1Greet, v2Greet)
	}
	if !v2Farewell {
		t.Error("identity: Farewell missing from the v2 scope")
	}
	if v1Farewell {
		t.Error("identity: Farewell present in the v1 scope — version scoping is broken")
	}
	if len(parentScopedSubCode) > 0 {
		t.Errorf("exclusion: submodule code under a non-submodule scope: %v", parentScopedSubCode)
	}

	// Loudness: the git source's status lists both submodule paths as
	// materialized with their pins.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/source-manifest/status", httpPort))
	if err != nil {
		t.Fatalf("status fetch: %v", err)
	}
	defer resp.Body.Close()
	var status struct {
		Sources []submoduleSourceStatus `json:"sources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	states := map[string]string{}
	shas := map[string]string{}
	for _, s := range status.Sources {
		for _, sm := range s.Submodules {
			states[sm.Path] = sm.State
			shas[sm.Path] = sm.SHA
		}
	}
	for path, wantSHA := range map[string]string{
		"semdev-test-sub":        fixtureShaV2,
		"nested/semdev-test-sub": fixtureShaV1,
	} {
		if states[path] != "materialized" {
			t.Errorf("loudness: %s state = %q, want materialized (all: %v)", path, states[path], states)
		}
		if shas[path] != wantSHA {
			t.Errorf("loudness: %s sha = %q, want %s", path, shas[path], wantSHA)
		}
	}
}
