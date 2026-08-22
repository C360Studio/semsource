//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/cutover"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	upgradeKnownPredicate   = "source.doc.file-path"
	upgradeRetiredPredicate = "source.doc.file_path"
	upgradeKnownObject      = "cutover.md"
	upgradeKnownContent     = "# Beta 148 Cutover\n\ncanonical known answer\n"
	upgradeObjectBucket     = "PRESERVED_OBJECTS"
	upgradeObjectKey        = "cutover-sentinel.txt"
	upgradeSentinelKey      = "cutover.sentinel"
	upgradeUnrelatedStream  = "UNRELATED_EVENTS"
	upgradeUnrelatedSubject = "unrelated.cutover.sentinel"
	upgradeUnrelatedMessage = "preserve-unrelated-stream-upgrade"
)

var upgradePreservedKVBuckets = []string{
	"AUTHORITATIVE_INPUTS",
	"SOURCE_STORE",
	"CONTENT_STORE",
	"MEDIA_STORE",
	"UNRELATED_STATE",
}

// upgradeRetainedKVBuckets is everything that must survive a cutover: the
// user-owned buckets the framework never claimed, plus the framework-owned
// buckets reviewed as non-rebuildable in internal/cutover.
func upgradeRetainedKVBuckets() []string {
	return append(append([]string{}, upgradePreservedKVBuckets...), cutover.Retained...)
}

// upgradeAssertPurged checks the other half of the classification: a bucket
// reviewed as rebuildable must actually be gone. This is what catches a purge
// loop that silently skips entries — the failure mode where a cutover leaves
// stale derived state behind and the next seed merges into it.
func upgradeAssertPurged(t *testing.T, ctx context.Context, js jetstream.JetStream) {
	t.Helper()
	for _, bucket := range cutover.Purged {
		if _, err := js.KeyValue(ctx, bucket); err == nil {
			t.Errorf("purged KV %s still exists after cutover", bucket)
		}
	}
}

type upgradeInventory struct {
	Streams []string
	KV      []string
	Objects []string
}

func TestE2E_UpgradePathRehearsal(t *testing.T) {
	natsURL, cleanupNATS := startNATS(t)
	defer cleanupNATS()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	sentinelValue := "preserve-upgrade"
	// Retained framework buckets are seeded because nothing in SemSource writes
	// them — TOOL_CALL_OUTCOMES belongs to semstreams' agentic-tools processor,
	// so without a sentinel its survival is unobservable.
	//
	// Seeding the PURGED buckets here is not possible: they are catalog buckets
	// the framework creates with its own configuration, and pre-creating them
	// with default KV settings breaks graph ingest outright (measured: the
	// known-answer query returns nothing). Their disposition is asserted
	// instead by upgradeAssertPurged, which needs no seeding because SemSource
	// creates them itself during the first writer run.
	for _, bucket := range upgradeRetainedKVBuckets() {
		upgradeCreateSentinelKV(t, ctx, js, bucket, sentinelValue)
	}
	upgradeCreateSentinelKV(t, ctx, js, "PREDICATE_CATALOG", "legacy-beta145")

	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     upgradeUnrelatedStream,
		Subjects: []string{upgradeUnrelatedSubject},
		Storage:  jetstream.MemoryStorage,
	}); err != nil {
		t.Fatalf("create unrelated sentinel stream: %v", err)
	}
	if _, err := js.Publish(ctx, upgradeUnrelatedSubject, []byte(upgradeUnrelatedMessage)); err != nil {
		t.Fatalf("publish unrelated stream sentinel: %v", err)
	}

	// This object store is authoritative test data and intentionally distinct
	// from SemSource's production CONTENT body store.
	objects, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: upgradeObjectBucket})
	if err != nil {
		t.Fatalf("create preserved object store: %v", err)
	}
	if _, err := objects.PutString(ctx, upgradeObjectKey, sentinelValue); err != nil {
		t.Fatalf("put object sentinel: %v", err)
	}

	binPath := buildBinary(t)
	workDir := t.TempDir()
	docsDir := filepath.Join(workDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("create docs fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "cutover.md"), []byte(upgradeKnownContent), 0o644); err != nil {
		t.Fatalf("write docs fixture: %v", err)
	}
	httpPort := freePort(t)
	wsPort := freePort(t)
	configPath := upgradeWriteDocsConfig(t, workDir, docsDir, httpPort)

	stopFirst := upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer stopFirst()
	upgradeWaitForReady(t, httpPort, 90*time.Second)
	upgradeAssertKnownAnswer(t, nc, 45*time.Second)

	// beta.160 removed the COMPONENT_STATUS diagnostic bucket, so no
	// system-created operational bucket exists to widen the preservation set
	// with; the operator-owned sentinel buckets above carry the preservation
	// proof alone.

	// Stop every graph writer before capturing the literal deletion sheet.
	stopFirst()
	inventory := upgradeCaptureInventory(t, ctx, js)
	t.Logf("pre-cutover streams=%v", inventory.Streams)
	t.Logf("pre-cutover kv=%v", inventory.KV)
	t.Logf("pre-cutover object_stores=%v", inventory.Objects)

	streamSet := upgradeStringSet(inventory.Streams)
	kvSet := upgradeStringSet(inventory.KV)
	objectSet := upgradeStringSet(inventory.Objects)
	if !streamSet["GRAPH"] || !kvSet["semstreams_config"] || !kvSet["PREDICATE_CATALOG"] {
		t.Fatalf("required observed cutover resources missing: GRAPH=%t semstreams_config=%t PREDICATE_CATALOG=%t",
			streamSet["GRAPH"], kvSet["semstreams_config"], kvSet["PREDICATE_CATALOG"])
	}
	if !objectSet[upgradeObjectBucket] {
		t.Fatalf("preservation inventory missing object bucket %s", upgradeObjectBucket)
	}
	if !streamSet[upgradeUnrelatedStream] {
		t.Fatalf("preservation inventory missing unrelated stream %s", upgradeUnrelatedStream)
	}

	// Execute only the reviewed literal inventory intersection. No wildcard,
	// inferred resource, or unrelated stream/bucket can enter this deletion.
	if streamSet["GRAPH"] {
		if err := js.DeleteStream(ctx, "GRAPH"); err != nil {
			t.Fatalf("delete observed GRAPH stream: %v", err)
		}
	}
	for _, bucket := range []string{"semstreams_config"} {
		if kvSet[bucket] {
			if err := js.DeleteKeyValue(ctx, bucket); err != nil {
				t.Fatalf("delete observed KV %s: %v", bucket, err)
			}
		}
	}
	for _, bucket := range cutover.Purged {
		if kvSet[bucket] {
			if err := js.DeleteKeyValue(ctx, bucket); err != nil {
				t.Fatalf("delete observed framework KV %s: %v", bucket, err)
			}
		}
	}
	// Only legacy resources outside the framework catalog belong here. kvSet is
	// the PRE-deletion snapshot, so anything already removed by the catalog pass
	// above would be deleted twice and fail; ENTITY_SUFFIX_INDEX and
	// GRAPH_INGEST_APPLIED_SEQ left this list when they entered the catalog.
	for _, bucket := range []string{"PREDICATE_CATALOG"} {
		if kvSet[bucket] {
			if err := js.DeleteKeyValue(ctx, bucket); err != nil {
				t.Fatalf("delete observed migration KV %s: %v", bucket, err)
			}
		}
	}

	upgradeAssertPreserved(t, ctx, js, sentinelValue)
	upgradeAssertPurged(t, ctx, js)
	upgradeAssertUnrelatedStream(t, ctx, js)
	if _, err := js.KeyValue(ctx, "PREDICATE_CATALOG"); err == nil {
		t.Fatal("legacy PREDICATE_CATALOG still exists after cutover")
	}

	stopSecond := upgradeStartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer stopSecond()
	upgradeWaitForReady(t, httpPort, 90*time.Second)
	upgradeAssertKnownAnswer(t, nc, 45*time.Second)
	stopSecond()

	upgradeAssertPreserved(t, ctx, js, sentinelValue)
	upgradeAssertUnrelatedStream(t, ctx, js)
	if _, err := js.KeyValue(ctx, "PREDICATE_CATALOG"); err == nil {
		t.Fatal("migrated restart recreated legacy PREDICATE_CATALOG")
	}
}

func upgradeWriteDocsConfig(t *testing.T, workDir, docsDir string, httpPort int) string {
	t.Helper()
	cfg := map[string]any{
		"namespace": "upgradecutover",
		"http_port": httpPort,
		// beta.160 metric servers bind synchronously and fail loudly on a
		// collision; the fixed 9091 default cannot be shared across tests or
		// with a developer\'s local stack.
		"metrics": map[string]any{"port": freePort(t)},
		"sources": []map[string]any{{
			"type":  "docs",
			"paths": []string{docsDir},
			"watch": false,
		}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal reviewed config: %v", err)
	}
	path := filepath.Join(workDir, "semsource.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write reviewed config: %v", err)
	}
	return path
}

func upgradeStartWriter(
	t *testing.T,
	binPath, configPath, workDir, natsURL string,
	httpPort, wsPort int,
) func() {
	t.Helper()
	cmd := exec.Command(binPath, "run",
		"--config", configPath,
		"--log-level", "info",
		"--nats-url", natsURL,
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SEMSOURCE_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMSOURCE_WS_BIND=127.0.0.1:%d", wsPort),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("writer stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("writer stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start migrated writer: %v", err)
	}
	logPipe(t, "upgrade stdout", stdout)
	logPipe(t, "upgrade stderr", stderr)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				if err != nil {
					t.Logf("migrated writer exit: %v", err)
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Errorf("migrated writer did not stop gracefully within 15s")
			}
		})
	}
}

func upgradeWaitForReady(t *testing.T, httpPort int, timeout time.Duration) {
	t.Helper()
	status := waitForReady(t, httpPort, timeout)
	if status.Phase != "ready" {
		t.Fatalf("SemSource phase = %q, want ready; status=%+v", status.Phase, status)
	}
}

func upgradeAssertKnownAnswer(t *testing.T, nc *nats.Conn, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		request, _ := json.Marshal(map[string]string{"predicate": upgradeKnownPredicate})
		response, err := nc.Request("graph.index.query.predicate", request, 5*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var predicateResponse semgraph.PredicateQueryResponse
		if err := json.Unmarshal(response.Data, &predicateResponse); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, id := range predicateResponse.Data.Entities {
			entityRequest, _ := json.Marshal(map[string]string{"id": id})
			entityResponse, err := nc.Request("graph.ingest.query.entity", entityRequest, 5*time.Second)
			if err != nil {
				lastErr = err
				continue
			}
			// beta.160 exact reads return graph.ExactEntity: the entity
			// wrapped with its authoritative KV revision.
			var exact semgraph.ExactEntity
			if err := json.Unmarshal(entityResponse.Data, &exact); err != nil || exact.Entity == nil {
				if err == nil {
					err = fmt.Errorf("exact read for %s carried no entity", id)
				}
				lastErr = err
				continue
			}
			entity := *exact.Entity
			for _, triple := range entity.Triples {
				if triple.Predicate == upgradeRetiredPredicate {
					t.Fatalf("known-answer entity %s contains retired predicate %s", entity.ID, upgradeRetiredPredicate)
				}
				if triple.Predicate == upgradeKnownPredicate && triple.Object == upgradeKnownObject {
					t.Logf("canonical known answer entity=%s predicate=%s", entity.ID, triple.Predicate)
					return
				}
			}
		}
		lastErr = fmt.Errorf("predicate query returned %d entities without exact known answer", len(predicateResponse.Data.Entities))
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("canonical known-answer query failed within %s: %v", timeout, lastErr)
}

func upgradeCreateSentinelKV(
	t *testing.T,
	ctx context.Context,
	js jetstream.JetStream,
	bucket, value string,
) {
	t.Helper()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: bucket})
	if err != nil {
		t.Fatalf("create sentinel KV %s: %v", bucket, err)
	}
	if _, err := kv.PutString(ctx, upgradeSentinelKey, value); err != nil {
		t.Fatalf("put sentinel KV %s: %v", bucket, err)
	}
}

func upgradeCaptureInventory(t *testing.T, ctx context.Context, js jetstream.JetStream) upgradeInventory {
	t.Helper()
	var inventory upgradeInventory
	streams := js.StreamNames(ctx)
	for name := range streams.Name() {
		inventory.Streams = append(inventory.Streams, name)
	}
	if err := streams.Err(); err != nil {
		t.Fatalf("list streams: %v", err)
	}
	kvs := js.KeyValueStoreNames(ctx)
	for name := range kvs.Name() {
		inventory.KV = append(inventory.KV, name)
	}
	if err := kvs.Error(); err != nil {
		t.Fatalf("list KV buckets: %v", err)
	}
	objects := js.ObjectStoreNames(ctx)
	for name := range objects.Name() {
		inventory.Objects = append(inventory.Objects, name)
	}
	if err := objects.Error(); err != nil {
		t.Fatalf("list object stores: %v", err)
	}
	sort.Strings(inventory.Streams)
	sort.Strings(inventory.KV)
	sort.Strings(inventory.Objects)
	return inventory
}

func upgradeAssertPreserved(t *testing.T, ctx context.Context, js jetstream.JetStream, want string) {
	t.Helper()
	for _, bucket := range upgradeRetainedKVBuckets() {
		kv, err := js.KeyValue(ctx, bucket)
		if err != nil {
			t.Errorf("preserved KV %s missing: %v", bucket, err)
			continue
		}
		entry, err := kv.Get(ctx, upgradeSentinelKey)
		if err != nil {
			t.Errorf("preserved KV %s sentinel missing: %v", bucket, err)
			continue
		}
		if got := string(entry.Value()); got != want {
			t.Errorf("preserved KV %s sentinel = %q, want %q", bucket, got, want)
		}
	}
	objects, err := js.ObjectStore(ctx, upgradeObjectBucket)
	if err != nil {
		t.Fatalf("preserved object store %s missing: %v", upgradeObjectBucket, err)
	}
	got, err := objects.GetString(ctx, upgradeObjectKey)
	if err != nil {
		t.Fatalf("preserved object sentinel missing: %v", err)
	}
	if got != want {
		t.Errorf("preserved object sentinel = %q, want %q", got, want)
	}
}

func upgradeAssertUnrelatedStream(t *testing.T, ctx context.Context, js jetstream.JetStream) {
	t.Helper()
	stream, err := js.Stream(ctx, upgradeUnrelatedStream)
	if err != nil {
		t.Fatalf("preserved unrelated stream %s missing: %v", upgradeUnrelatedStream, err)
	}
	message, err := stream.GetLastMsgForSubject(ctx, upgradeUnrelatedSubject)
	if err != nil {
		t.Fatalf("preserved unrelated stream sentinel missing: %v", err)
	}
	if got := string(message.Data); got != upgradeUnrelatedMessage {
		t.Fatalf("preserved unrelated stream sentinel = %q, want %q", got, upgradeUnrelatedMessage)
	}
}

func upgradeStringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
