//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/processor/supersession"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/ontology"
	"github.com/c360studio/semstreams/component"
	semgraph "github.com/c360studio/semstreams/graph"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
)

// TestIntegration_Supersession_CrossVersionLineage drives the supersession pass
// end to end over a real graph-ingest/index/query stack: it ingests the same two
// symbols at two versions (one whose body changed, one unchanged), triggers the
// pass over NATS, and asserts the lineage edges, change classification,
// idempotency, and retention of the superseded version.
func TestIntegration_Supersession_CrossVersionLineage(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(ctx, tc.Client, nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	mr := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = q.Stop(5 * time.Second) })

	// Ingest the same two symbols at two versions of source "semstreams":
	// Run's body changes across versions; Stable's body is identical.
	pub, err := entitypub.New(tc.Client, nil)
	if err != nil {
		t.Fatalf("entitypub.New: %v", err)
	}
	pub.Start(ctx)
	runOld := publishVersioned(t, ctx, pub, "semstreams", "v1.9.0", "pkg/run.go", "Run", "run", "code:run-old")
	runNew := publishVersioned(t, ctx, pub, "semstreams", "v1.10.0", "pkg/run.go", "Run", "run", "code:run-new")
	stableOld := publishVersioned(t, ctx, pub, "semstreams", "v1.9.0", "pkg/stable.go", "Stable", "stable", "code:stable-same")
	stableNew := publishVersioned(t, ctx, pub, "semstreams", "v1.10.0", "pkg/stable.go", "Stable", "stable", "code:stable-same")
	pub.Stop() // flush buffered publishes

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("query client: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	// Wait until all four entities are queryable.
	for _, id := range []string{runOld, runNew, stableOld, stableNew} {
		if _, ok := waitEntity(t, ctx, qc, id, 20*time.Second); !ok {
			t.Fatalf("entity never became queryable: %s", id)
		}
	}

	// Start the supersession component and trigger a pass over NATS.
	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdiscovered, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdiscovered.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = scomp.Stop(5 * time.Second) })

	summary := runPassAndSummary(t, ctx, tc.Client)
	if summary.Supersedes != 2 {
		t.Errorf("summary supersedes = %d, want 2 (Run + Stable): %+v", summary.Supersedes, summary)
	}
	if summary.Corresponding != 2 {
		t.Errorf("summary corresponding = %d, want 2: %+v", summary.Corresponding, summary)
	}
	if summary.Changed != 1 || summary.Unchanged != 1 {
		t.Errorf("summary changed/unchanged = %d/%d, want 1/1: %+v", summary.Changed, summary.Unchanged, summary)
	}

	// Newer Run supersedes older Run, classified changed; older carries the inverse.
	waitTriple(t, ctx, qc, runNew, semsourceast.CodeSupersedes, runOld, 15*time.Second)
	waitTriple(t, ctx, qc, runNew, semsourceast.CodeLineageChange, "changed", 15*time.Second)
	waitTriple(t, ctx, qc, runOld, semsourceast.CodeSupersededBy, runNew, 15*time.Second)

	// Stable is unchanged across versions.
	waitTriple(t, ctx, qc, stableNew, semsourceast.CodeLineageChange, "unchanged", 15*time.Second)

	// Retention: the superseded v1.9.0 Run entity and its identity triples survive.
	older, ok := fetchEntity(ctx, qc, runOld)
	if !ok {
		t.Fatal("superseded v1.9.0 Run entity disappeared after the pass")
	}
	assertHasObject(t, older, semsourceast.CodeProject, "semstreams")
	assertHasObject(t, older, semsourceast.CodeVersion, "v1.9.0")

	// Idempotency: a second pass over the now-edged graph adds no duplicate edge.
	_ = runPassAndSummary(t, ctx, tc.Client)
	time.Sleep(500 * time.Millisecond)
	newer, ok := fetchEntity(ctx, qc, runNew)
	if !ok {
		t.Fatal("newer Run entity missing on idempotency check")
	}
	if n := countObject(newer, semsourceast.CodeSupersedes, runOld); n != 1 {
		t.Errorf("supersedes edge count after two passes = %d, want exactly 1 (no duplicate)", n)
	}
}

// publishVersioned builds and publishes a versioned Go function entity carrying
// the version-scoping + body-hash triples the pass reads, and returns its ID.
func publishVersioned(t *testing.T, ctx context.Context, pub *entitypub.Publisher,
	project, version, path, name, pkg, bodyKey string) string {
	t.Helper()
	const org, lang, ctype = "acme", "golang", "function"
	system := entityid.ScopedSystemSlug(project, version)
	inst := semsourceast.BuildInstanceID(path, name, semsourceast.TypeFunction)
	id := entityid.Build(org, entityid.PlatformSemsource, lang, system, ctype, inst)
	now := time.Now()
	triples := []message.Triple{
		{Subject: id, Predicate: semsourceast.CodeType, Object: ctype},
		{Subject: id, Predicate: semsourceast.DcTitle, Object: name},
		{Subject: id, Predicate: semsourceast.CodePath, Object: path},
		{Subject: id, Predicate: semsourceast.CodePackage, Object: pkg},
		{Subject: id, Predicate: semsourceast.CodeProject, Object: project},
		{Subject: id, Predicate: semsourceast.CodeVersion, Object: version},
		{Subject: id, Predicate: semsourceast.CodeBodyKey, Object: bodyKey},
		{Subject: id, Predicate: semsourceast.DcCreated, Object: now.Format(time.RFC3339)},
	}
	payload := &semsourcegraph.EntityPayload{
		ID:                  id,
		TripleData:          ontology.StampClass(id, triples, now),
		UpdatedAt:           now,
		IndexingProfileHint: semsourcegraph.IndexingProfileContent,
	}
	if err := entitypub.ValidatePayload(payload); err != nil {
		t.Fatalf("invalid versioned payload %s: %v", id, err)
	}
	pub.Send(payload)
	return id
}

// runSummary mirrors the supersession pass's JSON run report.
type runSummary struct {
	Entities      int  `json:"entities"`
	Groups        int  `json:"groups"`
	Corresponding int  `json:"corresponding"`
	Supersedes    int  `json:"supersedes"`
	Incomparable  int  `json:"incomparable"`
	Changed       int  `json:"changed"`
	Unchanged     int  `json:"unchanged"`
	Truncated     bool `json:"truncated"`
}

// runPassAndSummary triggers one supersession pass over NATS and returns its
// decoded run summary.
func runPassAndSummary(t *testing.T, ctx context.Context, client *natsclient.Client) runSummary {
	t.Helper()
	resp, err := client.Request(ctx, supersession.DefaultTriggerSubject, nil, 20*time.Second)
	if err != nil {
		t.Fatalf("trigger supersession pass: %v", err)
	}
	var summary runSummary
	if err := json.Unmarshal(resp, &summary); err != nil {
		t.Fatalf("decode run summary %q: %v", resp, err)
	}
	return summary
}

// fetchEntity reads an entity's current state fresh via graph.query.prefix (no
// client-side entity cache, so it always reflects the latest ingest merge).
func fetchEntity(ctx context.Context, qc queryclient.Client, id string) (*semgraph.EntityState, bool) {
	ents, _, err := qc.QueryPrefixAll(ctx, semgraph.PrefixQueryRequest{Prefix: id}, 10)
	if err != nil {
		return nil, false
	}
	for i := range ents {
		if ents[i].ID == id {
			e := ents[i]
			return &e, true
		}
	}
	return nil, false
}

func waitEntity(t *testing.T, ctx context.Context, qc queryclient.Client, id string, timeout time.Duration) (*semgraph.EntityState, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e, ok := fetchEntity(ctx, qc, id); ok {
			return e, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, false
}

func waitTriple(t *testing.T, ctx context.Context, qc queryclient.Client, id, predicate, object string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e, ok := fetchEntity(ctx, qc, id); ok && countObject(e, predicate, object) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("entity %s never got triple %s -> %s within %s", id, predicate, object, timeout)
}

func countObject(e *semgraph.EntityState, predicate, object string) int {
	n := 0
	for i := range e.Triples {
		if e.Triples[i].Predicate == predicate {
			if s, ok := e.Triples[i].Object.(string); ok && s == object {
				n++
			}
		}
	}
	return n
}

func assertHasObject(t *testing.T, e *semgraph.EntityState, predicate, object string) {
	t.Helper()
	if countObject(e, predicate, object) == 0 {
		t.Errorf("entity %s missing triple %s -> %s", e.ID, predicate, object)
	}
}
