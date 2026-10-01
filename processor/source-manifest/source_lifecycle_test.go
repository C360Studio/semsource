package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/types"
)

type receiptJournal struct {
	sourceintent.Journal
	record   *sourceintent.Record
	revision uint64
	listErr  error
}

func (j *receiptJournal) Get(_ context.Context, handle string) (sourceintent.Record, uint64, error) {
	if j.record == nil || j.record.Binding.Handle != handle {
		return sourceintent.Record{}, 0, sourceintent.ErrNotFound
	}
	return *j.record, j.revision, nil
}
func (j *receiptJournal) Create(_ context.Context, r sourceintent.Record) (uint64, error) {
	if j.record != nil {
		return 0, sourceintent.ErrConflict
	}
	j.record = &r
	j.revision++
	return j.revision, nil
}
func (j *receiptJournal) Update(_ context.Context, r sourceintent.Record, rev uint64) (uint64, error) {
	if j.revision != rev {
		return 0, sourceintent.ErrConflict
	}
	j.record = &r
	j.revision++
	return j.revision, nil
}
func (j *receiptJournal) List(context.Context) ([]sourceintent.Record, error) {
	if j.listErr != nil {
		return nil, j.listErr
	}
	if j.record == nil {
		return nil, nil
	}
	return []sourceintent.Record{*j.record}, nil
}

type receiptStore struct {
	*desiredStore
	durable    types.ComponentConfig
	failMemory bool
}

func (s *receiptStore) PutComponentToKV(ctx context.Context, handle string, cc types.ComponentConfig) error {
	if handle == "docs" {
		s.durable = cc
		if s.failMemory {
			return errors.New("committed durable config; memory apply failed")
		}
	}
	return s.desiredStore.PutComponentToKV(ctx, handle, cc)
}

type receiptTail struct{}

func (receiptTail) Settled(_ context.Context, s sourceintent.SourceScope) (sourceintent.TailEvidence, error) {
	return sourceintent.TailEvidence{Proof: sourceintent.TailProofCurrentRetained, Inputs: []sourceintent.InputEvidence{{Input: s.Inputs[0]}}}, nil
}
func lifecycleReceiptFixture(t *testing.T) (*IngestHandlerConfig, *receiptStore, *receiptJournal) {
	t.Helper()
	cc := types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"paths":["/docs"],"project":"manuals","secret":"never-serve-retired-config"}`)}
	store := &receiptStore{desiredStore: newDesiredStore(t), durable: cc}
	store.components["docs"] = cc
	j := &receiptJournal{}
	owner, err := sourcelifecycle.NewCoordinator(sourcelifecycle.CoordinatorConfig{Authority: entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}, Namespace: "acme", Journal: j, Store: store, Tail: receiptTail{}, Projector: sourcelifecycle.NATSProjector{}, Inputs: []sourceintent.StreamInput{{Stream: "GRAPH", Consumer: "ingest", Filters: []string{"graph.ingest.entity"}}}, ReadDesired: func(_ context.Context, handle string) (types.ComponentConfig, bool, error) {
		return store.durable, handle == "docs", nil
	}, RepairManifest: func(context.Context, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return &IngestHandlerConfig{Namespace: "acme", Store: store, Spawn: sourcespawn.Options{Org: "acme"}, Lifecycle: owner}, store, j
}
func TestLifecycleRemovalReceiptReportsDurablePartialCommit(t *testing.T) {
	cfg, store, j := lifecycleReceiptFixture(t)
	store.failMemory = true
	c := &Component{logger: httpTestLogger()}
	reply := c.removeSource(context.Background(), "docs", "tester", *cfg)
	if reply.Error == nil || !reply.Removed || !reply.DesiredChanged || !reply.RestartRequired || reply.RuntimeChanged || reply.Generation != 1 {
		t.Fatalf("partial receipt: %+v", reply)
	}
	if !store.components["docs"].Enabled || store.durable.Enabled || j.record.Phase != sourceintent.Prepared {
		t.Fatal("test failed to hold ambiguous memory boundary")
	}
	store.failMemory = false
	if err := cfg.Lifecycle.RepairDesired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if j.record.Phase != sourceintent.Pending {
		t.Fatal("prepared durable disable not repaired")
	}
	reply = c.removeSource(context.Background(), "docs", "tester", *cfg)
	if reply.Error != nil || reply.Generation != 1 || reply.ProjectionPhase != "pending" {
		t.Fatalf("retry changed intent identity: %+v", reply)
	}
}
func TestLifecycleHTTPReportsTypedPendingWithoutConfiguration(t *testing.T) {
	cfg, _, j := lifecycleReceiptFixture(t)
	c := &Component{logger: httpTestLogger()}
	if reply := c.removeSource(context.Background(), "docs", "tester", *cfg); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	j.record.Progress.Blocker = &sourceintent.Blocker{Code: sourceintent.CodeAppliedTailUnproven, Message: "applied barrier unavailable", Retryable: true}
	cfg.APIToken = "token"
	mux := newHTTPComponent(t, cfg, nil)
	path := "/source-manifest/sources/docs/lifecycle"
	if got := doJSON(t, mux, http.MethodGet, path, nil, nil); got.Code != http.StatusUnauthorized {
		t.Fatal(got.Code)
	}
	header := map[string]string{"Authorization": "Bearer token"}
	got := doJSON(t, mux, http.MethodGet, path, nil, header)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "applied_tail_unproven") || strings.Contains(got.Body.String(), "never-serve") || strings.Contains(got.Body.String(), "retired_config") {
		t.Fatalf("status: %d %s", got.Code, got.Body.String())
	}
	if got := doJSON(t, mux, http.MethodGet, "/source-manifest/sources/missing/lifecycle", nil, header); got.Code != http.StatusNotFound {
		t.Fatal(got.Code)
	}
	j.listErr = errors.New("journal unavailable")
	if got := doJSON(t, mux, http.MethodGet, path, nil, header); got.Code != http.StatusServiceUnavailable {
		t.Fatal(got.Code)
	}
	cfg.Lifecycle = nil
	if got := doJSON(t, mux, http.MethodGet, path, nil, header); got.Code != http.StatusServiceUnavailable {
		t.Fatal(got.Code)
	}
}

func TestUnresolvedEffectRefusesAddBeforeAnyConfigWrite(t *testing.T) {
	cfg, store, j := lifecycleReceiptFixture(t)
	c := &Component{logger: httpTestLogger()}
	ctx := context.Background()
	removed := c.removeSource(ctx, "docs", "tester", *cfg)
	if removed.Error != nil {
		t.Fatal(removed.Error)
	}
	j.record.Effect = &sourceintent.EffectAttempt{ID: "pending-attempt", Binding: j.record.Binding, EntityID: j.record.Binding.Authority.Build("manuals", "web", "doc", "a"), Outcome: sourceintent.EffectUnknown}
	before, _ := json.Marshal(store.components)
	reply := c.addSource(ctx, AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/new"}}}, *cfg)
	after, _ := json.Marshal(store.components)
	if reply.Error == nil || reply.DesiredChanged || reply.RestartRequired || len(reply.Components) != 0 || string(before) != string(after) {
		t.Fatalf("unknown effect allowed desired write: %+v", reply)
	}
	j.record.Effect.Outcome = sourceintent.EffectVerified
	// A verified authoritative terminal attempt releases the global admission
	// fence. Unrelated source adds need no reactivation journal history.
	reply = c.addSource(ctx, AddRequest{Source: config.SourceEntry{Type: "url", URLs: []string{"https://example.com/new"}}}, *cfg)
	if reply.Error != nil || !reply.DesiredChanged || len(reply.Components) != 1 {
		t.Fatalf("verified evidence did not release admission: %+v", reply)
	}
}
