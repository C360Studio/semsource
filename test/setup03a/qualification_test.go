//go:build qualification

// Package setup03a qualifies a compiled SemSource through its public wire interfaces.
package setup03a

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type observation struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail any    `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}
type report struct {
	CorpusVersion   string         `json:"corpus_version"`
	Profile         string         `json:"profile"`
	SemEngineSlices []int          `json:"semengine_slices"`
	Started         time.Time      `json:"started"`
	BinarySHA256    string         `json:"binary_sha256"`
	CorpusSHA256    string         `json:"corpus_sha256"`
	Configuration   map[string]any `json:"configuration"`
	NATSContainer   string         `json:"nats_container"`
	Observations    []observation  `json:"observations"`
}
type harness struct {
	t                                                            *testing.T
	ctx                                                          context.Context
	out, source, binary, configPath, container, natsURL, httpURL string
	nc                                                           *nats.Conn
	js                                                           jetstream.JetStream
	cmd                                                          *exec.Cmd
	done                                                         chan error
	log                                                          *os.File
	starts                                                       int
	result                                                       report
	ids                                                          map[string]string
}
type exact struct {
	Entity *struct {
		ID      string `json:"id"`
		Version uint64 `json:"version"`
		Triples []struct {
			Predicate string `json:"predicate"`
			Object    any    `json:"object"`
		} `json:"triples"`
	} `json:"entity"`
	Revision uint64 `json:"kvRevision"`
}
type node struct {
	Name, Path, Body, Handle string
	BodyReason               string                                   `json:"body_reason"`
	Relations                map[string][]struct{ Name, Path string } `json:"relations"`
}
type fused struct {
	Nodes      []node `json:"nodes"`
	Deferred   bool   `json:"deferred"`
	Unhydrated []any  `json:"unhydrated"`
	Misses     []any  `json:"misses"`
	Index      struct {
		Ready     bool   `json:"ready"`
		State     string `json:"state"`
		Bootstrap bool   `json:"bootstrap_complete"`
	} `json:"index"`
}

func TestKnownAnswerCorpus(t *testing.T) {
	binary, out := os.Getenv("SETUP03A_BINARY"), os.Getenv("SETUP03A_OUT")
	if binary == "" || out == "" {
		t.Fatal("SETUP03A_BINARY and SETUP03A_OUT are required; qualification never silently skips")
	}
	profile := os.Getenv("SETUP03A_PROFILE")
	if profile == "" {
		profile = "bm25"
	}
	if profile != "bm25" && profile != "neural" {
		t.Fatalf("unknown profile %q", profile)
	}
	if profile == "neural" && os.Getenv("SETUP03A_PROVIDER") == "" {
		t.Fatal("neural qualification requires a real SETUP03A_PROVIDER endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	h := &harness{t: t, ctx: ctx, out: out, binary: binary, ids: map[string]string{}}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	h.result = report{CorpusVersion: "1", Profile: profile, SemEngineSlices: []int{0, 1}, Started: time.Now().UTC()}
	if profile == "neural" {
		h.result.SemEngineSlices = []int{0, 2}
	}
	defer h.finish()
	b, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(b)
	h.result.BinarySHA256 = hex.EncodeToString(digest[:])
	h.fixture()
	h.broker()
	h.configure(profile)
	if profile == "neural" {
		h.providerMetrics("before")
	}
	h.start()
	if !h.check("ingestion_ready", 60*time.Second, func() (any, error) {
		raw, err := h.rpc("graph.query.status", map[string]any{})
		if err != nil {
			return nil, err
		}
		var status struct {
			Phase   string `json:"phase"`
			Total   int    `json:"total_entities"`
			Sources []struct {
				Offered   int64 `json:"offered_total"`
				Delivered int64 `json:"delivered_total"`
				Lost      int64 `json:"lost_total"`
				SeedLost  int64 `json:"seed_lost"`
			} `json:"sources"`
		}
		if err = json.Unmarshal(raw, &status); err != nil {
			return nil, err
		}
		if status.Phase != "ready" || status.Total < 6 {
			return json.RawMessage(raw), fmt.Errorf("phase=%s entities=%d", status.Phase, status.Total)
		}
		for _, source := range status.Sources {
			if source.Offered != source.Delivered || source.Lost != 0 || source.SeedLost != 0 {
				return json.RawMessage(raw), fmt.Errorf("source delivery unsettled: offered=%d delivered=%d lost=%d seed_lost=%d", source.Offered, source.Delivered, source.Lost, source.SeedLost)
			}
		}
		return json.RawMessage(raw), nil
	}) {
		return
	}
	h.resolveIdentity()
	h.structural("initial", false)
	h.queries("cold")
	if profile == "neural" {
		h.providerMetrics("after_cold")
	}
	h.queries("warm")
	if profile == "neural" {
		h.providerMetrics("after_warm")
		h.check("neural_paraphrase", 30*time.Second, func() (any, error) {
			f, queryErr := h.fuse("doc", "context", "where should a blue package be sent", 10)
			if queryErr != nil {
				return f, queryErr
			}
			for _, n := range f.Nodes {
				if strings.Contains(n.Body, "north depot") {
					return f, nil
				}
			}
			return f, errors.New("paraphrase did not retrieve north-depot passage")
		})
	}
	h.check("graph_search_public_query", 30*time.Second, func() (any, error) {
		raw, err := h.rpc("graph.query.searchGraph", map[string]any{"query": "cobalt lantern routing", "include_sources": true, "include_summaries": false})
		if err != nil {
			return nil, err
		}
		var v map[string]any
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		if entities, ok := v["entities"].([]any); !ok || len(entities) == 0 {
			return json.RawMessage(raw), errors.New("searchGraph returned no corpus entities")
		}
		return json.RawMessage(raw), nil
	})
	alphaPath := filepath.Join(h.source, "alpha/greet.go")
	original, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(original, []byte("return Greet()"), []byte("return Farewell()"), 1)
	if err = os.WriteFile(alphaPath, edited, 0644); err != nil {
		t.Fatal(err)
	}
	h.structural("edited", true)
	if err = os.Remove(alphaPath); err != nil {
		t.Fatal(err)
	}
	h.lifecycle("deleted")
	h.check("deleted_retains_stale_history", 30*time.Second, func() (any, error) {
		e, err := h.entity(h.ids["run"])
		if err != nil {
			return e, err
		}
		if value(e, "entity.lifecycle.stale") == nil {
			return e, errors.New("deleted entity is retained but lacks stale marker")
		}
		return e, nil
	})
	h.check("deleted_query_visibility", 10*time.Second, func() (any, error) {
		f, err := h.fuse("code", "context", "Run", 10)
		if err != nil {
			return f, err
		}
		if len(f.Nodes) != 1 {
			return f, fmt.Errorf("retained Run results=%d want 1", len(f.Nodes))
		}
		return f, nil
	})
	if err = os.WriteFile(alphaPath, edited, 0644); err != nil {
		t.Fatal(err)
	}
	h.lifecycle("recreated")
	h.check("recreated_same_identity_not_stale", 30*time.Second, func() (any, error) {
		e, err := h.entity(h.ids["run"])
		if err != nil {
			return e, err
		}
		if value(e, "entity.lifecycle.stale") != nil {
			return e, errors.New("recreated entity still marked stale")
		}
		return e, nil
	})
	h.structural("recreated", true)
	h.stop(false)
	h.start()
	h.structural("application_restart", true)
	h.pendingBrokerRestart(edited)
	h.rpcCollision()
}

func (h *harness) fixture() {
	h.t.Helper()
	h.source = filepath.Join(h.out, "source")
	var hashInput []byte
	err := filepath.WalkDir("fixtures/v1", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel("fixtures/v1", path)
		if e != nil {
			return e
		}
		dest := filepath.Join(h.source, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		hashInput = append(hashInput, []byte(rel)...)
		hashInput = append(hashInput, data...)
		return os.WriteFile(dest, data, 0644)
	})
	if err != nil {
		h.t.Fatal(err)
	}
	sum := sha256.Sum256(hashInput)
	h.result.CorpusSHA256 = hex.EncodeToString(sum[:])
	raw, err := os.ReadFile("fixtures/v1/manifest.json")
	if err != nil {
		h.t.Fatal(err)
	}
	var m struct {
		IDs map[string]string `json:"expected_entities"`
	}
	if err = json.Unmarshal(raw, &m); err != nil {
		h.t.Fatal(err)
	}
	h.ids = m.IDs
}
func (h *harness) command(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(h.ctx, 45*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
}
func (h *harness) broker() {
	h.t.Helper()
	image := os.Getenv("SETUP03A_NATS_IMAGE")
	if image == "" {
		image = "nats:2.14.4-alpine"
	}
	name := fmt.Sprintf("semsource-setup03a-%d-%d", os.Getpid(), time.Now().UnixNano())
	out, err := h.command("docker", "run", "-d", "--name", name, "--label", "semsource.qualification=setup03a", "--cpus", "1", "--memory", "512m", "-p", "127.0.0.1::4222", image, "-js", "-sd", "/data")
	if err != nil {
		h.t.Fatalf("broker: %v: %s", err, out)
	}
	h.container = strings.TrimSpace(string(out))
	h.result.NATSContainer = h.container
	out, err = h.command("docker", "port", h.container, "4222/tcp")
	if err != nil {
		h.t.Fatal(err)
	}
	h.natsURL = "nats://" + strings.TrimSpace(string(out))
	h.connect()
	info, err := h.command("docker", "inspect", h.container)
	if err != nil {
		h.t.Fatal(err)
	}
	h.save("nats-inspect.json", info)
}
func (h *harness) connect() {
	h.t.Helper()
	// Docker may assign a new ephemeral host port after restart. Read the
	// binding from the same owned container instead of guessing it persisted.
	port, portErr := h.command("docker", "port", h.container, "4222/tcp")
	if portErr != nil {
		h.t.Fatal(portErr)
	}
	h.natsURL = "nats://" + strings.TrimSpace(string(port))
	if h.nc != nil {
		h.nc.Close()
	}
	var last error
	if !h.poll(20*time.Second, func() bool {
		h.nc, last = nats.Connect(h.natsURL, nats.Timeout(time.Second), nats.NoReconnect())
		return last == nil
	}) {
		h.t.Fatalf("NATS readiness: %v", last)
	}
	var err error
	h.js, err = jetstream.New(h.nc)
	if err != nil {
		h.t.Fatal(err)
	}
}
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
func (h *harness) configure(profile string) {
	cfgPath := "../../configs/tiers/tier0-statistical.json"
	if profile == "neural" {
		cfgPath = "../../configs/tiers/tier1-semantic.json"
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		h.t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(raw, &cfg); err != nil {
		h.t.Fatal(err)
	}
	port := freePort(h.t)
	h.httpURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	cfg["namespace"] = "setup03a"
	cfg["http_port"] = port
	cfg["metrics"] = map[string]any{"port": freePort(h.t)}
	cfg["websocket_bind"] = fmt.Sprintf("127.0.0.1:%d", freePort(h.t))
	cfg["sources"] = []map[string]any{{"type": "ast", "path": h.source, "languages": []string{"go"}, "watch": true, "project": "setup03a", "version": "corpus-v1"}, {"type": "docs", "paths": []string{filepath.Join(h.source, "docs")}, "watch": true, "project": "setup03a-docs"}}
	cfg["source_roots"] = []string{h.source}
	cfg["graph"].(map[string]any)["gateway_bind"] = fmt.Sprintf("127.0.0.1:%d", freePort(h.t))
	if profile == "neural" {
		cfg["model_registry"].(map[string]any)["endpoints"].(map[string]any)["semembed"].(map[string]any)["url"] = os.Getenv("SETUP03A_PROVIDER")
	}
	h.result.Configuration = cfg
	raw, err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		h.t.Fatal(err)
	}
	h.configPath = filepath.Join(h.out, "semsource.json")
	h.save("semsource.json", raw)
}
func (h *harness) start() {
	h.t.Helper()
	h.starts++
	f, err := os.Create(filepath.Join(h.out, fmt.Sprintf("application-%d.log", h.starts)))
	if err != nil {
		h.t.Fatal(err)
	}
	h.log = f
	cmd := exec.Command(h.binary, "run", "--config", h.configPath, "--log-level", "debug", "--nats-url", h.natsURL)
	cmd.Dir = h.out
	cmd.Stdout = f
	cmd.Stderr = f
	if err = cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	h.cmd = cmd
	h.done = make(chan error, 1)
	done := h.done
	go func() { done <- cmd.Wait() }()
	h.t.Logf("application pid=%d container=%s evidence=%s", h.cmd.Process.Pid, h.container, h.out)
}
func (h *harness) stop(force bool) {
	if h.cmd == nil {
		return
	}
	cmd := h.cmd
	h.cmd = nil
	signal := os.Interrupt
	if force {
		signal = os.Kill
	}
	if err := cmd.Process.Signal(signal); err != nil && !errors.Is(err, os.ErrProcessDone) {
		h.t.Errorf("signal application: %v", err)
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-h.done:
	case <-timer.C:
		_ = cmd.Process.Kill()
		select {
		case <-h.done:
		case <-time.After(3 * time.Second):
			h.t.Error("application failed to join after kill")
		}
	}
	if h.log != nil {
		if err := h.log.Close(); err != nil {
			h.t.Error(err)
		}
		h.log = nil
	}
}
func (h *harness) finish() {
	h.stop(false)
	if h.nc != nil {
		h.nc.Close()
	}
	if h.container != "" {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		logs, err := exec.CommandContext(cleanupCtx, "docker", "logs", h.container).CombinedOutput()
		if err != nil {
			h.t.Errorf("capture broker logs: %v", err)
		}
		h.save("nats.log", logs)
		out, err := exec.CommandContext(cleanupCtx, "docker", "rm", "-f", h.container).CombinedOutput()
		if err != nil {
			h.t.Errorf("cleanup owned container %s: %v: %s", h.container, err, out)
		}
	}
	raw, err := json.MarshalIndent(h.result, "", "  ")
	if err != nil {
		h.t.Error(err)
		return
	}
	h.save("report.json", raw)
}
func (h *harness) save(name string, b []byte) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.out, name), b, 0644); err != nil {
		h.t.Error(err)
	}
}
func (h *harness) poll(timeout time.Duration, probe func() bool) bool {
	ctx, cancel := context.WithTimeout(h.ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if probe() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
func (h *harness) check(name string, timeout time.Duration, probe func() (any, error)) bool {
	var detail any
	var err error
	ok := h.poll(timeout, func() bool { detail, err = probe(); return err == nil })
	o := observation{Name: name, Passed: ok, Detail: detail}
	if err != nil {
		o.Error = err.Error()
	}
	h.result.Observations = append(h.result.Observations, o)
	if !ok {
		h.t.Errorf("%s: %v", name, err)
	} else {
		h.t.Logf("PASS %s", name)
	}
	raw, e := json.MarshalIndent(h.result, "", "  ")
	if e == nil {
		h.save("report.json", raw)
	}
	return ok
}
func (h *harness) rpc(subject string, input any) ([]byte, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()
	msg, err := h.nc.RequestWithContext(ctx, subject, raw)
	if err != nil {
		return nil, err
	}
	var status struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(msg.Data, &status) == nil && status.Error != nil {
		return msg.Data, fmt.Errorf("RPC %s: %s", subject, msg.Data)
	}
	return msg.Data, nil
}
func (h *harness) entity(id string) (exact, error) {
	var e exact
	raw, err := h.rpc("graph.query.entity", map[string]any{"id": id})
	if err != nil {
		return e, err
	}
	if err = json.Unmarshal(raw, &e); err != nil {
		return e, err
	}
	if e.Entity == nil || e.Entity.ID != id || e.Revision == 0 {
		return e, fmt.Errorf("missing exact entity/revision: %s", raw)
	}
	return e, nil
}
func value(e exact, predicate string) any {
	if e.Entity != nil {
		for _, tr := range e.Entity.Triples {
			if tr.Predicate == predicate {
				return tr.Object
			}
		}
	}
	return nil
}
func values(e exact, predicate string) []string {
	var result []string
	if e.Entity != nil {
		for _, tr := range e.Entity.Triples {
			if tr.Predicate == predicate {
				result = append(result, fmt.Sprint(tr.Object))
			}
		}
	}
	sort.Strings(result)
	return result
}
func (h *harness) fuse(lens, verb, query string, limit int) (fused, error) {
	var result fused
	raw, err := json.Marshal(map[string]any{"query": query, "want": []string{"body", "relations"}, "budget": map[string]any{"max_nodes": limit}, "include_scores": true})
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.httpURL+"/"+lens+"-context/"+verb, bytes.NewReader(raw))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if resp.StatusCode != 200 {
		return result, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return result, err
	}
	if result.Deferred || len(result.Unhydrated) > 0 {
		return result, errors.New("query deferred or partially hydrated")
	}
	if !result.Index.Bootstrap {
		return result, errors.New("index bootstrap incomplete")
	}
	return result, nil
}
func (h *harness) resolveIdentity() {
	if os.Getenv("SETUP03A_IDENTITY") != "governed" {
		return
	}
	h.check("governed_identity_authority", 30*time.Second, func() (any, error) {
		raw, err := h.rpc("graph.query.byName", map[string]any{"name": "Run", "limit": 10})
		if err != nil {
			return nil, err
		}
		var response struct {
			Data struct {
				Matches []struct {
					ID string `json:"entity_id"`
				} `json:"matches"`
			} `json:"data"`
		}
		if err = json.Unmarshal(raw, &response); err != nil {
			return nil, err
		}
		if len(response.Data.Matches) != 1 {
			return json.RawMessage(raw), errors.New("expected one Run anchor")
		}
		parts := strings.Split(response.Data.Matches[0].ID, ".")
		if len(parts) != 6 || parts[0] != "setup03a" || parts[2] != "setup03a-corpus-v1" || parts[3] != "golang" || parts[4] != "function" || parts[5] != "alpha-greet-go-Run" {
			return json.RawMessage(raw), errors.New("identity violates pinned org/platform/system/domain/type/instance contract")
		}
		authority, authorityErr := h.platformIdentity()
		if authorityErr != nil {
			return json.RawMessage(raw), authorityErr
		}
		if authority["id"] != parts[1] || authority["org"] != "setup03a" || authority["stem"] != "semsource" {
			return authority, errors.New("observed platform differs from persisted platform_identity authority")
		}
		// Only the broker-authoritative platform is variable. Every other segment is
		// fixed by the versioned fixture; subsequent restart assertions reuse these IDs.
		for key, id := range h.ids {
			p := strings.Split(id, ".")
			h.ids[key] = strings.Join([]string{p[0], parts[1], p[3], p[2], p[4], p[5]}, ".")
		}
		return map[string]any{"platform": parts[1], "expected_entities": h.ids, "wire": json.RawMessage(raw)}, nil
	})
}
func (h *harness) structural(stage string, edited bool) {
	if os.Getenv("SETUP03A_IDENTITY") == "governed" {
		h.check(stage+"_persisted_authority", 10*time.Second, func() (any, error) {
			authority, err := h.platformIdentity()
			if err != nil {
				return authority, err
			}
			if authority["id"] != strings.Split(h.ids["run"], ".")[1] {
				return authority, errors.New("persisted authority changed")
			}
			return authority, nil
		})
	}
	for _, key := range []string{"run", "greet_alpha", "greet_beta", "farewell"} {
		key := key
		h.check(stage+"_entity_"+key, 20*time.Second, func() (any, error) { return h.entity(h.ids[key]) })
	}
	target := h.ids["greet_alpha"]
	body := "func Run() string { return Greet() }"
	if edited {
		target = h.ids["farewell"]
		body = "func Run() string { return Farewell() }"
	}
	h.check(stage+"_exact_relationship", 30*time.Second, func() (any, error) {
		e, err := h.entity(h.ids["run"])
		if err != nil {
			return e, err
		}
		calls := values(e, "code.relationship.calls")
		if len(calls) != 1 || calls[0] != target {
			return e, fmt.Errorf("calls=%v want exactly %s", calls, target)
		}
		return e, nil
	})
	h.check(stage+"_provenance", 20*time.Second, func() (any, error) {
		e, err := h.entity(h.ids["run"])
		if err != nil {
			return e, err
		}
		if value(e, "code.artifact.path") != "alpha/greet.go" || value(e, "code.artifact.project") != "setup03a" || value(e, "code.artifact.version") != "corpus-v1" {
			return e, errors.New("source path/project/revision mismatch")
		}
		return e, nil
	})
	h.check(stage+"_exact_content", 30*time.Second, func() (any, error) {
		f, err := h.fuse("code", "context", "Run", 10)
		if err != nil {
			return f, err
		}
		if len(f.Nodes) != 1 || f.Nodes[0].Handle != h.ids["run"] || f.Nodes[0].Body != body {
			return f, fmt.Errorf("expected one Run with exact body %q", body)
		}
		return f, nil
	})
}
func (h *harness) queries(stage string) {
	h.check(stage+"_duplicate_name_anchors", 30*time.Second, func() (any, error) {
		f, err := h.fuse("code", "context", "Greet", 10)
		if err != nil {
			return f, err
		}
		got := []string{}
		for _, n := range f.Nodes {
			got = append(got, n.Handle)
		}
		sort.Strings(got)
		want := []string{h.ids["greet_alpha"], h.ids["greet_beta"]}
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return f, fmt.Errorf("anchors=%v want %v", got, want)
		}
		return f, nil
	})
	h.check(stage+"_exact_absence", 20*time.Second, func() (any, error) {
		f, err := h.fuse("code", "context", "AbsentQualificationSymbol", 10)
		if err != nil {
			return f, err
		}
		if len(f.Nodes) != 0 || len(f.Misses) == 0 || !f.Index.Ready {
			return f, errors.New("absent fixture needs caught-up empty lookup with miss")
		}
		return f, nil
	})
	for _, lens := range []string{"code", "doc"} {
		lens := lens
		h.check(stage+"_"+lens+"_scope_before_limit", 45*time.Second, func() (any, error) {
			f, err := h.fuse(lens, "context", "cobalt lantern routing", 1)
			if err != nil {
				return f, err
			}
			if len(f.Nodes) != 1 {
				return f, fmt.Errorf("nodes=%d want 1", len(f.Nodes))
			}
			n := f.Nodes[0]
			if lens == "code" && (n.Body == "" || n.BodyReason != "") {
				return f, errors.New("missing body")
			}
			domain := ".golang."
			if lens == "doc" {
				domain = ".web."
			}
			if !strings.Contains(n.Handle, domain) {
				return f, fmt.Errorf("scope leak: %s", n.Handle)
			}
			return f, nil
		})
	}
	h.check(stage+"_doc_exact_passage_content", 30*time.Second, func() (any, error) {
		f, err := h.fuse("doc", "context", "cobalt lantern routing", 10)
		if err != nil {
			return f, err
		}
		for _, n := range f.Nodes {
			if strings.Contains(n.Handle, ".chunk.") {
				expected, readErr := os.ReadFile(filepath.Join(h.source, "docs/routing.md"))
				if readErr != nil {
					return f, readErr
				}
				if n.Body != string(expected) {
					return f, errors.New("document passage is not byte-exact corpus content")
				}
				return f, nil
			}
		}
		return f, errors.New("no passage returned for exact corpus content")
	})
}
func (h *harness) lifecycle(stage string) {
	h.check(stage+"_lifecycle_pass", 20*time.Second, func() (any, error) {
		raw, err := h.rpc("graph.lifecycle.run", map[string]any{"org": "setup03a", "systems": []string{"setup03a-corpus-v1"}, "root_path": h.source, "reason": "path_missing"})
		return json.RawMessage(raw), err
	})
}
func (h *harness) pendingBrokerRestart(edited []byte) {
	stream, err := h.js.Stream(h.ctx, "GRAPH")
	if err != nil {
		h.t.Error(err)
		return
	}
	info, err := stream.Info(h.ctx)
	if err != nil {
		h.t.Error(err)
		return
	}
	e, err := h.entity(h.ids["run"])
	if err != nil {
		h.t.Error(err)
		return
	}
	var captured *jetstream.RawStreamMsg
	for seq := info.State.LastSeq; seq >= info.State.FirstSeq && seq > 0; seq-- {
		msg, getErr := stream.GetMsg(h.ctx, seq)
		if getErr == nil && msg.Subject == "graph.ingest.entity" && bytes.Contains(msg.Data, []byte(h.ids["run"])) {
			captured = msg
			break
		}
	}
	if captured == nil {
		h.t.Error("no authentic Run envelope captured for pending-write probe")
		return
	}
	h.save("pending-authentic-envelope.json", captured.Data)
	if err = h.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		h.t.Error(err)
		return
	}
	// While every application worker is stopped, an authentic replay is accepted
	// by transport, but cannot advance authority/index state. The filesystem edit
	// is an independent changed source whose recovery requires re-ingestion.
	pending := bytes.Replace(edited, []byte("return Farewell()"), []byte("return Greet()"), 1)
	if err = os.WriteFile(filepath.Join(h.source, "alpha/greet.go"), pending, 0644); err != nil {
		h.t.Error(err)
		h.stop(true)
		return
	}
	ack, err := h.js.Publish(h.ctx, captured.Subject, captured.Data)
	if err != nil {
		h.t.Error(err)
		h.stop(true)
		return
	}
	kv, err := h.js.KeyValue(h.ctx, "ENTITY_STATES")
	var actualRevision uint64
	if err == nil {
		var entry jetstream.KeyValueEntry
		entry, err = kv.Get(h.ctx, h.ids["run"])
		if err == nil {
			actualRevision = entry.Revision()
		}
	}
	// beta.163 prefixes bucket names by org; discover only this private broker's
	// authority bucket, and retain the full bucket list as evidence.
	if err != nil {
		names := h.js.KeyValueStoreNames(h.ctx)
		for name := range names.Name() {
			if strings.HasSuffix(name, "ENTITY_STATES") {
				kv, err = h.js.KeyValue(h.ctx, name)
				if err == nil {
					entry, getErr := kv.Get(h.ctx, h.ids["run"])
					err = getErr
					if err == nil {
						actualRevision = entry.Revision()
						break
					}
				}
			}
		}
	}
	h.result.Observations = append(h.result.Observations, observation{Name: "accepted_unindexed_before_broker_restart", Passed: err == nil && actualRevision == e.Revision, Detail: map[string]any{"ack": ack, "storage": info.Config.Storage, "authority_before": e.Revision, "authority_after_ack": actualRevision}})
	if err != nil || actualRevision != e.Revision {
		h.t.Errorf("cannot prove pending unindexed work: %v before=%d after=%d", err, e.Revision, actualRevision)
	}
	h.stop(true)
	out, err := h.command("docker", "restart", "--time", "5", h.container)
	if err != nil {
		h.t.Errorf("persistent broker restart: %v: %s", err, out)
		return
	}
	h.connect()
	stream, err = h.js.Stream(h.ctx, "GRAPH")
	var pendingSurvived bool
	if err == nil {
		_, err = stream.GetMsg(h.ctx, ack.Sequence)
		pendingSurvived = err == nil
	}
	h.result.Observations = append(h.result.Observations, observation{Name: "memory_transport_not_durable", Passed: !pendingSurvived, Detail: map[string]any{"pending_sequence": ack.Sequence, "survived": pendingSurvived, "contract": "source re-ingestion, not publish-ack durability"}})
	if pendingSurvived {
		h.t.Error("memory transport unexpectedly survived; review durability observation")
	}
	h.check("authority_survives_broker_restart_before_reingest", 10*time.Second, func() (any, error) {
		persisted, openErr := h.js.KeyValue(h.ctx, kv.Bucket())
		if openErr != nil {
			return nil, openErr
		}
		entry, getErr := persisted.Get(h.ctx, h.ids["run"])
		if getErr != nil {
			return nil, getErr
		}
		if entry.Revision() != actualRevision {
			return entry.Revision(), errors.New("authority revision changed across broker restart without application")
		}
		return map[string]any{"bucket": kv.Bucket(), "revision": entry.Revision(), "entity": json.RawMessage(entry.Value())}, nil
	})
	h.check("content_survives_broker_restart_before_reingest", 10*time.Second, func() (any, error) {
		store, openErr := h.js.ObjectStore(h.ctx, "CONTENT")
		if openErr != nil {
			return nil, openErr
		}
		body, getErr := store.GetBytes(h.ctx, fmt.Sprint(value(e, "code.body.key")))
		if getErr != nil {
			return nil, getErr
		}
		if string(body) != "func Run() string { return Farewell() }" {
			return string(body), errors.New("persisted content does not match pre-restart reference")
		}
		return string(body), nil
	})
	h.start()
	h.structural("broker_restart_reingested", false)
	h.captureRestartGuard()
}
func (h *harness) rpcCollision() {
	// The healthy explicit-subject composition must already have served exact
	// queries above. Widen only this private broker after all semantic checks.
	stream, err := h.js.Stream(h.ctx, "GRAPH")
	if err != nil {
		h.t.Error(err)
		return
	}
	info, err := stream.Info(h.ctx)
	if err != nil {
		h.t.Error(err)
		return
	}
	h.stop(false)
	info.Config.Subjects = []string{"graph.ingest.>"}
	if _, err = h.js.UpdateStream(h.ctx, info.Config); err != nil {
		h.t.Error(err)
		return
	}
	h.check("rpc_wildcard_collision_reproduced", 5*time.Second, func() (any, error) {
		raw, err := h.rpc("graph.ingest.query.entity", map[string]any{"id": h.ids["run"]})
		if err != nil {
			return nil, err
		}
		var ack struct {
			Stream string `json:"stream"`
			Seq    uint64 `json:"seq"`
		}
		if err = json.Unmarshal(raw, &ack); err != nil {
			return nil, err
		}
		if ack.Stream != "GRAPH" || ack.Seq == 0 {
			return json.RawMessage(raw), errors.New("wildcard did not return conflicting PubAck")
		}
		return json.RawMessage(raw), nil
	})
}

func (h *harness) providerMetrics(stage string) {
	endpoint := strings.TrimSuffix(os.Getenv("SETUP03A_PROVIDER"), "/v1") + "/metrics"
	h.check("provider_metrics_"+stage, 5*time.Second, func() (any, error) {
		ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		h.save("provider-metrics-"+stage+".txt", raw)
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "semembed_requests_total ") {
				count, parseErr := strconv.ParseInt(strings.TrimPrefix(line, "semembed_requests_total "), 10, 64)
				return map[string]any{"endpoint": endpoint, "requests": count}, parseErr
			}
		}
		return string(raw), errors.New("semembed request counter missing")
	})
}

func (h *harness) platformIdentity() (map[string]string, error) {
	kv, err := h.js.KeyValue(h.ctx, "semstreams_config_setup03a_semsource")
	if err != nil {
		return nil, err
	}
	entry, err := kv.Get(h.ctx, "platform_identity")
	if err != nil {
		return nil, err
	}
	var identity map[string]string
	err = json.Unmarshal(entry.Value(), &identity)
	return identity, err
}
func (h *harness) captureRestartGuard() {
	bucket, err := h.js.KeyValue(h.ctx, "GRAPH_INGEST_APPLIED_SEQ")
	if err != nil {
		return
	}
	entry, err := bucket.Get(h.ctx, h.ids["run"]+"/GRAPH")
	if err != nil || len(entry.Value()) != 8 {
		return
	}
	stream, err := h.js.Stream(h.ctx, "GRAPH")
	if err != nil {
		return
	}
	info, err := stream.Info(h.ctx)
	if err != nil {
		return
	}
	var sequences []uint64
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		msg, getErr := stream.GetMsg(h.ctx, seq)
		if getErr == nil && bytes.Contains(msg.Data, []byte(h.ids["run"])) {
			sequences = append(sequences, seq)
		}
	}
	raw, err := json.MarshalIndent(map[string]any{"guard_key": entry.Key(), "applied_sequence": binary.BigEndian.Uint64(entry.Value()), "restarted_stream_last_sequence": info.State.LastSeq, "run_payload_sequences": sequences}, "", "  ")
	if err == nil {
		h.save("restart-guard.json", raw)
	}
}
