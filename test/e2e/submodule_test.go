//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
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

// TestE2E_SubmoduleIngest clones the real fixture from GitHub and asserts the
// submodule contract this migration pin supports (SETUP 03A,
// docs/testing/setup-03a/compatibility.md "Remote submodule discovery
// blocked"): component admission is boot-only, so remote submodule discovery
// is blocked loudly — a capability=blocked warning names the source — while
// the parent repository still ingests, no entity lands in a submodule version
// scope, and no submodule code is attributed to the parent scope.
//
// shortcut: asserts the blocked contract (#248); restore the per-pin
// completeness/identity/loudness assertions (git history before #249) once the
// SemEngine port restores runtime component admission.
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
	blockedWarned := make(chan struct{})
	go func() {
		var once sync.Once
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("[stdout] %s", line)
			if strings.Contains(line, `"level":"WARN"`) &&
				strings.Contains(line, "dynamic submodule discovery unavailable") &&
				strings.Contains(line, `"capability":"blocked"`) &&
				strings.Contains(line, fixtureRepoURL) {
				once.Do(func() { close(blockedWarned) })
			}
		}
	}()

	// Collect entity IDs until the stream goes quiet after the first entity,
	// or timeout. Nothing waits on submodule scopes: on this pin none appear.
	var mu sync.Mutex
	var ids []string
	collectCtx, collectCancel := context.WithTimeout(ctx, 240*time.Second)
	defer collectCancel()
	const stability = 15 * time.Second
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		lastActivity := time.Now()
		for collectCtx.Err() == nil {
			mu.Lock()
			seen := len(ids)
			mu.Unlock()
			if seen > 0 && time.Since(lastActivity) >= stability {
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

	select {
	case <-blockedWarned:
	case <-time.After(10 * time.Second):
		t.Error("blocked: no WARN capability=blocked submodule-discovery line for the fixture source")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ids) == 0 {
		t.Fatal("parent: no entities collected — the parent repository did not ingest")
	}
	var subScoped, parentScopedSubCode []string
	for _, id := range ids {
		lower := strings.ToLower(id)
		switch {
		case strings.Contains(lower, subProjectSlug):
			subScoped = append(subScoped, id)
		case strings.Contains(lower, "farewell") || strings.Contains(lower, "greeter"):
			parentScopedSubCode = append(parentScopedSubCode, id)
		}
	}
	if len(subScoped) > 0 {
		t.Errorf("blocked: %d entities in a submodule scope, want none on this pin (first: %s)", len(subScoped), subScoped[0])
	}
	if len(parentScopedSubCode) > 0 {
		t.Errorf("exclusion: submodule code under a non-submodule scope: %v", parentScopedSubCode)
	}
	t.Logf("parent ingested %d entities; submodule scopes empty as expected", len(ids))
}
