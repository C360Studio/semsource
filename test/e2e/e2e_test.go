//go:build e2e

// Package e2e runs black-box tests against the compiled semsource binary.
// The binary is built from source, pointed at its own repository, and run as
// a subprocess. NATS is started via Docker for the v2 runner. Assertions are
// made against JSON log output (stdout) and NATS messages — no internal
// imports from semsource are used.
package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
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

// logEntry is the subset of slog JSON fields we inspect.
type logEntry struct {
	Level   string `json:"level"`
	Msg     string `json:"msg"`
	Version string `json:"version"`
	Sources int    `json:"sources"`
}

// manifestPayload mirrors the source-manifest response structure.
type manifestPayload struct {
	Namespace string           `json:"namespace"`
	Sources   []manifestSource `json:"sources"`
	Timestamp string           `json:"timestamp"`
}

type manifestSource struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	URL          string   `json:"url,omitempty"`
	URLs         []string `json:"urls,omitempty"`
	Language     string   `json:"language,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Watch        bool     `json:"watch"`
	PollInterval string   `json:"poll_interval,omitempty"`
}

// statusPayload mirrors the source-manifest status response structure.
type statusPayload struct {
	Namespace     string         `json:"namespace"`
	Phase         string         `json:"phase"`
	Sources       []sourceStatus `json:"sources"`
	TotalEntities int64          `json:"total_entities"`
	Timestamp     string         `json:"timestamp"`
}

type sourceStatus struct {
	InstanceName string           `json:"instance_name"`
	SourceType   string           `json:"source_type"`
	Phase        string           `json:"phase"`
	EntityCount  int64            `json:"entity_count"`
	ErrorCount   int64            `json:"error_count"`
	TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
}

// summaryPayload mirrors the source-manifest summary response structure.
type summaryPayload struct {
	Namespace      string          `json:"namespace"`
	Phase          string          `json:"phase"`
	EntityIDFormat string          `json:"entity_id_format"`
	TotalEntities  int64           `json:"total_entities"`
	Domains        []domainSummary `json:"domains"`
	Timestamp      string          `json:"timestamp"`
}

type domainSummary struct {
	Domain      string      `json:"domain"`
	EntityCount int64       `json:"entity_count"`
	Types       []typeCount `json:"types"`
	Sources     []string    `json:"sources"`
}

type typeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

// queryManifestHTTP GETs the source manifest from the ServiceManager HTTP API.
// Retries for up to 15 seconds to allow the HTTP server and component to start.
func queryManifestHTTP(t *testing.T, httpPort int) manifestPayload {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/source-manifest/sources", httpPort)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var m manifestPayload
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("decode manifest response: %v", err)
		}
		return m
	}
	t.Fatalf("GET %s did not return 200 within 15s", url)
	return manifestPayload{}
}

// queryStatusHTTP GETs the ingestion status from the ServiceManager HTTP API.
// Retries for up to 15 seconds to allow startup.
func queryStatusHTTP(t *testing.T, httpPort int) statusPayload {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/source-manifest/status", httpPort)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var s statusPayload
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			t.Fatalf("decode status response: %v", err)
		}
		return s
	}
	t.Fatalf("GET %s did not return 200 within 15s", url)
	return statusPayload{}
}

// waitForReady polls the status endpoint until Phase is "ready" or "degraded",
// or the deadline expires. Returns the final status.
func waitForReady(t *testing.T, httpPort int, timeout time.Duration) statusPayload {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/source-manifest/status", httpPort)
	deadline := time.Now().Add(timeout)
	var last statusPayload
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		json.NewDecoder(resp.Body).Decode(&last)
		resp.Body.Close()
		if last.Phase == "ready" || last.Phase == "degraded" {
			return last
		}
		time.Sleep(2 * time.Second)
	}
	return last
}

// querySummaryHTTP GETs the graph summary from the ServiceManager HTTP API.
func querySummaryHTTP(t *testing.T, httpPort int) summaryPayload {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/source-manifest/summary", httpPort)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var s summaryPayload
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			t.Fatalf("decode summary response: %v", err)
		}
		return s
	}
	t.Fatalf("GET %s did not return 200 within 15s", url)
	return summaryPayload{}
}

// entityMessage is the envelope published to graph.ingest.entity.
type entityMessage struct {
	Type    string          `json:"type"`
	Source  string          `json:"source"`
	Payload json.RawMessage `json:"payload"`
}

// entityPayload is the payload inside the entity message.
type entityPayload struct {
	ID      string `json:"id"`
	Triples []struct {
		Predicate string `json:"predicate"`
		Object    any    `json:"object"`
	} `json:"triples"`
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

// freePort returns an available TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// startNATS starts a NATS container on a random port and returns the URL and
// a cleanup function. Requires Docker.
func startNATS(t *testing.T) (natsURL string, cleanup func()) {
	t.Helper()
	port := freePort(t)
	containerName := fmt.Sprintf("semsource-e2e-nats-%d", port)

	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:4222", port),
		"nats:2-alpine", "-js",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("start NATS container: %v\n%s", err, out)
	}
	containerID := strings.TrimSpace(string(out))

	natsURL = fmt.Sprintf("nats://127.0.0.1:%d", port)

	cleanup = func() {
		exec.Command("docker", "rm", "-f", containerID).Run()
	}

	// Wait for NATS to accept connections.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		nc, err := nats.Connect(natsURL, nats.Timeout(500*time.Millisecond))
		if err == nil {
			nc.Close()
			return natsURL, cleanup
		}
		time.Sleep(200 * time.Millisecond)
	}
	cleanup()
	t.Fatalf("NATS did not become ready at %s within 15s", natsURL)
	return "", nil
}

// writeConfig writes a semsource.json targeting the repo itself with the given
// NATS URL for entity store connectivity.
func writeConfig(t *testing.T, dir string, httpPort int) string {
	t.Helper()
	root := repoRoot(t)
	cfg := map[string]any{
		"namespace": "e2etest",
		"http_port": httpPort,
		"sources": []map[string]any{
			{"type": "ast", "path": root, "language": "go", "watch": false},
			{"type": "docs", "paths": []string{filepath.Join(root, "docs")}, "watch": false},
			{"type": "config", "paths": []string{root}, "watch": false},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	configPath := filepath.Join(dir, "semsource.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

// --- Tests ---

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
	configPath := writeConfig(t, workDir, 0)

	cmd := exec.Command(binPath, "validate", "--config", configPath)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out)
	}
	t.Logf("validate output: %s", strings.TrimSpace(string(out)))
}

func TestE2E_RunStartsAndPublishesEntities(t *testing.T) {
	// Start NATS via Docker.
	natsURL, cleanup := startNATS(t)
	defer cleanup()

	binPath := buildBinary(t)
	workDir := t.TempDir()
	httpPort := freePort(t)
	configPath := writeConfig(t, workDir, httpPort)

	// Subscribe to the graph ingest subject BEFORE starting semsource so we
	// don't miss any messages. Use JetStream consumer for reliable delivery.
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}

	// Create the GRAPH stream before semsource starts — semsource also does
	// EnsureStreams but the subscriber needs it first.
	ctx := context.Background()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "GRAPH",
		Subjects: []string{"graph.ingest.>"},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create GRAPH stream: %v", err)
	}

	// Create ephemeral consumer.
	cons, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	// Start semsource as a subprocess.
	runCtx, runCancel := context.WithTimeout(ctx, 60*time.Second)
	defer runCancel()

	cmd := exec.CommandContext(runCtx, binPath, "run",
		"--config", configPath,
		"--log-level", "debug",
		"--nats-url", natsURL,
	)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start semsource: %v", err)
	}

	// Capture stderr in background.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.Logf("[stderr] %s", scanner.Text())
		}
	}()

	// Collect log lines in a goroutine.
	var logs []logEntry
	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("[stdout] %s", line)
			var entry logEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			logs = append(logs, entry)
		}
	}()

	// Collect entity messages from NATS.
	var entities []entityPayload
	collectCtx, collectCancel := context.WithTimeout(ctx, 45*time.Second)
	defer collectCancel()

	// Fetch messages in a loop until we have enough or timeout.
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		for {
			if collectCtx.Err() != nil {
				return
			}
			msgs, err := cons.Fetch(10, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				continue
			}
			for msg := range msgs.Messages() {
				// Log first raw message for debugging envelope shape.
				if len(entities) == 0 {
					t.Logf("raw message sample: %s", string(msg.Data()[:min(len(msg.Data()), 500)]))
				}

				// Parse as generic JSON to find the payload.
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(msg.Data(), &raw); err != nil {
					t.Logf("unmarshal raw: %v", err)
					msg.Ack()
					continue
				}

				// The payload may be under "payload" or "data".
				payloadJSON, ok := raw["payload"]
				if !ok {
					payloadJSON, ok = raw["data"]
				}
				if !ok {
					msg.Ack()
					continue
				}

				var payload entityPayload
				if err := json.Unmarshal(payloadJSON, &payload); err != nil {
					t.Logf("unmarshal payload: %v", err)
					msg.Ack()
					continue
				}
				if payload.ID == "" {
					msg.Ack()
					continue
				}
				entities = append(entities, payload)
				msg.Ack()

				// Stop after collecting enough entities to validate.
				if len(entities) >= 20 {
					return
				}
			}
		}
	}()

	// Wait for entity collection or timeout.
	select {
	case <-fetchDone:
		t.Logf("collected %d entities from NATS", len(entities))
	case <-collectCtx.Done():
		t.Logf("collection timed out with %d entities", len(entities))
	}

	// Query the source manifest via HTTP while semsource is still running.
	manifest := queryManifestHTTP(t, httpPort)
	t.Logf("manifest: namespace=%s, sources=%d", manifest.Namespace, len(manifest.Sources))

	// Query ingestion status — should reach "ready" since all sources are no-watch.
	status := waitForReady(t, httpPort, 30*time.Second)
	t.Logf("status: phase=%s, total_entities=%d, sources=%d", status.Phase, status.TotalEntities, len(status.Sources))

	// Query graph summary — agent bootstrap endpoint.
	summary := querySummaryHTTP(t, httpPort)
	t.Logf("summary: domains=%d, total_entities=%d", len(summary.Domains), summary.TotalEntities)
	for _, d := range summary.Domains {
		t.Logf("  domain %q: %d entities, %d types", d.Domain, d.EntityCount, len(d.Types))
		for _, tc := range d.Types {
			t.Logf("    %s: %d", tc.Type, tc.Count)
		}
	}

	// Stop semsource gracefully.
	cmd.Process.Signal(os.Interrupt)
	cmd.Wait()
	<-logsDone

	// --- Assertions ---

	// 0. Source manifest is correct.
	if manifest.Namespace != "e2etest" {
		t.Errorf("manifest namespace = %q, want %q", manifest.Namespace, "e2etest")
	}
	if len(manifest.Sources) != 3 {
		t.Errorf("manifest sources count = %d, want 3 (ast, docs, config)", len(manifest.Sources))
	}
	manifestTypes := map[string]bool{}
	for _, s := range manifest.Sources {
		manifestTypes[s.Type] = true
	}
	for _, want := range []string{"ast", "docs", "config"} {
		if !manifestTypes[want] {
			t.Errorf("manifest missing source type %q", want)
		}
	}

	// --- Status assertions ---
	if status.Phase != "ready" {
		t.Errorf("status phase = %q, want 'ready'", status.Phase)
	}
	if status.Namespace != "e2etest" {
		t.Errorf("status namespace = %q, want %q", status.Namespace, "e2etest")
	}
	// 3 source components: ast, docs, config → 3 instance-level status entries.
	if len(status.Sources) != 3 {
		t.Errorf("status sources count = %d, want 3", len(status.Sources))
	}
	// Every source must have a non-empty instance_name.
	for _, src := range status.Sources {
		if src.InstanceName == "" {
			t.Errorf("status source %q has empty instance_name", src.SourceType)
		}
		if src.Phase != "watching" && src.Phase != "idle" {
			t.Errorf("status source %q: phase = %q, want 'watching' or 'idle'", src.InstanceName, src.Phase)
		}
	}
	// Total entities should match what we received (approximately — status may lag slightly).
	if status.TotalEntities == 0 {
		t.Error("status total_entities = 0, expected entities to be counted")
	}

	// --- Summary assertions ---
	if summary.Namespace != "e2etest" {
		t.Errorf("summary namespace = %q, want %q", summary.Namespace, "e2etest")
	}
	if summary.Phase != "ready" {
		t.Errorf("summary phase = %q, want 'ready'", summary.Phase)
	}
	if summary.EntityIDFormat == "" {
		t.Error("summary entity_id_format is empty")
	}
	if len(summary.Domains) == 0 {
		t.Error("summary has no domains — type counting not working")
	}
	if summary.TotalEntities == 0 {
		t.Error("summary total_entities = 0")
	}
	// Should have at least golang domain (we're indexing Go code).
	foundGolang := false
	for _, d := range summary.Domains {
		if d.Domain == "golang" {
			foundGolang = true
			if len(d.Types) == 0 {
				t.Error("golang domain has no type breakdown")
			}
		}
	}
	if !foundGolang {
		t.Error("summary missing 'golang' domain")
	}

	// 1. Binary started and produced log output.
	if len(logs) == 0 {
		t.Fatal("no log output from semsource")
	}

	// 2. Startup log message present.
	foundStartup := false
	for _, l := range logs {
		if l.Msg == "semsource starting" {
			foundStartup = true
			break
		}
	}
	if !foundStartup {
		t.Error("missing 'semsource starting' log entry")
		for i, l := range logs {
			if i < 10 {
				t.Logf("  log[%d]: %s", i, l.Msg)
			}
		}
	}

	// 3. Component factories registered.
	foundFactories := false
	for _, l := range logs {
		if l.Msg == "component factories registered" {
			foundFactories = true
			break
		}
	}
	if !foundFactories {
		t.Error("missing 'component factories registered' log entry")
	}

	// 4. Entities were published to NATS.
	if len(entities) == 0 {
		t.Fatal("no entities received on graph.ingest.entity")
	}
	t.Logf("received %d entities on graph.ingest.entity", len(entities))

	// 5. All entity IDs follow the 6-part format: {org}.{platform}.{domain}.{system}.{type}.{instance}
	for _, e := range entities {
		parts := strings.Split(e.ID, ".")
		if len(parts) < 6 {
			t.Errorf("entity ID %q has %d parts, want >= 6", e.ID, len(parts))
			continue
		}
		if parts[0] != "e2etest" {
			t.Errorf("entity ID %q: org = %q, want 'e2etest'", e.ID, parts[0])
		}
		if parts[1] != "semsource" {
			t.Errorf("entity ID %q: platform = %q, want 'semsource'", e.ID, parts[1])
		}
	}

	// 6. Entities have triples with vocabulary predicates.
	hasTriples := false
	for _, e := range entities {
		if len(e.Triples) > 0 {
			hasTriples = true
			break
		}
	}
	if !hasTriples {
		t.Error("no entities have triples — vocabulary predicates not being emitted")
	}

	// 7. Log some entity details for visibility.
	seen := map[string]bool{}
	for _, e := range entities {
		parts := strings.Split(e.ID, ".")
		if len(parts) >= 5 {
			key := parts[2] + "." + parts[4] // domain.type
			if !seen[key] {
				seen[key] = true
				t.Logf("  entity type: %s (example: %s, triples: %d)", key, e.ID, len(e.Triples))
			}
		}
	}
}

// writeOSHConfig writes a semsource.json that uses a "repo" meta-source
// pointing at the Open Sensor Hub osh-core repository.
func writeOSHConfig(t *testing.T, dir, workspaceDir string, httpPort int) string {
	t.Helper()
	cfg := map[string]any{
		"namespace":     "oshtest",
		"workspace_dir": workspaceDir,
		"http_port":     httpPort,
		"sources": []map[string]any{
			{
				"type":     "repo",
				"url":      "https://github.com/opensensorhub/osh-core",
				"language": "java",
				"branch":   "master",
				"watch":    false,
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	configPath := filepath.Join(dir, "semsource.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

// parseEntityID splits a 6-part entity ID and returns org, platform, domain, system, entityType, instance.
func parseEntityID(id string) (org, platform, domain, system, entityType, instance string, ok bool) {
	parts := strings.SplitN(id, ".", 6)
	if len(parts) < 6 {
		return "", "", "", "", "", "", false
	}
	return parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], true
}

func TestE2E_OSH_JavaMavenIngest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping OSH integration test in short mode")
	}

	// Start NATS via Docker.
	natsURL, cleanup := startNATS(t)
	defer cleanup()

	binPath := buildBinary(t)
	workDir := t.TempDir()
	workspaceDir := t.TempDir()
	httpPort := freePort(t)
	configPath := writeOSHConfig(t, workDir, workspaceDir, httpPort)

	// Subscribe to graph ingest subject BEFORE starting semsource.
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}

	ctx := context.Background()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "GRAPH",
		Subjects: []string{"graph.ingest.>"},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create GRAPH stream: %v", err)
	}

	cons, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	// Start semsource — the "repo" meta-source expands into git+ast+docs+config.
	// Clones osh-core (shallow), so give it a generous timeout.
	runCtx, runCancel := context.WithTimeout(ctx, 240*time.Second)
	defer runCancel()

	cmd := exec.CommandContext(runCtx, binPath, "run",
		"--config", configPath,
		"--log-level", "debug",
		"--nats-url", natsURL,
	)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start semsource: %v", err)
	}

	// Capture stderr in background.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.Logf("[stderr] %s", scanner.Text())
		}
	}()

	// Collect log lines in a goroutine.
	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.Logf("[stdout] %s", scanner.Text())
		}
	}()

	// Collect entities from NATS, organized by domain.
	// We wait until all 4 expected domains have entities OR we timeout.
	type domainEntities struct {
		entities []entityPayload
	}
	domains := make(map[string]*domainEntities) // git, config, web, java
	var allEntities []entityPayload
	var mu sync.Mutex

	wantDomains := map[string]bool{"git": true, "config": true, "web": true, "java": true}

	collectCtx, collectCancel := context.WithTimeout(ctx, 180*time.Second)
	defer collectCancel()

	// stabilityTimeout: after all domains are present, wait for the stream
	// to go quiet (no new entities for this duration) before stopping.
	const stabilityTimeout = 5 * time.Second

	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		lastActivity := time.Now()
		allDomainsPresent := false

		for {
			if collectCtx.Err() != nil {
				return
			}

			// Once all domains are present, check if stream has stabilised.
			if allDomainsPresent && time.Since(lastActivity) >= stabilityTimeout {
				mu.Lock()
				t.Logf("entity stream stabilised after %d entities (quiet for %s)", len(allEntities), stabilityTimeout)
				mu.Unlock()
				return
			}

			msgs, err := cons.Fetch(50, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				continue
			}
			for msg := range msgs.Messages() {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(msg.Data(), &raw); err != nil {
					msg.Ack()
					continue
				}

				payloadJSON, ok := raw["payload"]
				if !ok {
					payloadJSON, ok = raw["data"]
				}
				if !ok {
					msg.Ack()
					continue
				}

				var payload entityPayload
				if err := json.Unmarshal(payloadJSON, &payload); err != nil {
					msg.Ack()
					continue
				}
				if payload.ID == "" {
					msg.Ack()
					continue
				}
				msg.Ack()

				_, _, domain, _, _, _, idOK := parseEntityID(payload.ID)
				if !idOK {
					continue
				}

				mu.Lock()
				allEntities = append(allEntities, payload)
				lastActivity = time.Now()
				if domains[domain] == nil {
					domains[domain] = &domainEntities{}
					t.Logf("domain %q first entity: %s (total so far: %d)", domain, payload.ID, len(allEntities))
				}
				domains[domain].entities = append(domains[domain].entities, payload)

				if !allDomainsPresent {
					allDomainsPresent = true
					for d := range wantDomains {
						if domains[d] == nil {
							allDomainsPresent = false
							break
						}
					}
				}
				mu.Unlock()
			}
		}
	}()

	// Wait for entity collection or timeout.
	select {
	case <-fetchDone:
		t.Logf("all domains satisfied, collected %d total entities", len(allEntities))
	case <-collectCtx.Done():
		t.Logf("collection timed out with %d entities", len(allEntities))
	}

	// Query the source manifest via HTTP while semsource is still running.
	// The "repo" meta-source expands to git+ast+docs+config.
	manifest := queryManifestHTTP(t, httpPort)
	t.Logf("OSH manifest: namespace=%s, sources=%d", manifest.Namespace, len(manifest.Sources))

	// Query ingestion status — "repo" expands to 4 instances.
	// Use a generous timeout since git clone + Java parsing can be slow.
	oshStatus := waitForReady(t, httpPort, 120*time.Second)
	t.Logf("OSH status: phase=%s, total_entities=%d, sources=%d",
		oshStatus.Phase, oshStatus.TotalEntities, len(oshStatus.Sources))

	// Stop semsource gracefully.
	cmd.Process.Signal(os.Interrupt)
	cmd.Wait()
	<-logsDone

	// =====================================================================
	// Assertions
	// =====================================================================

	// Source manifest reflects expanded repo sources.
	if manifest.Namespace != "oshtest" {
		t.Errorf("manifest namespace = %q, want %q", manifest.Namespace, "oshtest")
	}
	// "repo" expands to 4 sources: git, ast, docs, config.
	if len(manifest.Sources) != 4 {
		t.Errorf("manifest sources count = %d, want 4 (git+ast+docs+config from repo expansion)", len(manifest.Sources))
		for i, s := range manifest.Sources {
			t.Logf("  manifest source[%d]: type=%s", i, s.Type)
		}
	}
	oshManifestTypes := map[string]bool{}
	for _, s := range manifest.Sources {
		oshManifestTypes[s.Type] = true
	}
	for _, want := range []string{"git", "ast", "docs", "config"} {
		if !oshManifestTypes[want] {
			t.Errorf("OSH manifest missing source type %q", want)
		}
	}

	// --- Status assertions ---
	if oshStatus.Phase != "ready" && oshStatus.Phase != "degraded" {
		t.Errorf("OSH status phase = %q, want 'ready' or 'degraded'", oshStatus.Phase)
	}
	if oshStatus.Namespace != "oshtest" {
		t.Errorf("OSH status namespace = %q, want %q", oshStatus.Namespace, "oshtest")
	}
	// 4 component instances from repo expansion: git, ast, docs, config.
	if len(oshStatus.Sources) < 4 {
		t.Errorf("OSH status sources = %d, want >= 4 (git+ast+docs+config instances)", len(oshStatus.Sources))
	}
	// Every status source must have instance_name and source_type.
	oshStatusTypes := map[string]bool{}
	for _, src := range oshStatus.Sources {
		if src.InstanceName == "" {
			t.Errorf("OSH status source has empty instance_name (type=%s)", src.SourceType)
		}
		oshStatusTypes[src.SourceType] = true
		t.Logf("  status source: instance=%s type=%s phase=%s entities=%d errors=%d",
			src.InstanceName, src.SourceType, src.Phase, src.EntityCount, src.ErrorCount)
	}
	// All 4 source types should appear in status.
	for _, want := range []string{"git", "ast", "docs", "config"} {
		if !oshStatusTypes[want] {
			t.Errorf("OSH status missing source type %q", want)
		}
	}
	if oshStatus.TotalEntities == 0 {
		t.Error("OSH status total_entities = 0")
	}

	if len(allEntities) == 0 {
		t.Fatal("no entities received on graph.ingest.entity for OSH repo")
	}

	// --- Structural: every entity has valid 6-part ID with correct org/platform ---
	for _, e := range allEntities {
		org, platform, _, _, _, _, ok := parseEntityID(e.ID)
		if !ok {
			t.Errorf("entity ID %q is not a valid 6-part ID", e.ID)
			continue
		}
		if org != "oshtest" {
			t.Errorf("entity ID %q: org = %q, want 'oshtest'", e.ID, org)
		}
		if platform != "semsource" {
			t.Errorf("entity ID %q: platform = %q, want 'semsource'", e.ID, platform)
		}
	}

	// --- Domain presence: all 4 domains should have produced entities ---
	for d := range wantDomains {
		de := domains[d]
		if de == nil || len(de.entities) == 0 {
			t.Errorf("domain %q produced 0 entities", d)
		} else {
			t.Logf("domain %q: %d entities", d, len(de.entities))
		}
	}

	// =====================================================================
	// Git assertions — we know exactly what to expect
	// =====================================================================
	if gitDomain := domains["git"]; gitDomain != nil {
		gitSystem := "github-com-opensensorhub-osh-core"

		// Index by entity type.
		gitByType := map[string][]entityPayload{}
		for _, e := range gitDomain.entities {
			_, _, _, _, eType, _, _ := parseEntityID(e.ID)
			gitByType[eType] = append(gitByType[eType], e)
		}

		// Must have commit, author, branch entity types.
		for _, expectedType := range []string{"commit", "author", "branch"} {
			if len(gitByType[expectedType]) == 0 {
				t.Errorf("git: no %q entities", expectedType)
			}
		}

		// Branch entity must be "master" (we cloned branch=master).
		expectedBranchID := "oshtest.semsource.git." + gitSystem + ".branch.master"
		foundBranch := false
		for _, e := range gitByType["branch"] {
			if e.ID == expectedBranchID {
				foundBranch = true
				// Verify branch triples.
				assertTriplePredicate(t, e, "source.git.branch.name", "master")
				break
			}
		}
		if !foundBranch {
			t.Errorf("git: missing expected branch entity %q", expectedBranchID)
			for _, e := range gitByType["branch"] {
				t.Logf("  got branch: %s", e.ID)
			}
		}

		// Commit entity: verify system slug and required predicates.
		for _, e := range gitByType["commit"] {
			_, _, _, sys, _, _, _ := parseEntityID(e.ID)
			if sys != gitSystem {
				t.Errorf("git commit system = %q, want %q", sys, gitSystem)
			}
			// Every commit must have SHA, author, subject predicates.
			assertHasPredicate(t, e, "source.git.commit.sha")
			assertHasPredicate(t, e, "source.git.commit.author")
			assertHasPredicate(t, e, "source.git.commit.subject")
			assertHasPredicate(t, e, "source.git.commit.authored_by")
			break // One commit is enough to validate structure.
		}

		// Author entity: must have name and email predicates.
		for _, e := range gitByType["author"] {
			assertHasPredicate(t, e, "source.git.author.name")
			assertHasPredicate(t, e, "source.git.author.email")
			break
		}
	}

	// =====================================================================
	// Config assertions — Maven pom.xml and/or Gradle build.gradle entities
	// =====================================================================
	if cfgDomain := domains["config"]; cfgDomain != nil {
		cfgByType := map[string][]entityPayload{}
		for _, e := range cfgDomain.entities {
			_, _, _, _, eType, _, _ := parseEntityID(e.ID)
			cfgByType[eType] = append(cfgByType[eType], e)
		}

		// osh-core has pom.xml and build.gradle → should produce "project" entities.
		if len(cfgByType["project"]) == 0 {
			t.Error("config: no 'project' entities")
		} else {
			t.Logf("config: %d project entities, %d dependency entities",
				len(cfgByType["project"]), len(cfgByType["dependency"]))

			// Every project entity must have a file_path predicate.
			for _, proj := range cfgByType["project"] {
				assertHasPredicate(t, proj, "source.config.file_path")
			}

			// At least one project should have an artifact_id predicate
			// (Maven uses project.artifact_id, Gradle uses project.artifact_id).
			foundArtifact := false
			for _, e := range cfgByType["project"] {
				if hasPredicateValue(e, "source.config.project.artifact_id") {
					foundArtifact = true
					break
				}
			}
			if !foundArtifact {
				t.Error("config: no project entity has source.config.project.artifact_id predicate")
			}
		}

		// Should have dependency entities (Maven or Gradle).
		if len(cfgByType["dependency"]) == 0 {
			t.Error("config: no 'dependency' entities")
		} else {
			dep := cfgByType["dependency"][0]
			assertHasPredicate(t, dep, "source.config.dependency.name")
			assertHasPredicate(t, dep, "source.config.dependency.kind")
			// Dependency kind should be "maven" or "gradle".
			kindOK := false
			for _, tr := range dep.Triples {
				if tr.Predicate == "source.config.dependency.kind" {
					if s, ok := tr.Object.(string); ok && (s == "maven" || s == "gradle") {
						kindOK = true
						t.Logf("config: dependency kind=%s (id=%s)", s, dep.ID)
					}
				}
			}
			if !kindOK {
				t.Error("config: dependency entity has no valid kind (expected 'maven' or 'gradle')")
			}
		}
	}

	// =====================================================================
	// Doc assertions — markdown/text files
	// =====================================================================
	if docDomain := domains["web"]; docDomain != nil {
		if len(docDomain.entities) == 0 {
			t.Error("web: no doc entities")
		} else {
			t.Logf("web: %d doc entities", len(docDomain.entities))

			// All doc entities should have domain=web, type=doc.
			for _, e := range docDomain.entities {
				_, _, domain, _, eType, _, _ := parseEntityID(e.ID)
				if domain != "web" {
					t.Errorf("doc entity domain = %q, want 'web'", domain)
				}
				if eType != "doc" {
					t.Errorf("doc entity type = %q, want 'doc'", eType)
				}
			}

			// Verify doc entity triple structure.
			doc := docDomain.entities[0]
			assertHasPredicate(t, doc, "source.doc.file_path")
			assertHasPredicate(t, doc, "source.doc.mime_type")
			assertHasPredicate(t, doc, "source.doc.file_hash")
		}
	}

	// =====================================================================
	// Java AST assertions — classes, methods, files from osh-core
	// =====================================================================
	if javaDomain := domains["java"]; javaDomain != nil {
		javaByType := map[string][]entityPayload{}
		for _, e := range javaDomain.entities {
			_, _, _, _, eType, _, _ := parseEntityID(e.ID)
			javaByType[eType] = append(javaByType[eType], e)
		}

		t.Logf("java entity types: files=%d, classes=%d, methods=%d, interfaces=%d, vars=%d",
			len(javaByType["file"]), len(javaByType["class"]),
			len(javaByType["method"]), len(javaByType["interface"]),
			len(javaByType["var"]))

		// osh-core is a Java project — must have file entities.
		if len(javaByType["file"]) == 0 {
			t.Error("java: no 'file' entities")
		}

		// Must have class or interface entities (osh-core has plenty of both).
		classCount := len(javaByType["class"]) + len(javaByType["interface"])
		if classCount == 0 {
			t.Error("java: no class or interface entities — AST parser did not extract types")
		}

		// Verify a Java type entity (class or interface) has expected structure.
		var sampleType entityPayload
		if len(javaByType["class"]) > 0 {
			sampleType = javaByType["class"][0]
		} else if len(javaByType["interface"]) > 0 {
			sampleType = javaByType["interface"][0]
		}
		if sampleType.ID != "" {
			_, _, domain, _, _, _, _ := parseEntityID(sampleType.ID)
			if domain != "java" {
				t.Errorf("java type entity domain = %q, want 'java'", domain)
			}
			if len(sampleType.Triples) == 0 {
				t.Error("java type entity has no triples")
			}
		}
	}

	// --- Summary ---
	typeCounts := map[string]int{}
	for _, e := range allEntities {
		_, _, domain, _, eType, _, ok := parseEntityID(e.ID)
		if ok {
			typeCounts[domain+"."+eType]++
		}
	}
	t.Logf("entity type counts: %v", typeCounts)
}

// hasPredicateValue returns true if the entity has a triple with the given predicate.
func hasPredicateValue(e entityPayload, predicate string) bool {
	for _, tr := range e.Triples {
		if tr.Predicate == predicate {
			return true
		}
	}
	return false
}

// assertHasPredicate fails if the entity has no triple with the given predicate.
func assertHasPredicate(t *testing.T, e entityPayload, predicate string) {
	t.Helper()
	for _, tr := range e.Triples {
		if tr.Predicate == predicate {
			return
		}
	}
	t.Errorf("entity %s: missing predicate %q (has %d triples)", e.ID, predicate, len(e.Triples))
}

// assertTriplePredicate fails if no triple matches predicate+object.
func assertTriplePredicate(t *testing.T, e entityPayload, predicate string, wantObject any) {
	t.Helper()
	for _, tr := range e.Triples {
		if tr.Predicate == predicate {
			// Compare as strings for flexibility (JSON numbers vs strings).
			if fmt.Sprint(tr.Object) == fmt.Sprint(wantObject) {
				return
			}
		}
	}
	t.Errorf("entity %s: no triple with predicate=%q object=%v", e.ID, predicate, wantObject)
}

func TestE2E_RunFailsGracefullyWithoutNATS(t *testing.T) {
	binPath := buildBinary(t)
	workDir := t.TempDir()
	configPath := writeConfig(t, workDir, 0)

	// Point at a port where nothing is listening.
	cmd := exec.Command(binPath, "run",
		"--config", configPath,
		"--nats-url", "nats://127.0.0.1:14222",
	)
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when NATS is unavailable")
	}

	output := string(out)
	if !strings.Contains(output, "NATS") && !strings.Contains(output, "connection") {
		t.Errorf("error output should mention NATS connection failure, got: %s", output)
	}
	t.Logf("graceful failure output: %s", strings.TrimSpace(output))
}

func TestE2E_HeadlessMode(t *testing.T) {
	// Start NATS via Docker.
	natsURL, cleanup := startNATS(t)
	defer cleanup()

	binPath := buildBinary(t)
	workDir := t.TempDir()
	httpPort := freePort(t)
	wsPort := freePort(t)

	// Write headless config — no graph subsystem, no WebSocket.
	root := repoRoot(t)
	cfg := map[string]any{
		"namespace":      "headless-test",
		"mode":           "headless",
		"http_port":      httpPort,
		"websocket_bind": fmt.Sprintf("0.0.0.0:%d", wsPort),
		"sources": []map[string]any{
			{"type": "ast", "path": root, "language": "go", "watch": false},
			{"type": "docs", "paths": []string{filepath.Join(root, "docs")}, "watch": false},
			{"type": "config", "paths": []string{root}, "watch": false},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath := filepath.Join(workDir, "semsource.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Subscribe to graph ingest subject BEFORE starting semsource.
	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}

	ctx := context.Background()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "GRAPH",
		Subjects: []string{"graph.ingest.>"},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create GRAPH stream: %v", err)
	}

	cons, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	// Start semsource in headless mode.
	runCtx, runCancel := context.WithTimeout(ctx, 60*time.Second)
	defer runCancel()

	cmd := exec.CommandContext(runCtx, binPath, "run",
		"--config", configPath,
		"--log-level", "debug",
		"--nats-url", natsURL,
	)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start semsource: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.Logf("[stderr] %s", scanner.Text())
		}
	}()

	logsDone := make(chan struct{})
	go func() {
		defer close(logsDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.Logf("[stdout] %s", scanner.Text())
		}
	}()

	// Collect entity messages from NATS.
	var entities []entityPayload
	collectCtx, collectCancel := context.WithTimeout(ctx, 45*time.Second)
	defer collectCancel()

	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		for {
			if collectCtx.Err() != nil {
				return
			}
			msgs, err := cons.Fetch(10, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				continue
			}
			for msg := range msgs.Messages() {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(msg.Data(), &raw); err != nil {
					msg.Ack()
					continue
				}
				payloadJSON, ok := raw["payload"]
				if !ok {
					payloadJSON, ok = raw["data"]
				}
				if !ok {
					msg.Ack()
					continue
				}
				var payload entityPayload
				if err := json.Unmarshal(payloadJSON, &payload); err != nil {
					msg.Ack()
					continue
				}
				if payload.ID == "" {
					msg.Ack()
					continue
				}
				entities = append(entities, payload)
				msg.Ack()
				if len(entities) >= 20 {
					return
				}
			}
		}
	}()

	select {
	case <-fetchDone:
		t.Logf("collected %d entities", len(entities))
	case <-collectCtx.Done():
		t.Logf("collection timed out with %d entities", len(entities))
	}

	// Query status — should reach "ready" in headless mode.
	status := waitForReady(t, httpPort, 30*time.Second)
	t.Logf("headless status: phase=%s, total_entities=%d, sources=%d",
		status.Phase, status.TotalEntities, len(status.Sources))

	// Stop semsource.
	cmd.Process.Signal(os.Interrupt)
	cmd.Wait()
	<-logsDone

	// --- Assertions ---

	// Entities were published to NATS even without graph subsystem.
	if len(entities) == 0 {
		t.Fatal("no entities received — headless mode should still publish to graph.ingest.entity")
	}

	// Status gating works in headless mode.
	if status.Phase != "ready" {
		t.Errorf("status phase = %q, want 'ready'", status.Phase)
	}
	if status.Namespace != "headless-test" {
		t.Errorf("status namespace = %q, want %q", status.Namespace, "headless-test")
	}
	if len(status.Sources) != 3 {
		t.Errorf("status sources = %d, want 3", len(status.Sources))
	}
	if status.TotalEntities == 0 {
		t.Error("status total_entities = 0")
	}

	// WebSocket should NOT be listening (headless mode skips it).
	wsURL := fmt.Sprintf("http://127.0.0.1:%d/graph", wsPort)
	resp, err := http.Get(wsURL)
	if err == nil {
		resp.Body.Close()
		t.Errorf("WebSocket port %d should not be listening in headless mode, but got response", wsPort)
	}
	// Connection refused is expected.
}
