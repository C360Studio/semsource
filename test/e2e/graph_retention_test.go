//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// warmRestartMaxBytes caps GRAPH at about 1.5x the bytes one seed of
// writeRetentionFixture publishes, so a stream that keeps consumed messages
// cannot hold a second seed. Measured on main 34dc268 (limits retention):
// 5,750,182 bytes after ready (1041 entity, 1 manifest, 1 predicates and
// 2 status messages).
const warmRestartMaxBytes int64 = 8_625_000

// writeRetentionFixture writes a Go module whose seed size is fixed by its
// content, not by this repository's size, so the byte cap above stays valid as
// the repository grows.
func writeRetentionFixture(t *testing.T, parent string) string {
	t.Helper()
	dir := filepath.Join(parent, "retention")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create retention fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/retention\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	for f := 0; f < 40; f++ {
		var b strings.Builder
		fmt.Fprintf(&b, "package retention\n\n")
		for fn := 0; fn < 25; fn++ {
			fmt.Fprintf(&b, "// F%03d_%03d is fixture function %d of file %d.\nfunc F%03d_%03d(n int) int {\n\tif n > 0 {\n\t\treturn F%03d_%03d(n - 1)\n\t}\n\treturn n\n}\n\n",
				f, fn, fn, f, f, fn, f, (fn+1)%25)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%03d.go", f)), []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write fixture file %d: %v", f, err)
		}
	}
	return dir
}

func writeRetentionConfig(t *testing.T, workDir, fixture string, httpPort int, maxBytes int64) string {
	t.Helper()
	cfg := map[string]any{
		"namespace": "e2eretention",
		"http_port": httpPort,
		"metrics":   map[string]any{"port": freePort(t)},
		"sources": []map[string]any{
			{"type": "ast", "path": fixture, "language": "go", "watch": false},
		},
	}
	cfg["streams"] = map[string]any{"GRAPH": map[string]any{"max_bytes": maxBytes}}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(workDir, "semsource.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// graphStreamState reads GRAPH's state, including per-subject message counts.
func graphStreamState(t *testing.T, nc *nats.Conn) jetstream.StreamState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}
	stream, err := js.Stream(ctx, "GRAPH")
	if err != nil {
		t.Fatalf("open GRAPH: %v", err)
	}
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter("graph.ingest.>"))
	if err != nil {
		t.Fatalf("GRAPH info: %v", err)
	}
	return info.State
}

// assertCleanReady fails unless the status is ready with no lost, failed, or
// errored entity on any source.
func assertCleanReady(t *testing.T, label string, status statusPayload) {
	t.Helper()
	if status.Phase != "ready" {
		t.Errorf("%s: phase = %q, want ready", label, status.Phase)
	}
	for _, src := range status.Sources {
		if src.LostTotal != 0 || src.SeedLost != 0 || src.ErrorCount != 0 {
			t.Errorf("%s: source %s lost_total=%d seed_lost=%d error_count=%d, want all 0",
				label, src.InstanceName, src.LostTotal, src.SeedLost, src.ErrorCount)
		}
	}
}

// TestE2E_WarmRestartWithinStreamWindow restarts SemSource alone, at once, on
// the NATS server that holds its first seed, with GRAPH capped at about 1.5x
// one seed. A stream that keeps consumed messages refuses the second seed
// (10077) and the source ends degraded; one that removes them does not.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: The ingest transport stream refuses rather than evicts
func TestE2E_WarmRestartWithinStreamWindow(t *testing.T) {
	natsURL, cleanup := startNATS(t)
	defer cleanup()
	binPath := buildBinary(t)
	workDir := t.TempDir()
	httpPort, wsPort := freePort(t), freePort(t)
	fixture := writeRetentionFixture(t, t.TempDir())
	configPath := writeRetentionConfig(t, workDir, fixture, httpPort, warmRestartMaxBytes)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()

	stop := upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	first := waitForDrained(t, httpPort, nc, 180*time.Second)
	assertCleanReady(t, "first boot", first)
	firstCount := len(readEntityStates(t, nc))
	firstState := graphStreamState(t, nc)
	// Every applied entity message is removed, so what remains is the
	// non-entity transport (manifest, status, predicates), which has no
	// consumer and stays until MaxAge (design Open Question 5).
	var nonEntityBytes uint64
	if firstState.Subjects["graph.ingest.entity"] == 0 {
		nonEntityBytes = firstState.Bytes
	}
	t.Logf("first seed: GRAPH bytes=%d non_entity_bytes=%d last_seq=%d subjects=%v entities=%d cap=%d",
		firstState.Bytes, nonEntityBytes, firstState.LastSeq, firstState.Subjects, firstCount, warmRestartMaxBytes)
	if n := firstState.Subjects["graph.ingest.entity"]; n != 0 {
		t.Errorf("after the first seed GRAPH holds %d graph.ingest.entity messages, want 0 (applied messages are removed)", n)
	}
	stop()

	stop = upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer stop()
	second := waitForDrained(t, httpPort, nc, 180*time.Second)
	assertCleanReady(t, "second boot", second)
	secondCount := len(readEntityStates(t, nc))
	secondState := graphStreamState(t, nc)
	if n := secondState.Subjects["graph.ingest.entity"]; n != 0 {
		t.Errorf("after the second seed GRAPH holds %d graph.ingest.entity messages, want 0", n)
	}
	// ENTITY_STATES keeps the first boot's state, so an equal count alone
	// would pass a second boot that published nothing: the stream must have
	// taken at least one more seed's worth of messages.
	if published := secondState.LastSeq - firstState.LastSeq; published < uint64(firstCount) {
		t.Errorf("second boot advanced GRAPH by %d messages, want at least %d (one seed)", published, firstCount)
	}
	if secondCount != firstCount {
		t.Errorf("entities after second boot = %d, want %d (first boot)", secondCount, firstCount)
	}
}

// TestE2E_LimitsGraphStreamRefusesBoot: a NATS server still holding the
// beta.10 GRAPH stream (limits retention) refuses SemSource's boot with the
// retention drift and its remedy, rather than reusing or recreating it.
//
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: The ingest transport stream refuses rather than evicts
func TestE2E_LimitsGraphStreamRefusesBoot(t *testing.T) {
	natsURL, cleanup := startNATS(t)
	defer cleanup()
	binPath := buildBinary(t)
	workDir := t.TempDir()
	fixture := writeRetentionFixture(t, t.TempDir())
	configPath := writeRetentionConfig(t, workDir, fixture, freePort(t), warmRestartMaxBytes)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// The beta.10 declaration: same subjects and storage, limits retention.
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      "GRAPH",
		Subjects:  []string{"graph.ingest.entity", "graph.ingest.batch", "graph.ingest.manifest", "graph.ingest.status", "graph.ingest.predicates"},
		Storage:   jetstream.MemoryStorage,
		Retention: jetstream.LimitsPolicy,
		MaxBytes:  256 * 1024 * 1024,
		MaxAge:    time.Hour,
		Discard:   jetstream.DiscardNew,
	}); err != nil {
		t.Fatalf("create limits GRAPH: %v", err)
	}

	cmd := exec.CommandContext(ctx, binPath, "run", "--config", configPath, "--nats-url", natsURL)
	cmd.Dir = workDir
	out, runErr := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("semsource did not exit within the deadline: %v\n%s", ctx.Err(), out)
	}
	if runErr == nil {
		t.Fatalf("semsource booted over a limits GRAPH stream, want a refusal\n%s", out)
	}
	for _, want := range []string{
		`reconcile stream "GRAPH" failed: existing stream diverges in a field that cannot be reconciled in place`,
		`refuses a retention change to or from "workqueue"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("boot output lacks %q\n%s", want, out)
		}
	}
	stream, err := js.Stream(ctx, "GRAPH")
	if err != nil {
		t.Fatalf("GRAPH after refused boot: %v", err)
	}
	if got := stream.CachedInfo().Config.Retention; got != jetstream.LimitsPolicy {
		t.Errorf("GRAPH retention after refused boot = %v, want limits (left untouched)", got)
	}
}

// jsStoreFailed is JetStream's err_code for a publish the stream refuses at its
// byte bound ("maximum bytes exceeded" under discard new).
const jsStoreFailed jetstream.ErrorCode = 10077

// watchedWriter is a running semsource whose exit and output the test can see.
type watchedWriter struct {
	exited <-chan struct{}
	stop   func()
	mu     sync.Mutex
	out    strings.Builder
}

func (w *watchedWriter) output() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.String()
}

// startWatchedWriter starts semsource like upgradeStartWriter, but exposes its
// exit and keeps its output, so a test can tell a crash from a running process.
func startWatchedWriter(t *testing.T, binPath, configPath, workDir, natsURL string, httpPort, wsPort int) *watchedWriter {
	t.Helper()
	cmd := exec.Command(binPath, "run", "--config", configPath, "--log-level", "info", "--nats-url", natsURL)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SEMSOURCE_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMSOURCE_WS_BIND=127.0.0.1:%d", wsPort),
	)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		t.Fatalf("start semsource: %v", err)
	}
	w := &watchedWriter{}
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("[semsource] %s", line)
			w.mu.Lock()
			w.out.WriteString(line + "\n")
			w.mu.Unlock()
		}
	}()
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		_ = pw.Close()
		<-scanned
		close(exited)
	}()
	w.exited = exited
	var once sync.Once
	w.stop = func() {
		once.Do(func() {
			select {
			case <-exited:
				return
			default:
			}
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-exited:
				if waitErr != nil {
					t.Errorf("semsource exit: %v", waitErr)
				}
			case <-time.After(35 * time.Second):
				_ = cmd.Process.Kill()
				<-exited
				t.Errorf("semsource did not stop gracefully within 35s")
			}
		})
	}
	return w
}

// fillGraphStream republishes captured entity envelopes, with no Msg-Id, until
// GRAPH refuses one at its byte bound, then pads the room left (less than one
// envelope, but enough for a manifest) with tiny graph.ingest.status messages,
// which no consumer reads. It returns how many entity envelopes it published.
func fillGraphStream(t *testing.T, nc *nats.Conn, envelopes [][]byte) int {
	t.Helper()
	published := publishUntilRefused(t, nc, "graph.ingest.entity", envelopes)
	publishUntilRefused(t, nc, "graph.ingest.status", [][]byte{[]byte("{}")})
	return published
}

// publishUntilRefused publishes payloads round-robin to subject until GRAPH
// refuses one with 10077, and returns how many it accepted.
func publishUntilRefused(t *testing.T, nc *nats.Conn, subject string, envelopes [][]byte) int {
	t.Helper()
	if len(envelopes) == 0 {
		t.Fatal("no entity envelopes captured to fill GRAPH with")
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create jetstream context: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	published := 0
	for pass := 0; pass < 100_000/len(envelopes)+1; pass++ {
		for _, data := range envelopes {
			_, err := js.Publish(ctx, subject, data)
			if err == nil {
				published++
				continue
			}
			var jsErr jetstream.JetStreamError
			if errors.As(err, &jsErr) && jsErr.APIError() != nil && jsErr.APIError().ErrorCode == jsStoreFailed {
				return published
			}
			t.Fatalf("fill GRAPH on %s after %d messages: %v", subject, published, err)
		}
	}
	t.Fatalf("GRAPH accepted %d messages on %s without refusing; is it bounded?", published, subject)
	return published
}

// TestE2E_BootIntoFullGraphStream boots SemSource onto a GRAPH stream already
// at its byte bound with entity messages graph-ingest has not consumed, the
// state stacked boots leave behind (#399). SemSource must stay up, graph-ingest
// must drain the stream, and the manifest broadcast must land once there is
// room.
//
// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting
func TestE2E_BootIntoFullGraphStream(t *testing.T) {
	natsURL, cleanup := startNATS(t)
	defer cleanup()
	binPath := buildBinary(t)
	workDir := t.TempDir()
	httpPort, wsPort := freePort(t), freePort(t)
	fixture := writeRetentionFixture(t, t.TempDir())
	configPath := writeRetentionConfig(t, workDir, fixture, httpPort, warmRestartMaxBytes)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()

	// Capture the real envelopes boot 1 publishes, so the filler is entities
	// graph-ingest applies (and so removes), not junk it might never drain.
	var capMu sync.Mutex
	var captured [][]byte
	sub, err := nc.Subscribe("graph.ingest.entity", func(m *nats.Msg) {
		capMu.Lock()
		defer capMu.Unlock()
		captured = append(captured, append([]byte(nil), m.Data...))
	})
	if err != nil {
		t.Fatalf("subscribe graph.ingest.entity: %v", err)
	}
	if err := sub.SetPendingLimits(-1, -1); err != nil {
		t.Fatalf("set pending limits: %v", err)
	}

	stop := upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	first := waitForDrained(t, httpPort, nc, 180*time.Second)
	assertCleanReady(t, "first boot", first)
	firstCount := len(readEntityStates(t, nc))
	stop()
	if err := sub.Unsubscribe(); err != nil {
		t.Fatalf("unsubscribe graph.ingest.entity: %v", err)
	}
	capMu.Lock()
	envelopes := captured
	capMu.Unlock()

	filled := fillGraphStream(t, nc, envelopes)
	before := graphStreamState(t, nc)
	t.Logf("filled GRAPH: published=%d bytes=%d cap=%d subjects=%v", filled, before.Bytes, warmRestartMaxBytes, before.Subjects)

	w := startWatchedWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer w.stop()
	select {
	case <-w.exited:
		t.Fatalf("semsource exited while booting into a full GRAPH stream, want it running\n%s", w.output())
	case <-time.After(30 * time.Second):
	}

	second := waitForDrained(t, httpPort, nc, 180*time.Second)
	assertCleanReady(t, "boot into full stream", second)
	after := graphStreamState(t, nc)
	if n := after.Subjects["graph.ingest.entity"]; n != 0 {
		t.Errorf("after the boot GRAPH holds %d graph.ingest.entity messages, want 0 (drained)", n)
	}
	if got := len(readEntityStates(t, nc)); got != firstCount {
		t.Errorf("entities after boot into full stream = %d, want %d (first boot)", got, firstCount)
	}
	// The retried manifest lands after graph-ingest makes room, so it can
	// trail the drain by one backoff step.
	deadline := time.Now().Add(30 * time.Second)
	for after.Subjects["graph.ingest.manifest"] <= before.Subjects["graph.ingest.manifest"] && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		after = graphStreamState(t, nc)
	}
	if got, was := after.Subjects["graph.ingest.manifest"], before.Subjects["graph.ingest.manifest"]; got <= was {
		t.Errorf("graph.ingest.manifest count = %d after the boot, want more than %d (the manifest broadcast was never delivered)", got, was)
	}
}
