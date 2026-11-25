//go:build integration

package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semsource/internal/natstest"
)

// errStreamFull stands in for GRAPH's 10077 refusal at its byte bound.
var errStreamFull = errors.New("nats: API error: code=503 err_code=10077 description=maximum bytes exceeded")

// broadcastSeam replaces the GRAPH publish. Status publishes always succeed;
// a manifest or predicate-schema publish is refused while refuse is set, and
// every refused attempt is reported on attempts so the test paces the retry.
type broadcastSeam struct {
	mu        sync.Mutex
	refuse    bool
	delivered map[string]int
	attempts  chan string
	deliver   chan string
}

func newBroadcastSeam() *broadcastSeam {
	return &broadcastSeam{
		refuse:    true,
		delivered: map[string]int{},
		attempts:  make(chan string, 16),
		deliver:   make(chan string, 16),
	}
}

func (s *broadcastSeam) publish(ctx context.Context, subject string, _ []byte) error {
	if subject == statusSubject {
		return nil
	}
	s.mu.Lock()
	refuse := s.refuse
	if !refuse {
		s.delivered[subject]++
	}
	s.mu.Unlock()
	if !refuse {
		s.deliver <- subject
		return nil
	}
	select {
	case s.attempts <- subject:
	case <-ctx.Done():
		return ctx.Err()
	}
	return errStreamFull
}

func (s *broadcastSeam) accept() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refuse = false
}

func (s *broadcastSeam) deliveries(subject string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delivered[subject]
}

// awaitRefused blocks until subject has been refused n times, so the inline
// attempt and at least one background retry have run.
func (s *broadcastSeam) awaitRefused(ctx context.Context, t *testing.T, subject string, n int) {
	t.Helper()
	for seen := 0; seen < n; {
		select {
		case got := <-s.attempts:
			if got == subject {
				seen++
			}
		case <-ctx.Done():
			t.Fatalf("%s refused %d times, want %d", subject, seen, n)
		}
	}
}

// awaitDelivered accepts from now on and blocks until both broadcasts land,
// draining refused attempts still queued from before.
func (s *broadcastSeam) awaitDelivered(ctx context.Context, t *testing.T) {
	t.Helper()
	s.accept()
	landed := map[string]bool{}
	for len(landed) < 2 {
		select {
		case <-s.attempts:
		case got := <-s.deliver:
			landed[got] = true
		case <-ctx.Done():
			t.Fatalf("broadcasts delivered: %v, want %s and %s", landed, manifestSubject, predicatesSubject)
		}
	}
}

// recordingHandler keeps every log record so a test can count them.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) count(level slog.Level, msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == level && r.Message == msg {
			n++
		}
	}
	return n
}

// startRefusedManifest starts a source-manifest whose GRAPH publish is the
// seam, refusing, with a backoff short enough that retries are not the wait.
func startRefusedManifest(ctx context.Context, t *testing.T, logger *slog.Logger) (*Component, *broadcastSeam, *natstest.Server) {
	t.Helper()
	tc := natstest.New(t)
	rawCfg, err := json.Marshal(Config{
		Namespace:           "acme",
		Sources:             []ManifestSource{{Type: "ast", Path: "/workspace/src", Language: "go"}},
		ExpectedSourceCount: 1,
		HeartbeatInterval:   "1h",
		SeedTimeout:         "1h",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	deps := component.Dependencies{NATSClient: tc.Client}
	if logger != nil {
		deps.Logger = logger
	}
	disc, err := NewComponent(rawCfg, deps)
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := disc.(*Component)
	seam := newBroadcastSeam()
	c.publishStream = seam.publish
	c.broadcastBackoffMin, c.broadcastBackoffMax = time.Millisecond, 4*time.Millisecond
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start with GRAPH refusing the broadcasts = %v, want nil", err)
	}
	return c, seam, tc
}

// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting
func TestIntegrationRefusedBroadcastsDoNotStopStartAndLandOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, seam, tc := startRefusedManifest(ctx, t, nil)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, c.Stop) })
	seam.awaitRefused(ctx, t, manifestSubject, 2)
	seam.awaitRefused(ctx, t, predicatesSubject, 2)

	// The surfaces answer from memory while both broadcasts are pending.
	var manifest ManifestPayload
	requestJSON(t, ctx, tc.Client, querySubject, &manifest)
	if manifest.Namespace != "acme" || len(manifest.Sources) != 1 || manifest.Sources[0].Type != "ast" {
		t.Fatalf("graph.query.sources while the broadcast is refused = %+v", manifest)
	}
	var predicates PredicateSchemaPayload
	requestJSON(t, ctx, tc.Client, predicatesQuerySubject, &predicates)
	if len(predicates.Sources) != 1 || predicates.Sources[0].SourceType != "ast" {
		t.Fatalf("graph.query.predicates while the broadcast is refused = %+v", predicates.Sources)
	}
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("/source-manifest", mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/source-manifest/sources", nil))
	var served ManifestPayload
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &served) != nil || len(served.Sources) != 1 {
		t.Fatalf("GET /source-manifest/sources while the broadcast is refused = %d %s", rec.Code, rec.Body.String())
	}

	seam.awaitDelivered(ctx, t)
	// Stop joins the retry workers, so no later delivery can be missed.
	if err := stopWithin(5*time.Second, c.Stop); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	for _, subject := range []string{manifestSubject, predicatesSubject} {
		if got := seam.deliveries(subject); got != 1 {
			t.Errorf("%s delivered %d times, want exactly 1", subject, got)
		}
	}
}

// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting
func TestIntegrationRefusedBroadcastLogsOnceOnEntryAndOnceOnDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logs := &recordingHandler{}
	c, seam, _ := startRefusedManifest(ctx, t, slog.New(logs))
	t.Cleanup(func() { _ = stopWithin(5*time.Second, c.Stop) })
	seam.awaitRefused(ctx, t, manifestSubject, 3)
	seam.awaitDelivered(ctx, t)
	if err := stopWithin(5*time.Second, c.Stop); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for _, what := range []string{"source manifest", "predicate schema"} {
		if got := logs.count(slog.LevelWarn, what+" broadcast refused — retrying in the background"); got != 1 {
			t.Errorf("%s refusal warnings = %d over repeated refusals, want 1", what, got)
		}
		if got := logs.count(slog.LevelInfo, what+" broadcast delivered after retry"); got != 1 {
			t.Errorf("%s delivery lines = %d, want 1", what, got)
		}
	}
}

// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting
func TestIntegrationStopEndsABroadcastStillRetrying(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, seam, _ := startRefusedManifest(ctx, t, nil)
	seam.awaitRefused(ctx, t, manifestSubject, 2)
	seam.awaitRefused(ctx, t, predicatesSubject, 2)

	// The seam still refuses, so only Stop can end the retries.
	if err := stopWithin(5*time.Second, c.Stop); err != nil {
		t.Fatalf("Stop while broadcasts are retrying = %v, want nil", err)
	}
	joinCtx, joinCancel := context.WithTimeout(ctx, 5*time.Second)
	defer joinCancel()
	if err := c.workers.Wait(joinCtx); err != nil {
		t.Fatalf("retry workers still running after Stop: %v", err)
	}
	if got := seam.deliveries(manifestSubject) + seam.deliveries(predicatesSubject); got != 0 {
		t.Fatalf("deliveries with the seam refusing = %d, want 0", got)
	}
}

// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting
func TestIntegrationPhaseIsReadyWhileTheBroadcastIsUndelivered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, seam, tc := startRefusedManifest(ctx, t, nil)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, c.Stop) })
	seam.awaitRefused(ctx, t, manifestSubject, 2)

	report, err := json.Marshal(SourceStatusReport{
		InstanceName: "ast-source-repo",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  42,
		Timestamp:    time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	// Applied directly so the status read below needs no polling.
	c.handleStatusReport(ctx, report)

	var status StatusPayload
	requestJSON(t, ctx, tc.Client, statusQuerySubject, &status)
	if status.Phase != PhaseReady {
		t.Fatalf("phase with every source seeded and the manifest undelivered = %q, want %q", status.Phase, PhaseReady)
	}
	if got := seam.deliveries(manifestSubject); got != 0 {
		t.Fatalf("manifest delivered %d times before the test accepted, want 0", got)
	}
}
