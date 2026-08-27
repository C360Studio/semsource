//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	nats "github.com/nats-io/nats.go"

	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/internal/miniotest"
	"github.com/c360studio/semsource/internal/sourcestatus"
	objectstoresource "github.com/c360studio/semsource/processor/objectstore-source"
)

// corpus is the fixture this test ingests: three documents the pipeline
// parses, one artifact it cannot, and one empty file. The last two are here so
// the skip accounting is exercised by the same pass that proves ingestion —
// a bucket of pure markdown would never show whether skips are counted.
var corpus = map[string]string{
	"reports/q3-review.md": "# Q3 Review\n\nRevenue grew by eleven percent across the quarter.\n",
	"reports/q4-plan.md":   "# Q4 Plan\n\nHiring focus shifts to platform reliability.\n",
	"reports/notes.txt":    "Standing notes for the quarterly review cycle.\n",
	"reports/diagram.png":  "not a document the pipeline can parse",
	"reports/empty.md":     "",
	// Outside the configured prefix: it must not be ingested at all.
	"scratch/ignored.md": "# Ignored\n\nThis lives outside the prefix.\n",
}

// TestIntegration_ObjectStoreCorpusReachesTheGraph is the end-to-end proof for
// the object-store source: a real bucket, a real NATS graph, and a query that
// answers from what was ingested.
//
// Everything below the component is real. The one thing it deliberately does
// not prove is Garage compatibility — MinIO passing does not establish that,
// and #202 exists to close it.
func TestIntegration_ObjectStoreCorpusReachesTheGraph(t *testing.T) {
	ctx := context.Background()

	// ── The bucket ──────────────────────────────────────────────────────────
	store, bucket := miniotest.NewStore(t)
	for key, body := range corpus {
		if err := store.Put(ctx, key, []byte(body)); err != nil {
			t.Fatalf("seed %q: %v", key, err)
		}
	}

	// ── The graph ───────────────────────────────────────────────────────────
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)

	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, index.Stop) })
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, query.Stop) })

	// ── The source ──────────────────────────────────────────────────────────
	reports := collectStatusReports(t, tc.Client)
	source := startObjectStoreSource(t, ctx, tc.Client, metricsRegistry, bucket, "reports/")
	t.Cleanup(func() { _ = stopWithin(10*time.Second, source.Stop) })

	// ── The proof ───────────────────────────────────────────────────────────
	//
	// Identity is built from the bucket and the object key and nothing local,
	// so the expected ID is computable here rather than discovered.
	system := entitySystem(t, bucket)
	q3ID := dochandler.DocumentEntityID(testOrgNamespace, system, "reports/q3-review.md")

	stored := waitForEntityState(t, ctx, tc.Client, q3ID, 60*time.Second)
	if stored == nil {
		t.Fatal("the document never reached ENTITY_STATES")
	}
	if !hasPredicate(stored, "source.doc.file-path") {
		t.Errorf("the stored document carries no path predicate: %+v", stored)
	}

	// One query, answered from the ingested corpus. graph.query.prefix is the
	// governed read contract a consumer would use.
	page := requestPrefixPage(t, ctx, tc.Client, semgraph.PrefixQueryRequest{
		Prefix: testOrgNamespace + ".semsource.web." + system,
		Limit:  50,
	})
	if len(page.Entities) == 0 {
		t.Fatal("graph.query.prefix answered with nothing after a completed ingest")
	}

	ingestedKeys := map[string]bool{}
	for i := range page.Entities {
		for _, triple := range page.Entities[i].Triples {
			if triple.Predicate != "source.doc.file-path" {
				continue
			}
			if path, isString := triple.Object.(string); isString {
				ingestedKeys[path] = true
			}
		}
	}

	for _, key := range []string{"reports/q3-review.md", "reports/q4-plan.md", "reports/notes.txt"} {
		if !ingestedKeys[key] {
			t.Errorf("%q is missing from the graph; ingested: %v", key, keysOf(ingestedKeys))
		}
	}
	// Prefix scoping is a promise about what is NOT there, which is the half
	// that fails silently.
	if ingestedKeys["scratch/ignored.md"] {
		t.Error("an object outside the configured prefix was ingested")
	}
	// Unparseable and empty objects must not arrive as documents that exist
	// and say nothing.
	for _, key := range []string{"reports/diagram.png", "reports/empty.md"} {
		if ingestedKeys[key] {
			t.Errorf("%q was published as a document rather than skipped", key)
		}
	}

	// ── Status ──────────────────────────────────────────────────────────────
	report := awaitReport(t, reports, 60*time.Second, func(r sourcestatus.Report) bool {
		return r.EntityCount > 0 && r.ObjectsSkipped != nil
	})
	if report.SourceType != "s3" {
		t.Errorf("SourceType = %q", report.SourceType)
	}
	if report.Phase == "" {
		t.Error("the source reported no phase")
	}
	// The skipped objects are visible on the shared contract, with reasons —
	// the difference between "there is no such document" and "that document
	// was never parsed".
	if report.ObjectsSkipped["unsupported_format"] < 1 {
		t.Errorf("ObjectsSkipped = %v, want the unparseable artifact counted", report.ObjectsSkipped)
	}
	if report.ObjectsSkipped["empty_object"] < 1 {
		t.Errorf("ObjectsSkipped = %v, want the empty object counted", report.ObjectsSkipped)
	}
	if report.LostTotal != 0 {
		t.Errorf("LostTotal = %d — the corpus did not arrive whole", report.LostTotal)
	}
}

const testOrgNamespace = "acme"

// startObjectStoreSource builds and starts the component against a live
// bucket, through the same factory the composition root uses. An empty prefix
// means the whole bucket.
func startObjectStoreSource(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	metricsRegistry *metric.MetricsRegistry,
	bucket, prefix string,
) *objectstoresource.Component {
	t.Helper()

	miniotest.SetCredentials(t)

	configJSON, err := json.Marshal(map[string]any{
		"bucket":          bucket,
		"prefix":          prefix,
		"endpoint":        miniotest.Endpoint(t),
		"region":          miniotest.Region,
		"path_style":      true,
		"org":             testOrgNamespace,
		"watch_enabled":   false,
		"body_store_root": t.TempDir(),
		"instance_name":   "objectstore-source-test",
		"ports": map[string]any{
			"outputs": []component.PortDefinition{
				{
					Name: "graph.ingest",
					Config: component.JetStreamPort{
						StreamName: "GRAPH",
						Subjects:   []string{"graph.ingest.entity"},
					},
					Required: true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal objectstore-source config: %v", err)
	}

	discovered, err := objectstoresource.NewComponent(configJSON, component.Dependencies{
		NATSClient:      client,
		MetricsRegistry: metricsRegistry,
	})
	if err != nil {
		t.Fatalf("NewComponent() error = %v", err)
	}
	source, isComponent := discovered.(*objectstoresource.Component)
	if !isComponent {
		t.Fatalf("NewComponent returned %T", discovered)
	}
	if err := source.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := source.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return source
}

// entitySystem is the system segment the source publishes under: the bucket
// slug, since this test declares no project override.
//
// Computed through the handler's own System method rather than by
// reimplementing the slug rule, so a change to identity construction moves the
// expectation with it instead of leaving this test asserting the old shape.
func entitySystem(t *testing.T, bucket string) string {
	t.Helper()

	probe := objectstore.New(nil, nil, testOrgNamespace)
	system := probe.System(bucket)
	if system == "" {
		t.Fatalf("bucket %q produced no system slug", bucket)
	}
	return system
}

// collectStatusReports subscribes to the internal status subject and returns a
// channel of decoded reports.
func collectStatusReports(t *testing.T, client *natsclient.Client) <-chan sourcestatus.Report {
	t.Helper()

	reports := make(chan sourcestatus.Report, 32)
	sub, err := client.Subscribe(t.Context(), "semsource.internal.status", func(_ context.Context, msg *nats.Msg) {
		// Strict decoding: a field the source publishes that this contract
		// does not know about is a contract break, not a curiosity.
		dec := json.NewDecoder(strings.NewReader(string(msg.Data)))
		dec.DisallowUnknownFields()

		var report sourcestatus.Report
		if err := dec.Decode(&report); err != nil {
			t.Errorf("status report does not decode against the shared contract: %v; body=%s", err, msg.Data)
			return
		}
		select {
		case reports <- report:
		default:
		}
	})
	if err != nil {
		t.Fatalf("subscribe to status: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return reports
}

// awaitReport waits for a status report satisfying want.
func awaitReport(
	t *testing.T,
	reports <-chan sourcestatus.Report,
	timeout time.Duration,
	want func(sourcestatus.Report) bool,
) sourcestatus.Report {
	t.Helper()

	deadline := time.After(timeout)
	var last sourcestatus.Report
	for {
		select {
		case report := <-reports:
			last = report
			if want(report) {
				return report
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a matching status report; last: %+v", last)
			return sourcestatus.Report{}
		}
	}
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}
