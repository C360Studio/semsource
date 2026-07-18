//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	beta148KnownPredicate   = "source.doc.file-path"
	beta148RetiredPredicate = "source.doc.file_path"
	beta148KnownObject      = "cutover.md"
	beta148KnownContent     = "# Beta 148 Cutover\n\ncanonical known answer\n"
	beta148ObjectBucket     = "PRESERVED_OBJECTS"
	beta148ObjectKey        = "cutover-sentinel.txt"
	beta148SentinelKey      = "cutover.sentinel"
	beta148UnrelatedStream  = "UNRELATED_EVENTS"
	beta148UnrelatedSubject = "unrelated.cutover.sentinel"
	beta148UnrelatedMessage = "preserve-unrelated-stream-beta148"
)

// This literal list is the reviewed beta.148 default. The parity assertion is
// intentionally first: a framework-owned bucket addition must stop the
// rehearsal for review instead of silently widening its deletion boundary.
var beta148DefaultFrameworkBuckets = []string{
	"ENTITY_STATES",
	"PREDICATE_INDEX",
	"INCOMING_INDEX",
	"OUTGOING_INDEX",
	"ALIAS_INDEX",
	"NAME_INDEX",
	"SPATIAL_INDEX",
	"TEMPORAL_INDEX",
	"TEMPORAL_INDEX_REVERSE",
	"CONTEXT_INDEX",
	"EMBEDDINGS_CACHE",
	"EMBEDDING_INDEX",
	"EMBEDDING_DEDUP",
	"COMMUNITY_INDEX",
	"ANOMALY_INDEX",
	"STRUCTURAL_INDEX",
}

var beta148PreservedKVBuckets = []string{
	"AUTHORITATIVE_INPUTS",
	"SOURCE_STORE",
	"CONTENT_STORE",
	"MEDIA_STORE",
	"UNRELATED_STATE",
}

type beta148Inventory struct {
	Streams []string
	KV      []string
	Objects []string
}

func TestE2E_Beta148CutoverRehearsal(t *testing.T) {
	if !reflect.DeepEqual(beta148DefaultFrameworkBuckets, semgraph.FrameworkOwnedBuckets()) {
		t.Fatalf("beta.148 framework bucket drift: literal=%v runtime=%v",
			beta148DefaultFrameworkBuckets, semgraph.FrameworkOwnedBuckets())
	}

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
	sentinelValue := "preserve-beta148"
	for _, bucket := range beta148PreservedKVBuckets {
		beta148CreateSentinelKV(t, ctx, js, bucket, sentinelValue)
	}
	beta148CreateSentinelKV(t, ctx, js, "PREDICATE_CATALOG", "legacy-beta145")

	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     beta148UnrelatedStream,
		Subjects: []string{beta148UnrelatedSubject},
		Storage:  jetstream.MemoryStorage,
	}); err != nil {
		t.Fatalf("create unrelated sentinel stream: %v", err)
	}
	if _, err := js.Publish(ctx, beta148UnrelatedSubject, []byte(beta148UnrelatedMessage)); err != nil {
		t.Fatalf("publish unrelated stream sentinel: %v", err)
	}

	// This object store is authoritative test data and intentionally distinct
	// from SemSource's production CONTENT body store.
	objects, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: beta148ObjectBucket})
	if err != nil {
		t.Fatalf("create preserved object store: %v", err)
	}
	if _, err := objects.PutString(ctx, beta148ObjectKey, sentinelValue); err != nil {
		t.Fatalf("put object sentinel: %v", err)
	}

	binPath := buildBinary(t)
	workDir := t.TempDir()
	docsDir := filepath.Join(workDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("create docs fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "cutover.md"), []byte(beta148KnownContent), 0o644); err != nil {
		t.Fatalf("write docs fixture: %v", err)
	}
	httpPort := freePort(t)
	wsPort := freePort(t)
	configPath := beta148WriteDocsConfig(t, workDir, docsDir, httpPort)

	stopFirst := beta148StartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer stopFirst()
	beta148WaitForReady(t, httpPort, 90*time.Second)
	beta148AssertKnownAnswer(t, nc, 45*time.Second)

	statusBucket, err := js.KeyValue(ctx, "COMPONENT_STATUS")
	if err != nil {
		t.Fatalf("open COMPONENT_STATUS: %v", err)
	}
	if _, err := statusBucket.PutString(ctx, beta148SentinelKey, sentinelValue); err != nil {
		t.Fatalf("put COMPONENT_STATUS sentinel: %v", err)
	}

	// Stop every graph writer before capturing the literal deletion sheet.
	stopFirst()
	inventory := beta148CaptureInventory(t, ctx, js)
	t.Logf("pre-cutover streams=%v", inventory.Streams)
	t.Logf("pre-cutover kv=%v", inventory.KV)
	t.Logf("pre-cutover object_stores=%v", inventory.Objects)

	streamSet := beta148StringSet(inventory.Streams)
	kvSet := beta148StringSet(inventory.KV)
	objectSet := beta148StringSet(inventory.Objects)
	if !streamSet["GRAPH"] || !kvSet["semstreams_config"] || !kvSet["PREDICATE_CATALOG"] {
		t.Fatalf("required observed cutover resources missing: GRAPH=%t semstreams_config=%t PREDICATE_CATALOG=%t",
			streamSet["GRAPH"], kvSet["semstreams_config"], kvSet["PREDICATE_CATALOG"])
	}
	if !kvSet["COMPONENT_STATUS"] || !objectSet[beta148ObjectBucket] {
		t.Fatalf("preservation inventory missing COMPONENT_STATUS or %s", beta148ObjectBucket)
	}
	if !streamSet[beta148UnrelatedStream] {
		t.Fatalf("preservation inventory missing unrelated stream %s", beta148UnrelatedStream)
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
	for _, bucket := range beta148DefaultFrameworkBuckets {
		if kvSet[bucket] {
			if err := js.DeleteKeyValue(ctx, bucket); err != nil {
				t.Fatalf("delete observed framework KV %s: %v", bucket, err)
			}
		}
	}
	for _, bucket := range []string{"ENTITY_SUFFIX_INDEX", "GRAPH_INGEST_APPLIED_SEQ", "PREDICATE_CATALOG"} {
		if kvSet[bucket] {
			if err := js.DeleteKeyValue(ctx, bucket); err != nil {
				t.Fatalf("delete observed migration KV %s: %v", bucket, err)
			}
		}
	}

	beta148AssertPreserved(t, ctx, js, sentinelValue)
	beta148AssertUnrelatedStream(t, ctx, js)
	if _, err := js.KeyValue(ctx, "PREDICATE_CATALOG"); err == nil {
		t.Fatal("legacy PREDICATE_CATALOG still exists after cutover")
	}

	stopSecond := beta148StartWriter(t, binPath, configPath, workDir, natsURL, httpPort, wsPort)
	defer stopSecond()
	beta148WaitForReady(t, httpPort, 90*time.Second)
	beta148AssertKnownAnswer(t, nc, 45*time.Second)
	stopSecond()

	beta148AssertPreserved(t, ctx, js, sentinelValue)
	beta148AssertUnrelatedStream(t, ctx, js)
	if _, err := js.KeyValue(ctx, "PREDICATE_CATALOG"); err == nil {
		t.Fatal("migrated restart recreated legacy PREDICATE_CATALOG")
	}
}

func beta148WriteDocsConfig(t *testing.T, workDir, docsDir string, httpPort int) string {
	t.Helper()
	cfg := map[string]any{
		"namespace": "beta148cutover",
		"http_port": httpPort,
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

func beta148StartWriter(
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
	logPipe(t, "beta148 stdout", stdout)
	logPipe(t, "beta148 stderr", stderr)
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

func beta148WaitForReady(t *testing.T, httpPort int, timeout time.Duration) {
	t.Helper()
	status := waitForReady(t, httpPort, timeout)
	if status.Phase != "ready" {
		t.Fatalf("SemSource phase = %q, want ready; status=%+v", status.Phase, status)
	}
}

func beta148AssertKnownAnswer(t *testing.T, nc *nats.Conn, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		request, _ := json.Marshal(map[string]string{"predicate": beta148KnownPredicate})
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
			var entity semgraph.EntityState
			if err := json.Unmarshal(entityResponse.Data, &entity); err != nil {
				lastErr = err
				continue
			}
			for _, triple := range entity.Triples {
				if triple.Predicate == beta148RetiredPredicate {
					t.Fatalf("known-answer entity %s contains retired predicate %s", entity.ID, beta148RetiredPredicate)
				}
				if triple.Predicate == beta148KnownPredicate && triple.Object == beta148KnownObject {
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

func beta148CreateSentinelKV(
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
	if _, err := kv.PutString(ctx, beta148SentinelKey, value); err != nil {
		t.Fatalf("put sentinel KV %s: %v", bucket, err)
	}
}

func beta148CaptureInventory(t *testing.T, ctx context.Context, js jetstream.JetStream) beta148Inventory {
	t.Helper()
	var inventory beta148Inventory
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

func beta148AssertPreserved(t *testing.T, ctx context.Context, js jetstream.JetStream, want string) {
	t.Helper()
	for _, bucket := range append(append([]string{}, beta148PreservedKVBuckets...), "COMPONENT_STATUS") {
		kv, err := js.KeyValue(ctx, bucket)
		if err != nil {
			t.Errorf("preserved KV %s missing: %v", bucket, err)
			continue
		}
		entry, err := kv.Get(ctx, beta148SentinelKey)
		if err != nil {
			t.Errorf("preserved KV %s sentinel missing: %v", bucket, err)
			continue
		}
		if got := string(entry.Value()); got != want {
			t.Errorf("preserved KV %s sentinel = %q, want %q", bucket, got, want)
		}
	}
	objects, err := js.ObjectStore(ctx, beta148ObjectBucket)
	if err != nil {
		t.Fatalf("preserved object store %s missing: %v", beta148ObjectBucket, err)
	}
	got, err := objects.GetString(ctx, beta148ObjectKey)
	if err != nil {
		t.Fatalf("preserved object sentinel missing: %v", err)
	}
	if got != want {
		t.Errorf("preserved object sentinel = %q, want %q", got, want)
	}
}

func beta148AssertUnrelatedStream(t *testing.T, ctx context.Context, js jetstream.JetStream) {
	t.Helper()
	stream, err := js.Stream(ctx, beta148UnrelatedStream)
	if err != nil {
		t.Fatalf("preserved unrelated stream %s missing: %v", beta148UnrelatedStream, err)
	}
	message, err := stream.GetLastMsgForSubject(ctx, beta148UnrelatedSubject)
	if err != nil {
		t.Fatalf("preserved unrelated stream sentinel missing: %v", err)
	}
	if got := string(message.Data); got != beta148UnrelatedMessage {
		t.Fatalf("preserved unrelated stream sentinel = %q, want %q", got, beta148UnrelatedMessage)
	}
}

func beta148StringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
