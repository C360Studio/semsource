package objectstoresource

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/statustest"
	"github.com/c360studio/semsource/storage/s3store"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testDependencies is what the framework hands a component at construction.
// A nil NATS client is enough here: nothing in these tests publishes, and the
// entity publisher only dials when it starts.
func testDependencies(t *testing.T) component.Dependencies {
	t.Helper()
	return component.Dependencies{Logger: discardLogger()}
}

// ─── Configuration ──────────────────────────────────────────────────────────

func TestConfig_Validate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"no bucket", Config{Org: "acme"}, "bucket"},
		{"no org", Config{Bucket: "artifacts"}, "org"},
		{"unparseable endpoint", Config{Bucket: "artifacts", Org: "acme", Endpoint: "localhost:9000"}, "endpoint"},
		{"negative poll interval", Config{Bucket: "artifacts", Org: "acme", PollIntervalMs: -1}, "poll_interval_ms"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatalf("expected an error naming %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error should name %q, got: %v", tc.wantErr, err)
			}
		})
	}

	valid := Config{Bucket: "artifacts", Org: "acme", Endpoint: "https://garage.internal:3900", PathStyle: true}
	if err := valid.Validate(); err != nil {
		t.Errorf("a complete config should validate, got: %v", err)
	}
	if valid.PollInterval() != DefaultPollInterval {
		t.Errorf("PollInterval = %v, want the default", valid.PollInterval())
	}
}

// TestConfig_CarriesNoCredentialFields is the structural half of the
// credentials-never-in-configuration rule. This document is watched and
// replicated through KV, so an access key on it would be distributed well
// beyond the process that needs it — strict decoding rejects one because the
// struct has nowhere to put it.
func TestConfig_CarriesNoCredentialFields(t *testing.T) {
	for _, field := range []string{"access_key_id", "secret_access_key", "aws_secret_access_key", "credentials"} {
		t.Run(field, func(t *testing.T) {
			dec := json.NewDecoder(strings.NewReader(
				`{"bucket":"artifacts","org":"acme","` + field + `":"AKIAIOSFODNN7EXAMPLE"}`))
			dec.DisallowUnknownFields()

			var cfg Config
			if err := dec.Decode(&cfg); err == nil {
				t.Fatalf("strict decoding accepted a %q field", field)
			}
		})
	}
}

// ─── Construction and discovery ─────────────────────────────────────────────

func newTestComponent(t *testing.T) *Component {
	t.Helper()

	t.Setenv(s3store.EnvAccessKeyID, "AKIAIOSFODNN7EXAMPLE")
	t.Setenv(s3store.EnvSecretAccessKey, "wJalrXUtnFEMI-K7MDENG-bPxRfiCYEXAMPLEKEY")

	// Hand-written rather than marshaled from a Config: a declared "ports"
	// section replaces the defaults wholesale, so a struct with a nil Ports
	// would encode "ports":null and take the graph.ingest port away — which is
	// framework behavior worth not tripping over in a fixture.
	raw := json.RawMessage(`{
		"bucket": "artifacts",
		"prefix": "reports/",
		"endpoint": "http://127.0.0.1:9000",
		"region": "us-east-1",
		"path_style": true,
		"org": "acme",
		"watch_enabled": true,
		"body_store_root": ` + strconv.Quote(t.TempDir()) + `,
		"instance_name": "objectstore-1"
	}`)

	discoverable, err := NewComponent(raw, testDependencies(t))
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c, isComponent := discoverable.(*Component)
	if !isComponent {
		t.Fatalf("NewComponent returned %T", discoverable)
	}
	return c
}

func TestNewComponent_IsDiscoverable(t *testing.T) {
	c := newTestComponent(t)

	meta := c.Meta()
	if meta.Name != "objectstore-source" {
		t.Errorf("Meta().Name = %q", meta.Name)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta().Type = %q", meta.Type)
	}
	if len(c.InputPorts()) != 0 {
		t.Error("an object-store source consumes no input ports")
	}
	outputs := c.OutputPorts()
	if len(outputs) != 1 || outputs[0].Name != "graph.ingest" {
		t.Errorf("OutputPorts = %+v, want one graph.ingest port", outputs)
	}
	if c.ConfigSchema().Properties == nil {
		t.Error("ConfigSchema carries no properties")
	}
	if health := c.Health(); health.Healthy {
		t.Error("a component that has not started is not healthy")
	}
}

// TestNewComponent_ResolvesCredentialsFromTheEnvironment pins where the secret
// comes from: construction fails when the environment holds none, and the
// configuration document never carries one.
func TestNewComponent_ResolvesCredentialsFromTheEnvironment(t *testing.T) {
	for _, name := range []string{
		s3store.EnvAccessKeyID, s3store.EnvSecretAccessKey,
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	} {
		t.Setenv(name, "")
	}

	raw := json.RawMessage(`{"bucket":"artifacts","org":"acme"}`)

	if _, err := NewComponent(raw, testDependencies(t)); err == nil {
		t.Fatal("expected construction to fail with no credentials in the environment")
	}
}

func TestRegister_RejectsNilRegistry(t *testing.T) {
	if err := Register(nil); err == nil {
		t.Error("expected an error registering against a nil registry")
	}
}

// TestSourceIntroducesNoPayloadType asserts what task 4.5 assumes rather than
// leaving it assumed: object-store artifacts become ordinary document
// entities, so the payload registry is untouched by this source.
func TestSourceIntroducesNoPayloadType(t *testing.T) {
	runGo, err := os.ReadFile("../../cmd/semsource/run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}

	registry := string(runGo)
	start := strings.Index(registry, "func buildPayloadRegistry()")
	if start < 0 {
		t.Fatal("buildPayloadRegistry not found in run.go")
	}
	end := strings.Index(registry[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not delimit buildPayloadRegistry")
	}

	if body := registry[start : start+end]; strings.Contains(body, "objectstore") {
		t.Errorf("buildPayloadRegistry mentions the object-store source:\n%s", body)
	}
}

// ─── Typed publishing ───────────────────────────────────────────────────────

func TestHandleChangeEvent_MissingTypedStateRecordsContractError(t *testing.T) {
	c := &Component{logger: discardLogger()}

	c.handleChangeEvent(context.Background(), handler.ChangeEvent{
		Path:      "reports/q3.md",
		Operation: handler.OperationCreate,
	})

	if got := c.ingestErrors.Load(); got != 1 {
		t.Fatalf("ingestErrors = %d, want 1", got)
	}
	if got := c.entitiesPublished.Load(); got != 0 {
		t.Fatalf("a contract failure published %d entities, want 0", got)
	}
}

func TestHandleChangeEvent_NilStateRecordsErrorWithoutPublishing(t *testing.T) {
	c := &Component{logger: discardLogger()}

	c.handleChangeEvent(context.Background(), handler.ChangeEvent{
		Path:         "reports/q3.md",
		Operation:    handler.OperationModify,
		EntityStates: []*handler.EntityState{nil},
	})

	if got := c.ingestErrors.Load(); got != 1 {
		t.Fatalf("ingestErrors = %d, want 1", got)
	}
	if got := c.entitiesPublished.Load(); got != 0 {
		t.Fatalf("published %d entities from a nil state", got)
	}
}

func TestPublishDocument_EmptyStatesIsAContractFailure(t *testing.T) {
	c := &Component{logger: discardLogger()}

	c.publishDocument(context.Background(), "reports/q3.md", nil)

	if got := c.ingestErrors.Load(); got != 1 {
		t.Fatalf("ingestErrors = %d, want 1", got)
	}
}

// ─── Retraction ─────────────────────────────────────────────────────────────

// TestRetractionRequest_UsesTheAbsentSetNotAFilesystemRoot is the shape that
// makes object-key removal work at all. RootPath anchors a stat check, and an
// object key is not a file; an empty RootPath means "mark everything in
// scope", which is the source-removed shape, not one document's.
func TestRetractionRequest_UsesTheAbsentSetNotAFilesystemRoot(t *testing.T) {
	c := &Component{system: "artifacts"}
	c.config.Org = "acme"
	c.config.Bucket = "artifacts"

	req, hasWork := c.retractionRequest([]objectstore.Removal{
		{Key: "reports/q4.md", EntityID: "acme.semsource.web.artifacts.doc.reports-q4-md"},
		{Key: "reports/old.md", EntityID: "acme.semsource.web.artifacts.doc.reports-old-md"},
	})
	if !hasWork {
		t.Fatal("expected a request")
	}

	if req.RootPath != "" {
		t.Errorf("RootPath = %q — an object key cannot be stat-ed", req.RootPath)
	}
	if len(req.Absent) != 2 {
		t.Fatalf("Absent = %v, want both keys", req.Absent)
	}
	if req.Org != "acme" {
		t.Errorf("Org = %q", req.Org)
	}
	// Scoped by the same system slug the ingest path published under, or the
	// pass matches nothing.
	if len(req.Systems) != 1 || req.Systems[0] != "artifacts" {
		t.Errorf("Systems = %v, want the publishing system slug", req.Systems)
	}
	if req.Reason == "" {
		t.Error("a staleness marker with no reason says nothing about why")
	}
}

// TestRetractionRequest_NothingRemovedSendsNothing keeps this source from
// asserting a liveness claim it did not make. An empty absent set says
// "nothing is gone" and clears markers; no request at all says nothing.
func TestRetractionRequest_NothingRemovedSendsNothing(t *testing.T) {
	c := &Component{system: "artifacts"}
	c.config.Org = "acme"

	if _, hasWork := c.retractionRequest(nil); hasWork {
		t.Error("a pass with no removals must send no lifecycle request")
	}
	if _, hasWork := c.retractionRequest([]objectstore.Removal{}); hasWork {
		t.Error("a pass with no removals must send no lifecycle request")
	}
}

// TestRetractionRequest_SurvivesTheWire pairs with the field's missing
// omitempty: the absent set has to still be an absent set when supersession
// decodes it, or the request degrades into "mark every entity in scope".
func TestRetractionRequest_SurvivesTheWire(t *testing.T) {
	c := &Component{system: "artifacts"}
	c.config.Org = "acme"

	req, _ := c.retractionRequest([]objectstore.Removal{{Key: "reports/q4.md"}})

	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded graph.LifecycleRunRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Absent == nil {
		t.Fatalf("the absent set did not survive the wire: %s", encoded)
	}
	if decoded.RootPath != "" {
		t.Errorf("RootPath came back as %q", decoded.RootPath)
	}
}

// ─── Status ─────────────────────────────────────────────────────────────────

func TestBuildStatusReport_CarriesTheSharedContract(t *testing.T) {
	pub, accepted := statustest.LossyPublisher(t, 2)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}
	c.config.InstanceName = "objectstore-1"
	c.entitiesPublished.Store(accepted)

	r := c.buildStatusReport("watching")

	if r.LostTotal == 0 {
		t.Fatal("harness produced no loss; the assertions below would prove nothing")
	}
	if r.InstanceName != "objectstore-1" {
		t.Errorf("InstanceName = %q", r.InstanceName)
	}
	if r.SourceType != "s3" {
		t.Errorf("SourceType = %q", r.SourceType)
	}
	if r.Phase != "watching" {
		t.Errorf("Phase = %q", r.Phase)
	}
	// Acceptance is not arrival: delivered and lost come from the publisher's
	// confirmed counters, never from this source's hand-off counter.
	if r.DeliveredTotal != pub.Published() {
		t.Errorf("DeliveredTotal = %d, want %d (publisher confirmed)", r.DeliveredTotal, pub.Published())
	}
	if r.LostTotal != pub.Lost() {
		t.Errorf("LostTotal = %d, want %d", r.LostTotal, pub.Lost())
	}
	if r.OfferedTotal != r.DeliveredTotal+r.LostTotal {
		t.Errorf("figures do not reconcile: offered %d != delivered %d + lost %d",
			r.OfferedTotal, r.DeliveredTotal, r.LostTotal)
	}
	if r.Backpressure != pub.InBackpressure() {
		t.Error("Backpressure is not read from the publisher")
	}
}

// TestBuildStatusReport_SkipCountsAreVisible is what makes "no such document"
// distinguishable from "that document was never parsed" without a
// source-specific query.
func TestBuildStatusReport_SkipCountsAreVisible(t *testing.T) {
	pub, _ := statustest.LossyPublisher(t, 1)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}

	c.recordSkips(&objectstore.Result{Skipped: []objectstore.Skip{
		{Key: "reports/a.png", Reason: objectstore.SkipUnsupportedFormat},
		{Key: "reports/b.pdf", Reason: objectstore.SkipUnsupportedFormat},
		{Key: "reports/c.md", Reason: objectstore.SkipEmptyObject},
	}})

	r := c.buildStatusReport("watching")

	if r.ObjectsSkipped["unsupported_format"] != 2 {
		t.Errorf("ObjectsSkipped = %v, want two unsupported_format", r.ObjectsSkipped)
	}
	if r.ObjectsSkipped["empty_object"] != 1 {
		t.Errorf("ObjectsSkipped = %v, want one empty_object", r.ObjectsSkipped)
	}

	// The figures describe the LAST COMPLETED PASS. A skip repeats every pass
	// by nature, so a lifetime total would climb forever while describing the
	// same few objects.
	c.recordSkips(&objectstore.Result{Skipped: []objectstore.Skip{
		{Key: "reports/a.png", Reason: objectstore.SkipUnsupportedFormat},
	}})
	if got := c.buildStatusReport("watching").ObjectsSkipped["unsupported_format"]; got != 1 {
		t.Errorf("ObjectsSkipped accumulated across passes: %d, want 1", got)
	}

	// A pass that skipped nothing reports nothing rather than zeros.
	c.recordSkips(&objectstore.Result{})
	if got := c.buildStatusReport("watching").ObjectsSkipped; got != nil {
		t.Errorf("ObjectsSkipped = %v, want nothing when nothing was skipped", got)
	}
}

func TestBuildStatusReport_IsACopy(t *testing.T) {
	pub, _ := statustest.LossyPublisher(t, 1)
	c := &Component{publisher: pub, distinct: entitypub.NewDistinctTracker()}
	c.recordSkips(&objectstore.Result{Skipped: []objectstore.Skip{
		{Key: "reports/a.png", Reason: objectstore.SkipUnsupportedFormat},
	}})

	r := c.buildStatusReport("watching")
	r.ObjectsSkipped["unsupported_format"] = 99

	if got := c.buildStatusReport("watching").ObjectsSkipped["unsupported_format"]; got != 1 {
		t.Errorf("mutating a published report changed the component's counts: %d", got)
	}
}

// failingStore is an object store whose listing always fails, standing in for
// an endpoint that is down or a credential that stopped working.
type failingStore struct{ err error }

func (f failingStore) Objects(context.Context, string) ([]s3store.ObjectInfo, error) {
	return nil, f.err
}

func (f failingStore) Get(context.Context, string) ([]byte, error) {
	return nil, f.err
}

// TestIngestOnce_UnreachableBucketSurfacesWithoutRetracting covers the failure
// an operator is most likely to hit and least likely to notice.
//
// A listing that fails must do three things and no others: report itself, keep
// the source running, and retract nothing. The third is the one with teeth —
// an unreachable bucket and a legitimately emptied one look identical from the
// outside, and treating the first as the second retracts an entire corpus.
func TestIngestOnce_UnreachableBucketSurfacesWithoutRetracting(t *testing.T) {
	store := failingStore{err: errors.New("dial tcp 10.0.0.1:3900: connect: connection refused")}

	pub, _ := statustest.LossyPublisher(t, 1)
	c := &Component{
		logger:    discardLogger(),
		publisher: pub,
		distinct:  entitypub.NewDistinctTracker(),
		handler:   objectstore.New(store, dochandler.New(), "acme"),
		system:    "artifacts",
		sourceCfg: &sourceCfg{url: objectstore.SourceURL("artifacts", "reports/")},
	}
	c.config.Org = "acme"
	c.config.Bucket = "artifacts"
	c.config.InstanceName = "objectstore-1"

	// The source had ingested two documents before the store became
	// unreachable, so there is something a mistaken retraction could destroy.
	c.handler.Record("reports/q3.md", "etag-a")
	c.handler.Record("reports/q4.md", "etag-b")

	err := c.ingestOnce(context.Background())
	if err == nil {
		t.Fatal("a failed listing must be reported as an error, not absorbed")
	}
	// The cause reaches the operator, along with the bucket it came from.
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("the error should carry the cause, got: %v", err)
	}

	// Nothing was retracted, and change detection still holds everything the
	// last successful pass established.
	if c.handler.Tracked() != 2 {
		t.Errorf("a failed pass discarded change-detection state: tracking %d keys", c.handler.Tracked())
	}
	if c.entitiesPublished.Load() != 0 {
		t.Errorf("a failed pass published %d entities", c.entitiesPublished.Load())
	}

	// And it is visible on the status surface rather than only in a log.
	report := c.buildStatusReport("watching")
	if report.ErrorCount == 0 {
		t.Error("the failed pass is invisible on the status surface")
	}
}

// TestIngestOnce_RecoveryClearsTheCondition pairs with it: the degraded
// condition is edge-triggered, so a store that comes back must clear rather
// than leave the source reading as broken forever.
func TestIngestOnce_RecoveryClearsTheCondition(t *testing.T) {
	pub, _ := statustest.LossyPublisher(t, 1)
	c := &Component{
		logger:    discardLogger(),
		publisher: pub,
		distinct:  entitypub.NewDistinctTracker(),
		handler:   objectstore.New(failingStore{err: errors.New("connection refused")}, dochandler.New(), "acme"),
		system:    "artifacts",
		sourceCfg: &sourceCfg{url: objectstore.SourceURL("artifacts", "reports/")},
	}
	c.config.Org = "acme"
	c.config.Bucket = "artifacts"

	if err := c.ingestOnce(context.Background()); err == nil {
		t.Fatal("expected the first pass to fail")
	}

	// The store comes back, holding nothing under the prefix — a real,
	// completed answer.
	c.handler = objectstore.New(emptyStore{}, dochandler.New(), "acme")

	if err := c.ingestOnce(context.Background()); err != nil {
		t.Fatalf("the recovered pass should succeed, got: %v", err)
	}
}

// emptyStore is a reachable store with nothing under the prefix.
type emptyStore struct{}

func (emptyStore) Objects(context.Context, string) ([]s3store.ObjectInfo, error) { return nil, nil }
func (emptyStore) Get(context.Context, string) ([]byte, error)                   { return nil, nil }
