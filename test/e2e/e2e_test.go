//go:build e2e

// Package e2e runs black-box tests against the compiled semsource binary.
// The binary is built from source, pointed at its own repository, and run as
// a subprocess. Assertions are made against the JSON log output on stdout.
package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// logEntry is the subset of slog JSON fields we inspect.
type logEntry struct {
	Level      string `json:"level"`
	Msg        string `json:"msg"`
	Type       string `json:"type"`
	Namespace  string `json:"namespace"`
	SourceID   string `json:"source_id"`
	EntityID   string `json:"entity_id"`
	EntityCount int   `json:"entity_count"`
}

// repoRoot walks up from the current directory to find go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// buildBinary compiles the semsource binary into a temp directory and returns
// the path to the executable.
func buildBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "semsource")

	root := repoRoot(t)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/semsource")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binPath
}

// writeConfig writes a semsource.json targeting the repo itself.
func writeConfig(t *testing.T, dir string) string {
	t.Helper()
	root := repoRoot(t)
	cfg := map[string]any{
		"namespace": "e2etest",
		"flow": map[string]any{
			"outputs":       []map[string]any{{
				"name": "log", "type": "log", "subject": "test",
			}},
			"delivery_mode": "at-least-once",
			"ack_timeout":   "5s",
		},
		"sources": []map[string]any{
			{"type": "ast", "path": root, "language": "go"},
			{"type": "docs", "paths": []string{root}},
			{"type": "config", "paths": []string{root}},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	configPath := filepath.Join(dir, "semsource.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func TestE2E_SelfIngest_ProducesLogEvents(t *testing.T) {
	binPath := buildBinary(t)
	workDir := t.TempDir()
	configPath := writeConfig(t, workDir)

	// Run the binary with a timeout. It will ingest sources and emit log lines
	// until we kill it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "run",
		"--config", configPath,
		"--log-level", "info",
	)
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start binary: %v", err)
	}

	// Collect log lines until we see the "starting semsource" + SEED events,
	// or we time out.
	var logs []logEntry
	scanner := bufio.NewScanner(stdout)

	// Read lines in a goroutine with a deadline.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			var entry logEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue // skip non-JSON lines
			}
			logs = append(logs, entry)

			// Once we've seen enough SEED events, stop reading.
			seedCount := 0
			for _, l := range logs {
				if l.Msg == "graph event emitted" && l.Type == "SEED" {
					seedCount++
				}
			}
			if seedCount >= 5 {
				return
			}
		}
	}()

	// Wait for either enough events or timeout.
	select {
	case <-done:
	case <-time.After(20 * time.Second):
	}

	// Kill the process gracefully.
	cmd.Process.Signal(os.Interrupt)
	cmd.Wait()

	if len(logs) == 0 {
		t.Fatal("no log output from binary")
	}

	// Verify "starting semsource" message.
	foundStartup := false
	for _, l := range logs {
		if l.Msg == "starting semsource" {
			foundStartup = true
			break
		}
	}
	if !foundStartup {
		t.Error("did not find 'starting semsource' log entry")
	}

	// Verify "configuration loaded" message.
	foundConfigLoaded := false
	for _, l := range logs {
		if l.Msg == "configuration loaded" && l.Namespace == "e2etest" {
			foundConfigLoaded = true
			break
		}
	}
	if !foundConfigLoaded {
		t.Error("did not find 'configuration loaded' with namespace 'e2etest'")
	}

	// Count SEED events in log output.
	seedEvents := 0
	for _, l := range logs {
		if l.Msg == "graph event emitted" && l.Type == "SEED" {
			seedEvents++
		}
	}
	if seedEvents == 0 {
		t.Error("no SEED events in log output")
	}
	t.Logf("log lines: %d, SEED events: %d", len(logs), seedEvents)

	// Verify all SEED events have correct namespace.
	for _, l := range logs {
		if l.Msg == "graph event emitted" && l.Namespace != "" && l.Namespace != "e2etest" {
			t.Errorf("event namespace = %q, want 'e2etest'", l.Namespace)
		}
	}
}

func TestE2E_Version(t *testing.T) {
	binPath := buildBinary(t)

	out, err := exec.Command(binPath, "version").Output()
	if err != nil {
		t.Fatalf("version command: %v", err)
	}

	output := strings.TrimSpace(string(out))
	if !strings.HasPrefix(output, "semsource ") {
		t.Errorf("version output = %q, want prefix 'semsource '", output)
	}
	t.Logf("version: %s", output)
}

func TestE2E_Validate(t *testing.T) {
	binPath := buildBinary(t)
	workDir := t.TempDir()
	configPath := writeConfig(t, workDir)

	cmd := exec.Command(binPath, "validate", "--config", configPath)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out)
	}
	t.Logf("validate output: %s", strings.TrimSpace(string(out)))
}
