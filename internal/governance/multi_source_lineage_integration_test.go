//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/processor/supersession"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semstreams/component"
	semgraph "github.com/c360studio/semstreams/graph"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/pkg/fusion/fusionvocab"
)

// TestIntegration_MultiSourceVersionedLineage is the Tier-B multi-source live
// validation (never validated live before): the REAL ast-source component indexes
// THREE sources into one graph — the same dependency at two versions (depA v1.9.0
// and v1.10.0) plus a distinct app (appB, version-less) — so this exercises, end to
// end through real parsing (not hand-stamped triples):
//
//   - ast-source emits code.artifact.project/version from real per-source config;
//   - the supersession pass links the two depA versions with lineage edges and
//     classifies changed vs unchanged symbols;
//   - cross-source isolation: appB (a different project) is NOT linked to depA;
//   - the versioned-source differentiator ranks live — the current version outranks
//     the demoted historical one;
//   - both sources are queryable in the same graph (cross-source retrieval).
func TestIntegration_MultiSourceVersionedLineage(t *testing.T) {
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

	root := t.TempDir()
	write := func(rel, src string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// depA at two versions: run() changes body across versions; stable() is identical.
	write("depA-1.9.0/pkg/client.py", "def run():\n    return 1\n\ndef stable():\n    return 0\n")
	write("depA-1.10.0/pkg/client.py", "def run():\n    return 1 + 1\n\ndef stable():\n    return 0\n")
	// A distinct second source (different project, version-less).
	write("appB/main.py", "def app_main():\n    return 42\n")

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": filepath.Join(root, "depA-1.9.0"), "org": "acme", "project": "depA", "version": "v1.9.0", "languages": []string{"python"}},
			{"path": filepath.Join(root, "depA-1.10.0"), "org": "acme", "project": "depA", "version": "v1.10.0", "languages": []string{"python"}},
			{"path": filepath.Join(root, "appB"), "org": "acme", "project": "appB", "languages": []string{"python"}},
		},
		"watch_enabled":  false,
		"index_interval": "",
		"stream_name":    "GRAPH",
	})
	if err != nil {
		t.Fatalf("marshal ast-source config: %v", err)
	}
	discovered, err := astsource.NewComponent(astCfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("ast-source NewComponent: %v", err)
	}
	astComp := discovered.(*astsource.Component)
	if err := astComp.Initialize(); err != nil {
		t.Fatalf("ast-source Initialize: %v", err)
	}
	if err := astComp.Start(ctx); err != nil {
		t.Fatalf("ast-source Start: %v", err)
	}
	t.Cleanup(func() { _ = astComp.Stop(5 * time.Second) })

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("query client: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	// Wait until all three sources' symbols are queryable, then locate them.
	var runOld, runNew, stableOld, stableNew, appMain *semgraph.EntityState
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ents := prefixAll(ctx, qc, "acme.semsource.python")
		runOld = pickByNameVersion(ents, "run", "v1.9.0")
		runNew = pickByNameVersion(ents, "run", "v1.10.0")
		stableOld = pickByNameVersion(ents, "stable", "v1.9.0")
		stableNew = pickByNameVersion(ents, "stable", "v1.10.0")
		appMain = pickByNameVersion(ents, "app_main", "")
		if runOld != nil && runNew != nil && stableOld != nil && stableNew != nil && appMain != nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if runOld == nil || runNew == nil || stableOld == nil || stableNew == nil || appMain == nil {
		t.Fatalf("not all symbols indexed: runOld=%v runNew=%v stableOld=%v stableNew=%v appMain=%v",
			runOld != nil, runNew != nil, stableOld != nil, stableNew != nil, appMain != nil)
	}

	// (1) REAL ast-source emitted version scoping on depA; appB stays version-less.
	assertHasObject(t, runOld, semsourceast.CodeProject, "depA")
	assertHasObject(t, runOld, semsourceast.CodeVersion, "v1.9.0")
	if countObject(appMain, semsourceast.CodeVersion, "v1.9.0")+countObject(appMain, semsourceast.CodeVersion, "v1.10.0") != 0 {
		t.Errorf("appB app_main should carry no version triple, got %+v", appMain.Triples)
	}
	// Cross-source distinctness: the two depA versions are distinct entities.
	if runOld.ID == runNew.ID {
		t.Fatalf("v1.9.0 and v1.10.0 run collapsed to one ID %q", runOld.ID)
	}

	// Start supersession and run a pass over the REAL parsed entities.
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
	// Only depA's versioned entities are considered — appB is version-less, so the
	// pass ignores it entirely (entities=6 = 2 versions × {file, run, stable}).
	// Each of the three artifacts corresponds across the two versions; run and the
	// containing file changed (run's body differs), stable did not. We assert the
	// meaningful symbol-level classification below and keep the counts robust.
	if summary.Entities != 6 {
		t.Errorf("supersession considered %d entities, want 6 (only depA's versioned symbols): %+v", summary.Entities, summary)
	}
	if summary.Supersedes < 2 {
		t.Errorf("supersedes = %d, want >= 2 (run + stable at least): %+v", summary.Supersedes, summary)
	}
	if summary.Changed < 1 || summary.Unchanged < 1 {
		t.Errorf("changed/unchanged = %d/%d, want >= 1 each: %+v", summary.Changed, summary.Unchanged, summary)
	}

	// (2) Lineage edges connect the two depA versions with the right direction.
	waitTriple(t, ctx, qc, runNew.ID, semsourceast.CodeSupersedes, runOld.ID, 15*time.Second)
	waitTriple(t, ctx, qc, runOld.ID, semsourceast.CodeSupersededBy, runNew.ID, 15*time.Second)
	waitTriple(t, ctx, qc, runNew.ID, semsourceast.CodeLineageChange, "changed", 15*time.Second)
	waitTriple(t, ctx, qc, stableNew.ID, semsourceast.CodeLineageChange, "unchanged", 15*time.Second)

	// (3) Cross-source isolation: appB's symbol carries no lineage edge to depA.
	freshApp, _ := fetchEntity(ctx, qc, appMain.ID)
	if freshApp != nil {
		for _, pred := range []string{semsourceast.CodeSupersedes, semsourceast.CodeSupersededBy} {
			for i := range freshApp.Triples {
				if freshApp.Triples[i].Predicate == pred {
					t.Errorf("appB app_main got a lineage edge %s -> %v; must not cross projects",
						pred, freshApp.Triples[i].Object)
				}
			}
		}
	}

	// (4) Differentiator ranks live: the current (v1.10.0) run outranks the demoted
	// historical (v1.9.0) one, and both are retained (demotion is a reorder).
	// WithSignals attaches predicate-salience ranking (matching production
	// code-context wiring); without it, ranking is pure resolve-order +
	// lexical and never actually exercises the code.lineage.superseded_by
	// weight this test claims to prove (ci-proof-chain D5).
	engine := fusion.NewEngine(fusionnats.New(tc.Client, 0), fusion.NewBodyResolver(fusion.MapStoreResolver{})).
		WithSignals(fusionvocab.New())
	lens := prefixLens{code.New()}
	prefix := commonPrefix(runOld.ID, runNew.ID)
	var newRank, oldRank int
	rankDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(rankDeadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{Query: prefix, Want: []fusion.Want{fusion.WantRelations}}, lens)
		if ferr != nil {
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		newRank = rankByHandle(resp.Nodes, runNew.ID)
		oldRank = rankByHandle(resp.Nodes, runOld.ID)
		if newRank >= 0 && oldRank >= 0 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if newRank < 0 || oldRank < 0 {
		t.Fatalf("both run versions must be retained and rankable: newRank=%d oldRank=%d", newRank, oldRank)
	}
	if newRank >= oldRank {
		t.Errorf("current v1.10.0 (rank %d) should outrank historical v1.9.0 (rank %d)", newRank, oldRank)
	}

	// (5) Cross-source retrieval: the distinct appB source is queryable in the same
	// graph as depA (both projects live side by side).
	if appMain.ID == "" || countObject(appMain, semsourceast.DcTitle, "app_main") == 0 {
		t.Errorf("appB app_main not retrievable alongside depA: %+v", appMain)
	}
}

// prefixAll fetches every entity under a prefix (bounded), or nil on error.
func prefixAll(ctx context.Context, qc queryclient.Client, prefix string) []semgraph.EntityState {
	ents, _, err := qc.QueryPrefixAll(ctx, semgraph.PrefixQueryRequest{Prefix: prefix}, 500)
	if err != nil {
		return nil
	}
	return ents
}

// pickByNameVersion returns the entity whose DcTitle == name and CodeVersion ==
// version ("" matches an entity with no version triple), or nil.
func pickByNameVersion(ents []semgraph.EntityState, name, version string) *semgraph.EntityState {
	for i := range ents {
		e := &ents[i]
		if countObject(e, semsourceast.DcTitle, name) == 0 {
			continue
		}
		hasVer := countObject(e, semsourceast.CodeVersion, version) > 0
		versionless := true
		for j := range e.Triples {
			if e.Triples[j].Predicate == semsourceast.CodeVersion {
				versionless = false
				break
			}
		}
		if (version == "" && versionless) || (version != "" && hasVer) {
			return e
		}
	}
	return nil
}

// commonPrefix returns the longest shared dot-segment prefix of two entity IDs —
// the seed the fusion prefix resolver expands to both version subgraphs.
func commonPrefix(a, b string) string {
	as, bs := splitSegs(a), splitSegs(b)
	n := len(as)
	if len(bs) < n {
		n = len(bs)
	}
	shared := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if as[i] != bs[i] {
			break
		}
		shared = append(shared, as[i])
	}
	out := ""
	for i, s := range shared {
		if i > 0 {
			out += "."
		}
		out += s
	}
	return out
}

func splitSegs(s string) []string {
	var segs []string
	cur := ""
	for _, r := range s {
		if r == '.' {
			segs = append(segs, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(segs, cur)
}
