//go:build integration

package sourcemanifest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

func TestIntegration_WorkbenchMissingReadinessRespondersDoNotOpenCircuit(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx := context.Background()
	sub, err := tc.Client.SubscribeForRequests(ctx, "semsource.test.unrelated", func(_ context.Context, body []byte) ([]byte, error) {
		return body, nil
	})
	if err != nil {
		t.Fatalf("subscribe unrelated responder: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	c := &Component{
		config:           Config{Namespace: "acme"},
		client:           tc.Client,
		logger:           newTestLogger(),
		running:          true,
		readinessTimeout: 10 * time.Millisecond,
		statusData: mustMarshal(t, &StatusPayload{
			Namespace: "acme",
			Phase:     PhaseReady,
			Sources:   []SourceStatus{{InstanceName: "ast-source", Phase: SourcePhaseWatching}},
			Timestamp: time.Now(),
		}),
	}

	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		c.handleWorkbenchCapabilities(rec,
			httptest.NewRequest(http.MethodGet, "/source-manifest/capabilities", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", i, rec.Code, rec.Body.String())
		}
		var got workbenchCapabilitiesResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("request %d decode: %v", i, err)
		}
		if got.Readiness.StructuralIndex.State != readinessUnknown ||
			got.Readiness.SemanticIndex.State != readinessUnknown {
			t.Fatalf("request %d readiness=%+v", i, got.Readiness)
		}
	}

	if tc.Client.Status() == natsclient.StatusCircuitOpen {
		t.Fatal("optional readiness probes opened the shared NATS circuit")
	}
	reply, err := tc.Client.Request(ctx, "semsource.test.unrelated", []byte("ok"), time.Second)
	if err != nil {
		t.Fatalf("unrelated request failed after optional probes: %v", err)
	}
	if string(reply) != "ok" {
		t.Fatalf("unrelated reply = %q, want ok", reply)
	}
}
