//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/processor/supersession"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semstreams/component"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/pkg/fusion/fusionvocab"
)

// TestIntegration_Supersession_DemotesHistoricalInRanking proves ADR-0008 #3:
// after the supersession pass marks v1.9.0 as superseded_by v1.10.0, the fusion
// ranker (reading the negative code.lineage.superseded_by weight) places the
// current (un-superseded) entity ABOVE the historical one — while the historical
// entity stays present in the results (bounded reorder, not exclusion).
func TestIntegration_Supersession_DemotesHistoricalInRanking(t *testing.T) {
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

	// Same symbol at two versions of one source; identical name/path so only the
	// supersession demote can separate them in ranking.
	pub, err := entitypub.New(tc.Client, nil)
	if err != nil {
		t.Fatalf("entitypub.New: %v", err)
	}
	pub.Start(ctx)
	runOld := publishVersioned(t, ctx, pub, "semstreams", "v1.9.0", "pkg/run.go", "Run", "run", "code:run-old")
	runNew := publishVersioned(t, ctx, pub, "semstreams", "v1.10.0", "pkg/run.go", "Run", "run", "code:run-new")
	pub.Stop()

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("query client: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })
	for _, id := range []string{runOld, runNew} {
		if _, ok := waitEntity(t, ctx, qc, id, 20*time.Second); !ok {
			t.Fatalf("entity never became queryable: %s", id)
		}
	}

	// Run the supersession pass so runOld carries superseded_by (the demote input).
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
	_ = runPassAndSummary(t, ctx, tc.Client)
	waitTriple(t, ctx, qc, runOld, semsourceast.CodeSupersededBy, runNew, 15*time.Second)

	// Rank both versions via the fusion engine over their shared version prefix.
	// prefixLens forces deterministic prefix resolution (the harness has no
	// graph-embedding); rankEntities still folds predicate salience, so the
	// -2.0 superseded_by weight decides the order.
	// WithSignals attaches predicate-salience ranking (matching production
	// code-context wiring); without it, ranking is pure resolve-order +
	// lexical and never actually exercises the code.lineage.superseded_by
	// weight this test claims to prove (ci-proof-chain D5).
	engine := fusion.NewEngine(fusionnats.New(tc.Client, 0), fusion.NewBodyResolver(fusion.MapStoreResolver{})).
		WithSignals(fusionvocab.New())
	lens := prefixLens{code.New()}
	// graph.query.prefix matches on dot-delimited segment boundaries, and the two
	// versions differ in their `system` segment (semstreams-v1-9-0 vs -v1-10-0),
	// so the deepest shared segment-boundary prefix is org.platform.domain.
	const prefix = "acme.semsource.golang"

	newRank, oldRank := -1, -1
	var last fusion.Response
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: prefix,
			Want:  []fusion.Want{fusion.WantRelations},
		}, lens)
		if ferr != nil {
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		last = resp
		newRank = rankByHandle(resp.Nodes, runNew)
		oldRank = rankByHandle(resp.Nodes, runOld)
		if resp.Index.Ready && newRank >= 0 && oldRank >= 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Both present → retention (demotion is a reorder, not an exclusion).
	if newRank < 0 || oldRank < 0 {
		handles := make([]string, 0, len(last.Nodes))
		for i := range last.Nodes {
			handles = append(handles, last.Nodes[i].Name+"="+last.Nodes[i].Handle)
		}
		t.Fatalf("both versions must appear in ranked results (retention): newRank=%d oldRank=%d\n ready=%v state=%s nodes=%v misses=%+v\n runNew=%s runOld=%s",
			newRank, oldRank, last.Index.Ready, last.Index.State, handles, last.Misses, runNew, runOld)
	}
	// Current above historical.
	if newRank >= oldRank {
		t.Errorf("current (v1.10.0) must rank above historical (v1.9.0): newRank=%d oldRank=%d", newRank, oldRank)
	}
}

// rankByHandle returns the 0-based rank of the node whose opaque Handle equals
// the given entity ID, or -1 if absent.
func rankByHandle(nodes []fusion.Node, id string) int {
	for i := range nodes {
		if nodes[i].Handle == id {
			return i
		}
	}
	return -1
}
