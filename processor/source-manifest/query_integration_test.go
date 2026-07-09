//go:build integration

package sourcemanifest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

func TestIntegration_QuerySubjects(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name: "GRAPH",
			Subjects: []string{
				"graph.ingest.manifest",
				"graph.ingest.status",
				"graph.ingest.predicates",
			},
		}),
	)

	cfg := Config{
		Namespace: "acme",
		Sources: []ManifestSource{
			{Type: "ast", Path: "/workspace/src", Language: "go", Watch: false},
		},
		ExpectedSourceCount: 1,
		HeartbeatInterval:   "1h",
		SeedTimeout:         "1h",
	}
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	disc, err := NewComponent(rawCfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := disc.(*Component)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop(5 * time.Second) })

	var manifest ManifestPayload
	requestJSON(t, ctx, tc.Client, querySubject, &manifest)
	if manifest.Namespace != "acme" || len(manifest.Sources) != 1 || manifest.Sources[0].Type != "ast" {
		t.Fatalf("unexpected graph.query.sources response: %+v", manifest)
	}

	var predicates PredicateSchemaPayload
	requestJSON(t, ctx, tc.Client, predicatesQuerySubject, &predicates)
	if len(predicates.Sources) != 1 || predicates.Sources[0].SourceType != "ast" {
		t.Fatalf("unexpected graph.query.predicates sources: %+v", predicates.Sources)
	}
	if len(predicates.Sources[0].Predicates) == 0 {
		t.Fatal("graph.query.predicates returned no ast predicates")
	}

	report := SourceStatusReport{
		InstanceName: "ast-source-repo",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  42,
		TypeCounts: map[string]int64{
			"code.symbol": 40,
			"code.file":   2,
		},
		Timestamp: time.Now(),
	}
	reportData, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := tc.Client.Publish(ctx, statusReportSubject, reportData); err != nil {
		t.Fatalf("publish status report: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		var status StatusPayload
		requestJSON(t, ctx, tc.Client, statusQuerySubject, &status)
		if status.Namespace == "acme" && status.Phase == PhaseReady && status.TotalEntities == 42 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("graph.query.status never reached ready with 42 entities")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func requestJSON(t *testing.T, ctx context.Context, client *natsclient.Client, subject string, out any) {
	t.Helper()
	raw, err := client.Request(ctx, subject, []byte("{}"), 3*time.Second)
	if err != nil {
		t.Fatalf("%s request: %v", subject, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode %s response %q: %v", subject, raw, err)
	}
}
