//go:build qualification && removal

package setup03a

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const replayTargetProject = "setup03a-corpus-v1"
const replayTargetHandle = "doc-source-" + replayTargetProject

// These wire structs intentionally do not import the implementation under test.
type replayBinding struct {
	Authority    struct{ Org, Platform string } `json:"authority"`
	Namespace    string                         `json:"namespace"`
	Handle       string                         `json:"handle"`
	Generation   uint64                         `json:"generation"`
	BootEpoch    string                         `json:"boot_epoch"`
	Factory      string                         `json:"factory"`
	ConfigDigest string                         `json:"config_digest"`
}
type replayRecord struct {
	Version          int           `json:"version"`
	Binding          replayBinding `json:"binding"`
	Operation        string        `json:"operation"`
	Phase            string        `json:"phase"`
	DesiredCommitted bool          `json:"desired_committed"`
	Seed             *struct {
		BatchID    string          `json:"batch_id"`
		Initial    bool            `json:"initial"`
		Successful bool            `json:"successful"`
		Count      int             `json:"count"`
		Digest     string          `json:"digest"`
		Failure    json.RawMessage `json:"failure"`
	} `json:"seed"`
	Progress struct {
		CompletedCount int `json:"completed_count"`
		Blocker        *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"blocker"`
	} `json:"progress"`
}
type replayQualification struct {
	h          *harness
	platform   string
	targetPath string
	targetIDs  []string
	siblings   map[string]string
	content    map[string]string
}
type replayProcessExit struct {
	PID      int    `json:"pid"`
	Forced   bool   `json:"forced"`
	Observed bool   `json:"observed"`
	ExitCode int    `json:"exit_code"`
	Signal   string `json:"signal,omitempty"`
}

// The frozen harness keeps its historical Stop semantics. This additive wrapper
// proves the old process exited, and rejects a failed or escalated graceful stop.
func replayCheckedStop(h *harness, force bool) (replayProcessExit, error) {
	if h.cmd == nil {
		return replayProcessExit{}, errors.New("no admitted child process to retire")
	}
	cmd := h.cmd
	result := replayProcessExit{PID: cmd.Process.Pid, Forced: force}
	h.stop(force)
	select {
	case <-h.exited:
		result.Observed = true
	default:
		return result, errors.New("child exit was not observed")
	}
	if cmd.ProcessState == nil {
		return result, errors.New("child process state absent")
	}
	result.ExitCode = cmd.ProcessState.ExitCode()
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		result.Signal = status.Signal().String()
	}
	if force {
		if result.Signal != syscall.SIGKILL.String() {
			return result, fmt.Errorf("expected SIGKILL retirement, got %+v", result)
		}
	} else if h.exitErr != nil || result.ExitCode != 0 {
		return result, fmt.Errorf("graceful child retirement failed: %v (exit=%d)", h.exitErr, result.ExitCode)
	}
	return result, nil
}
func (p *replayQualification) stop(name string, force bool) {
	result, err := replayCheckedStop(p.h, force)
	if !p.h.check(name, time.Nanosecond, func() (any, error) { return result, err }) {
		p.h.t.FailNow()
	}
}
func (p *replayQualification) restart(name string, force bool) {
	p.stop(name, force)
	p.h.start()
}

func newReplayQualification(t *testing.T, runtimeAdded bool) *replayQualification {
	t.Helper()
	binary, base := os.Getenv("SETUP03A_BINARY"), os.Getenv("SETUP03A_OUT")
	if binary == "" || base == "" || os.Getenv("SETUP03A_IDENTITY") != "governed" {
		t.Fatal("SETUP03A_BINARY, SETUP03A_OUT and SETUP03A_IDENTITY=governed are required")
	}
	if profile := os.Getenv("SETUP03A_PROFILE"); profile != "" && profile != "bm25" {
		t.Fatal("removal replay process proof requires BM25")
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(base, strings.ReplaceAll(t.Name(), "/", "-"))
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatalf("fresh evidence directory required: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	h := &harness{t: t, ctx: ctx, out: out, binary: binary}
	p := &replayQualification{h: h, siblings: map[string]string{}, content: map[string]string{}}
	t.Cleanup(func() {
		if h.cmd != nil {
			result, err := replayCheckedStop(h, false)
			h.check("cleanup_checked_process_exit", time.Nanosecond, func() (any, error) { return result, err })
		}
		h.finish()
	})
	h.result = report{CorpusVersion: "removal-replay-v1", Profile: "bm25-removal-replay", SemEngineSlices: []int{0, 1}, Started: time.Now().UTC()}
	b, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	h.result.BinarySHA256 = hex.EncodeToString(sum[:])
	h.fixture()
	if err := copyReplayFixture(h.source); err != nil {
		t.Fatal(err)
	}
	p.targetPath = filepath.Join(h.source, "target")
	h.result.CorpusSHA256 = replayTreeHash(t, h.source)
	h.broker()
	h.configure("bm25")
	cfg := h.result.Configuration
	baseSources := cfg["sources"].([]map[string]any)
	sources := []map[string]any{baseSources[0], {"type": "docs", "paths": []string{filepath.Join(h.source, "sibling")}, "project": "removal-sibling", "watch": true}}
	if !runtimeAdded {
		sources = append(sources, p.targetSource())
	}
	cfg["sources"] = sources
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	h.save("semsource.json", raw)
	h.start()
	p.settled("initial_settled", !runtimeAdded)
	h.resolveIdentity()
	if t.Failed() {
		t.FailNow()
	}
	p.platform = strings.Split(h.ids["run"], ".")[1]
	prefix := "setup03a." + p.platform + "." + replayTargetProject + ".web."
	p.targetIDs = []string{prefix + "doc.a-md", prefix + "chunk.a-md-0000", prefix + "doc.b-md", prefix + "chunk.b-md-0000"}
	sort.Strings(p.targetIDs)
	if runtimeAdded {
		p.add("runtime_add_receipt")
		p.settled("runtime_add_deferred", false)
		p.restart("runtime_add_checked_exit", false)
		p.settled("runtime_add_admitted", true)
	}
	p.markers("initial_target_current", p.targetIDs, "")
	p.captureSiblings()
	return p
}
func (p *replayQualification) targetSource() map[string]any {
	return map[string]any{"type": "docs", "paths": []string{p.targetPath}, "project": replayTargetProject, "watch": true}
}
func copyReplayFixture(dest string) error {
	return filepath.WalkDir("fixtures/removal-replay-v1", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("fixtures/removal-replay-v1", path)
		if err != nil {
			return err
		}
		if rel == "manifest.json" {
			rel = "removal-replay-manifest.json"
		}
		to := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, 0644)
	})
}
func replayTreeHash(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = hash.Write([]byte(rel))
		_, _ = hash.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
func (p *replayQualification) settled(name string, target bool) {
	if !p.h.check(name, 45*time.Second, func() (any, error) {
		status, raw, err := p.h.removalStatus()
		if err != nil {
			return raw, err
		}
		want := 2
		if target {
			want = 3
		}
		present := false
		for _, s := range status.Sources {
			if s.Instance == replayTargetHandle {
				present = true
			}
			if s.Offered != s.Delivered || s.Lost != 0 || s.SeedLost != 0 {
				return raw, errors.New("unsettled or lossy source")
			}
		}
		if status.Phase != "ready" || len(status.Sources) != want || present != target {
			return raw, fmt.Errorf("unexpected boot admission: target=%t sources=%d phase=%s", present, len(status.Sources), status.Phase)
		}
		return raw, nil
	}) {
		p.h.t.FailNow()
	}
}
func (p *replayQualification) add(name string) {
	raw, err := p.h.rpc("graph.ingest.add.setup03a", map[string]any{"source": p.targetSource(), "provenance": map[string]string{"actor": "removal215-qualification"}})
	p.h.save(name+".json", raw)
	if !p.h.check(name, time.Nanosecond, func() (any, error) {
		if err != nil {
			return json.RawMessage(raw), err
		}
		var response struct {
			DesiredChanged  bool `json:"desired_changed"`
			RuntimeChanged  bool `json:"runtime_changed"`
			RestartRequired bool `json:"restart_required"`
			Components      []struct {
				Instance string `json:"instance_name"`
				Created  bool   `json:"created"`
			} `json:"components"`
		}
		if decodeErr := json.Unmarshal(raw, &response); decodeErr != nil {
			return json.RawMessage(raw), decodeErr
		}
		if !response.DesiredChanged || response.RuntimeChanged || !response.RestartRequired || len(response.Components) != 1 || response.Components[0].Instance != replayTargetHandle || !response.Components[0].Created {
			return json.RawMessage(raw), errors.New("add receipt must commit desired state for same handle only")
		}
		return json.RawMessage(raw), nil
	}) {
		p.h.t.FailNow()
	}
}
func (p *replayQualification) remove() replayRecord {
	raw, err := p.h.rpc("graph.ingest.remove.setup03a", map[string]any{"instance_name": replayTargetHandle, "provenance": map[string]string{"actor": "removal215-qualification"}})
	p.h.save("remove-receipt.json", raw)
	if !p.h.check("remove_receipt", time.Nanosecond, func() (any, error) {
		if err != nil {
			return json.RawMessage(raw), err
		}
		var response struct {
			Removed         bool `json:"removed"`
			DesiredChanged  bool `json:"desired_changed"`
			RuntimeChanged  bool `json:"runtime_changed"`
			RestartRequired bool `json:"restart_required"`
		}
		if e := json.Unmarshal(raw, &response); e != nil {
			return json.RawMessage(raw), e
		}
		if !response.Removed || !response.DesiredChanged || response.RuntimeChanged || !response.RestartRequired {
			return json.RawMessage(raw), errors.New("remove receipt must commit desired state only")
		}
		return json.RawMessage(raw), nil
	}) {
		p.h.t.FailNow()
	}
	p.settled("old_boot_producer_still_admitted", true)
	record := p.awaitRecord("durable_pending_before_projection", "remove", "pending", 0, 8*time.Second)
	p.markers("old_boot_targets_not_marked", p.targetIDs, "")
	return record
}
func (p *replayQualification) record() (replayRecord, map[string]any, error) {
	ctx, cancel := context.WithTimeout(p.h.ctx, 2*time.Second)
	defer cancel()
	kv, err := p.h.js.KeyValue(ctx, "SEMSOURCE_SOURCE_LIFECYCLE")
	if err != nil {
		return replayRecord{}, nil, err
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		return replayRecord{}, nil, err
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "source.") {
			continue
		}
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return replayRecord{}, nil, err
		}
		var record replayRecord
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			return record, nil, err
		}
		if record.Binding.Handle != replayTargetHandle {
			continue
		}
		detail := map[string]any{"key": key, "kv_revision": entry.Revision(), "record": json.RawMessage(entry.Value())}
		if record.Version != 1 || record.Binding.Namespace != "setup03a" || record.Binding.Authority.Org != "setup03a" || record.Binding.Authority.Platform != p.platform || record.Binding.Generation == 0 {
			return record, detail, errors.New("invalid journal ownership/schema/generation")
		}
		return record, detail, nil
	}
	return replayRecord{}, nil, errors.New("exact source journal record absent")
}
func (p *replayQualification) awaitRecord(name, operation, phase string, minGeneration uint64, timeout time.Duration) replayRecord {
	var record replayRecord
	if !p.h.check(name, timeout, func() (any, error) {
		r, detail, err := p.record()
		record = r
		if err != nil {
			return detail, err
		}
		if r.Operation != operation || r.Phase != phase || r.Binding.Generation <= minGeneration || !r.DesiredCommitted {
			return detail, fmt.Errorf("journal not %s/%s after generation %d", operation, phase, minGeneration)
		}
		return detail, nil
	}) {
		p.h.t.FailNow()
	}
	return record
}

// markers keeps the original prerequisite behavior. Deliberately blocked
// positive acceptance uses observeMarkers so independent evidence is not lost.
func (p *replayQualification) markers(name string, ids []string, want string) {
	if !p.observeMarkers(name, ids, want) {
		p.h.t.FailNow()
	}
}
func (p *replayQualification) observeMarkers(name string, ids []string, want string) bool {
	return p.h.check(name, 25*time.Second, func() (any, error) {
		entities := map[string]exact{}
		var failures error
		for _, id := range ids {
			e, err := p.h.entity(id)
			entities[id] = e
			if err != nil {
				failures = errors.Join(failures, fmt.Errorf("read %s: %w", id, err))
				continue
			}
			marker := value(e, "entity.lifecycle.stale")
			if (want == "" && marker != nil) || (want != "" && marker != want) {
				failures = errors.Join(failures, fmt.Errorf("entity %s marker=%v want %q", id, marker, want))
			}
		}
		return entities, failures
	})
}
func (p *replayQualification) prefixIDs(prefix string) ([]string, error) {
	cursor := ""
	seen := map[string]bool{}
	ids := map[string]bool{}
	for pages := 0; pages < 100; pages++ {
		raw, err := p.h.rpc("graph.query.prefix", map[string]any{"prefix": prefix, "limit": 2, "cursor": cursor})
		if err != nil {
			return nil, err
		}
		var page struct {
			Entities []struct {
				ID string `json:"id"`
			} `json:"entities"`
			Next string `json:"next_cursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		for _, e := range page.Entities {
			if !strings.HasPrefix(e.ID, prefix+".") {
				return nil, fmt.Errorf("out-of-scope entity %s", e.ID)
			}
			if ids[e.ID] {
				return nil, fmt.Errorf("duplicate entity across pages %s", e.ID)
			}
			ids[e.ID] = true
		}
		if page.Next == "" {
			out := make([]string, 0, len(ids))
			for id := range ids {
				out = append(out, id)
			}
			sort.Strings(out)
			return out, nil
		}
		if seen[page.Next] {
			return nil, errors.New("repeated opaque cursor")
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return nil, errors.New("prefix traversal exceeded finite page bound")
}
func replaySourceFacts(e exact) string {
	rows := []string{}
	for _, tr := range e.Entity.Triples {
		b, _ := json.Marshal(tr)
		rows = append(rows, string(b))
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// replayASTReseed records the one reviewed observation-time difference. The
// preserved before-binary plain-restart control reproduces it without lifecycle
// operations: source/ast CodeEntity.Triples derives dc.terms.created from IndexedAt.
type replayASTReseed struct {
	Before string `json:"before_created"`
	After  string `json:"after_created"`
}

// This fixture admits one known AST producer under this authority and system.
// A document or another producer never inherits the timestamp exception.
func (p *replayQualification) isASTSibling(id string) bool {
	parts := strings.Split(id, ".")
	if len(parts) != 6 || parts[0] != "setup03a" || parts[1] != p.platform || parts[2] != replayTargetProject {
		return false
	}
	switch parts[3] {
	case "code", "golang", "typescript", "javascript", "java", "python", "svelte", "c", "cpp":
		return true
	}
	return false
}
func replayASTFacts(raw string) (string, string, error) {
	rows := []string{}
	created := ""
	for _, row := range strings.Split(raw, "\n") {
		var triple struct {
			Predicate string `json:"predicate"`
			Object    any    `json:"object"`
		}
		if err := json.Unmarshal([]byte(row), &triple); err != nil {
			return "", "", err
		}
		if triple.Predicate != "dc.terms.created" {
			rows = append(rows, row)
			continue
		}
		timestamp, ok := triple.Object.(string)
		if !ok || created != "" {
			return "", "", errors.New("AST ingestion timestamp must be one string fact")
		}
		if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
			return "", "", err
		}
		created = timestamp
	}
	if created == "" {
		return "", "", errors.New("AST ingestion timestamp disappeared")
	}
	return strings.Join(rows, "\n"), created, nil
}
func replayCompareSibling(id, want string, got exact, knownAST bool) (*replayASTReseed, error) {
	if got.Entity == nil || got.Entity.ID != id {
		return nil, errors.New("sibling entity ID changed")
	}
	actual := replaySourceFacts(got)
	var metadata *replayASTReseed
	if knownAST {
		before, oldCreated, err := replayASTFacts(want)
		if err != nil {
			return nil, err
		}
		after, newCreated, err := replayASTFacts(actual)
		if err != nil {
			return nil, err
		}
		want, actual = before, after
		metadata = &replayASTReseed{Before: oldCreated, After: newCreated}
	}
	if actual != want {
		return metadata, fmt.Errorf("sibling source facts changed: %s", id)
	}
	return metadata, nil
}
func replayCompareContent(key, want string, got []byte) error {
	if string(got) != want {
		return fmt.Errorf("sibling exact content changed: %s", key)
	}
	return nil
}
func (p *replayQualification) captureSiblings() {
	prefix := "setup03a." + p.platform + "."
	if !p.h.check("sibling_source_facts_captured", 25*time.Second, func() (any, error) {
		ids, err := p.prefixIDs(prefix + replayTargetProject)
		if err != nil {
			return ids, err
		}
		other, err := p.prefixIDs(prefix + "removal-sibling")
		if err != nil {
			return other, err
		}
		ids = append(ids, other...)
		store, err := p.h.js.ObjectStore(p.h.ctx, "CONTENT")
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if strings.Contains(id, "."+replayTargetProject+".web.") {
				continue
			}
			e, err := p.h.entity(id)
			if err != nil {
				return e, err
			}
			if value(e, "entity.lifecycle.stale") != nil {
				return e, errors.New("sibling initially stale")
			}
			p.siblings[id] = replaySourceFacts(e)
			for _, predicate := range []string{"code.body.key", "source.doc.body-key"} {
				if key, ok := value(e, predicate).(string); ok {
					b, err := store.GetBytes(p.h.ctx, key)
					if err != nil {
						return e, err
					}
					p.content[key] = string(b)
				}
			}
		}
		if len(p.siblings) < 6 || len(p.content) < 2 {
			return p.siblings, errors.New("insufficient same-system AST and independent document siblings")
		}
		return map[string]any{"ids": p.siblings, "content_keys": p.content}, nil
	}) {
		p.h.t.FailNow()
	}
}
func (p *replayQualification) unchangedSiblings(name string) {
	if !p.h.check(name, 20*time.Second, func() (any, error) {
		store, err := p.h.js.ObjectStore(p.h.ctx, "CONTENT")
		if err != nil {
			return nil, err
		}
		metadata := map[string]replayASTReseed{}
		for id, want := range p.siblings {
			e, err := p.h.entity(id)
			if err != nil {
				return e, err
			}
			reseed, err := replayCompareSibling(id, want, e, p.isASTSibling(id))
			if reseed != nil {
				metadata[id] = *reseed
			}
			if err != nil {
				return map[string]any{"entity": e, "ast_reseed_metadata": metadata}, err
			}
		}
		for key, want := range p.content {
			b, err := store.GetBytes(p.h.ctx, key)
			if err != nil {
				return nil, err
			}
			if err := replayCompareContent(key, want, b); err != nil {
				return key, err
			}
		}
		identity, err := p.h.platformIdentity()
		if err != nil {
			return nil, err
		}
		if identity["id"] != p.platform || identity["org"] != "setup03a" || identity["stem"] != "semsource" {
			return identity, errors.New("persisted authority changed")
		}
		raw, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return nil, err
		}
		p.h.save(name+"-ast-reseed-metadata.json", raw)
		return map[string]any{"unchanged_entities": len(p.siblings), "unchanged_bodies": len(p.content), "ast_reseed_metadata": metadata}, nil
	}) {
		p.h.t.FailNow()
	}
}

// Keep an authoritative watcher alive across child exits. Before it stops,
// every current target revision must have reached the observation channel.
func (p *replayQualification) watchTargetStates() (func() ([]json.RawMessage, error), error) {
	ctx, cancel := context.WithTimeout(p.h.ctx, 3*time.Minute)
	names := p.h.js.KeyValueStoreNames(ctx)
	var kv jetstream.KeyValue
	for name := range names.Name() {
		if !strings.HasSuffix(name, "ENTITY_STATES") {
			continue
		}
		candidate, err := p.h.js.KeyValue(ctx, name)
		if err != nil {
			cancel()
			return nil, err
		}
		if _, err = candidate.Get(ctx, p.targetIDs[0]); err == nil {
			kv = candidate
			break
		}
	}
	if kv == nil {
		cancel()
		return nil, errors.New("retained authority bucket not found")
	}
	watcher, err := kv.Watch(ctx, "setup03a."+p.platform+"."+replayTargetProject+".web.>")
	if err != nil {
		cancel()
		return nil, err
	}
	ready, done, changed := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	var mu sync.Mutex
	var values []json.RawMessage
	seen := map[string]uint64{}
	go func() {
		defer close(done)
		initialized := false
		for entry := range watcher.Updates() {
			if entry == nil {
				if !initialized {
					close(ready)
					initialized = true
				}
				continue
			}
			mu.Lock()
			values = append(values, append(json.RawMessage(nil), entry.Value()...))
			seen[entry.Key()] = entry.Revision()
			mu.Unlock()
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		_ = watcher.Stop()
		<-done
		cancel()
		return nil, ctx.Err()
	}
	return func() ([]json.RawMessage, error) {
		defer cancel()
		var barrierErr error
		want := map[string]uint64{}
		for _, id := range p.targetIDs {
			entry, e := kv.Get(ctx, id)
			if e != nil {
				barrierErr = e
				break
			}
			want[id] = entry.Revision()
		}
		for barrierErr == nil {
			complete := true
			mu.Lock()
			for id, rev := range want {
				if seen[id] < rev {
					complete = false
				}
			}
			mu.Unlock()
			if complete {
				break
			}
			select {
			case <-changed:
			case <-ctx.Done():
				barrierErr = ctx.Err()
			case <-done:
				barrierErr = errors.New("authority watcher ended before revision barrier")
			}
		}
		stopErr := watcher.Stop()
		<-done
		return values, errors.Join(barrierErr, stopErr)
	}, nil
}

func (p *replayQualification) retainedPass(generation uint64) replayRecord {
	var record replayRecord
	if !p.h.check("retained_markers_converged_but_terminal_completion_unqualified", 45*time.Second, func() (any, error) {
		r, detail, err := p.record()
		record = r
		if err != nil {
			return detail, err
		}
		if r.Operation != "remove" || r.Phase != "pending" || r.Binding.Generation != generation || r.Progress.Blocker == nil || r.Progress.Blocker.Code != "applied_tail_unproven" {
			return detail, errors.New("retained pass must remain pending/applied_tail_unproven at the same generation")
		}
		if r.Progress.CompletedCount < len(p.targetIDs) {
			return detail, errors.New("retained projection has not recorded the complete expected set")
		}
		return detail, nil
	}) {
		p.h.t.FailNow()
	}
	p.markers("all_retained_target_entities_source_removed", p.targetIDs, "source_removed")
	gate := map[string]any{"gate": "automatic_terminal_removal_completion", "qualified": false, "status": "blocked", "blocker": "applied_tail_unproven", "upstream": "https://github.com/C360Studio/semstreams/issues/1444", "retained_pass_is_not_terminal_completion": true}
	raw, err := json.MarshalIndent(gate, "", "  ")
	if err != nil {
		p.h.t.Fatal(err)
	}
	p.h.save("qualification-gates.json", raw)
	return record
}

type replayReceipt struct {
	Binding      replayBinding `json:"binding"`
	BatchID      string        `json:"batch_id"`
	EntityID     string        `json:"entity_id"`
	Fingerprint  string        `json:"fingerprint"`
	Acknowledged bool          `json:"acknowledged"`
}
type replayManifestEntry struct {
	EntityID    string `json:"entity_id"`
	Fingerprint string `json:"fingerprint"`
}

// The receipt value identifies the current binding; its observed key supplies
// the opaque prefix verbatim. The harness never implements the token codec.
func (p *replayQualification) currentSeedProof(record replayRecord, expected []string) (any, error) {
	if record.Binding.BootEpoch == "" || record.Seed == nil || !record.Seed.Initial || !record.Seed.Successful || record.Seed.BatchID == "" || record.Seed.Digest == "" || record.Seed.Count != len(expected) {
		return record, errors.New("current epoch lacks a successful exact-count initial seed seal")
	}
	if len(record.Seed.Failure) > 0 && string(record.Seed.Failure) != "null" {
		return record, errors.New("seed seal records failure")
	}
	ctx, cancel := context.WithTimeout(p.h.ctx, 3*time.Second)
	defer cancel()
	kv, err := p.h.js.KeyValue(ctx, "SEMSOURCE_SOURCE_LIFECYCLE")
	if err != nil {
		return nil, err
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		return nil, err
	}
	receipts := map[string]replayReceipt{}
	prefix := ""
	for _, key := range keys {
		if !strings.HasPrefix(key, "receipt.") {
			continue
		}
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		var receipt replayReceipt
		if err := json.Unmarshal(entry.Value(), &receipt); err != nil {
			return nil, err
		}
		if receipt.Binding != record.Binding || receipt.BatchID != record.Seed.BatchID {
			continue
		}
		if !receipt.Acknowledged || receipt.Fingerprint == "" {
			return receipt, errors.New("current receipt lacks acknowledged immutable fingerprint")
		}
		if _, duplicate := receipts[receipt.EntityID]; duplicate {
			return receipt, errors.New("duplicate current receipt ID")
		}
		receipts[receipt.EntityID] = receipt
		currentPrefix := key[:strings.LastIndex(key, ".")+1]
		if prefix != "" && prefix != currentPrefix {
			return receipts, errors.New("current receipts disagree on opaque binding prefix")
		}
		prefix = currentPrefix
	}
	if len(receipts) != len(expected) {
		return receipts, errors.New("current receipts differ from exact expected ID set")
	}
	manifestPrefix := strings.Replace(prefix, "receipt.", "manifest.", 1)
	entries := []replayManifestEntry{}
	for _, key := range keys {
		if !strings.HasPrefix(key, manifestPrefix) {
			continue
		}
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		var item replayManifestEntry
		if err := json.Unmarshal(entry.Value(), &item); err != nil {
			return nil, err
		}
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EntityID < entries[j].EntityID })
	want := append([]string(nil), expected...)
	sort.Strings(want)
	if len(entries) != len(want) {
		return entries, errors.New("current manifest differs from exact expected ID set")
	}
	for i, item := range entries {
		receipt, ok := receipts[item.EntityID]
		if item.EntityID != want[i] || !ok || item.Fingerprint != receipt.Fingerprint {
			return entries, errors.New("manifest ID/fingerprint mismatch")
		}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != record.Seed.Digest {
		return entries, errors.New("manifest checksum does not match journal terminal seal")
	}
	return map[string]any{"binding": record.Binding, "seal": record.Seed, "entries": entries, "receipts": receipts}, nil
}
func replayReactivationEpoch(record, before replayRecord) error {
	if record.Operation != "reactivate" || record.Binding.Generation != before.Binding.Generation || record.Binding.BootEpoch == "" || record.Binding.BootEpoch == before.Binding.BootEpoch {
		return errors.New("reactivation requires the same generation with a new current boot epoch and independent proof")
	}
	return nil
}
func replayReactivationComplete(record, before replayRecord) error {
	if err := replayReactivationEpoch(record, before); err != nil {
		return err
	}
	if record.Phase != "complete" {
		return errors.New("reactivation has not completed; pending upstream work does not satisfy positive acceptance")
	}
	return nil
}

// reactivated records publication proof separately from graph completion. The
// positive acceptance remains strict and failing; its failure must not hide the
// independent offline-deletion, sibling-isolation, and checked-exit evidence.
func (p *replayQualification) reactivated(name string, before replayRecord, expected []string) replayRecord {
	var result replayRecord
	p.h.check(name+"_current_epoch_publication_proof", 45*time.Second, func() (any, error) {
		record, detail, err := p.record()
		result = record
		if err != nil {
			return detail, err
		}
		if err := replayReactivationEpoch(record, before); err != nil {
			return detail, err
		}
		proof, err := p.currentSeedProof(record, expected)
		if err == nil {
			raw, marshalErr := json.MarshalIndent(proof, "", "  ")
			if marshalErr != nil {
				return proof, marshalErr
			}
			p.h.save(name+"-publication-proof.json", raw)
		}
		return proof, err
	})
	p.h.check(name+"_projection_outcome_observed", 45*time.Second, func() (any, error) {
		record, detail, err := p.record()
		result = record
		if err != nil {
			return detail, err
		}
		if err := replayReactivationEpoch(record, before); err != nil {
			return detail, err
		}
		if record.Phase == "complete" || (record.Phase == "pending" && record.Progress.Blocker != nil && record.Progress.Blocker.Code == "conditional_reconcile_unavailable") {
			return detail, nil
		}
		return detail, errors.New("projection has neither completed nor exposed the recognized conditional-reconcile blocker")
	})
	p.h.check(name, time.Nanosecond, func() (any, error) {
		record, detail, err := p.record()
		result = record
		if err != nil {
			return detail, err
		}
		if err := replayReactivationComplete(record, before); err != nil {
			return detail, err
		}
		return p.currentSeedProof(record, expected)
	})
	if result.Phase == "pending" && result.Progress.Blocker != nil && result.Progress.Blocker.Code == "conditional_reconcile_unavailable" {
		p.h.check(name+"_honest_pending_conditional_reconcile", time.Nanosecond, func() (any, error) {
			record, detail, err := p.record()
			if err != nil {
				return detail, err
			}
			if err := replayReactivationEpoch(record, before); err != nil {
				return detail, err
			}
			if record.Phase != "pending" || record.Progress.Blocker == nil || record.Progress.Blocker.Code != "conditional_reconcile_unavailable" {
				return detail, errors.New("conditional reconcile blocker was not retained honestly")
			}
			return detail, nil
		})
		p.observeMarkers(name+"_no_unfenced_current_marker_clear", expected, "source_removed")
	}
	gate := map[string]any{"gate": "conditional_source_removed_reactivation", "qualified": false, "status": "blocked", "blocker": "conditional_reconcile_unavailable", "upstream": "https://github.com/C360Studio/semstreams/issues/1445", "positive_completion_and_freshness_checks_remain_required": true, "observed_record": result}
	raw, err := json.MarshalIndent(gate, "", "  ")
	if err != nil {
		p.h.t.Error(err)
	} else {
		p.h.save("reactivation-qualification-gates.json", raw)
	}
	return result
}
func (p *replayQualification) legacySweep() {
	raw, err := p.h.rpc("graph.lifecycle.run", map[string]any{"org": "setup03a", "systems": []string{replayTargetProject}, "root_path": p.h.source, "reason": "path_missing"})
	if !p.h.check("legacy_sweep_completed", time.Nanosecond, func() (any, error) { return json.RawMessage(raw), err }) {
		p.h.t.FailNow()
	}
	p.markers("legacy_sweep_preserves_source_removed", p.targetIDs, "source_removed")
}
