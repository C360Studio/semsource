package sourcemanifest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
)

func TestNewComponent_ValidConfig(t *testing.T) {
	cfg := Config{
		Namespace: "acme",
		Sources: []ManifestSource{
			{Type: "ast", Path: "./src", Language: "go", Watch: true},
			{Type: "git", URL: "https://github.com/acme/app", Branch: "main"},
		},
		Ports: DefaultConfig().Ports,
	}
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	deps := component.Dependencies{}
	comp, err := NewComponent(rawCfg, deps)
	if err != nil {
		t.Fatalf("NewComponent failed: %v", err)
	}

	c := comp.(*Component)
	if c.config.Namespace != "acme" {
		t.Errorf("namespace = %q, want %q", c.config.Namespace, "acme")
	}
	if len(c.config.Sources) != 2 {
		t.Errorf("sources count = %d, want 2", len(c.config.Sources))
	}
}

func TestNewComponent_MissingNamespace(t *testing.T) {
	cfg := Config{Sources: []ManifestSource{{Type: "git", URL: "https://example.com"}}}
	rawCfg, _ := json.Marshal(cfg)

	_, err := NewComponent(rawCfg, component.Dependencies{})
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestNewComponent_InvalidJSON(t *testing.T) {
	_, err := NewComponent(json.RawMessage(`{invalid`), component.Dependencies{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid",
			config:  Config{Namespace: "acme"},
			wantErr: false,
		},
		{
			name:    "missing namespace",
			config:  Config{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManifestPayload_RoundTrip(t *testing.T) {
	payload := &ManifestPayload{
		Namespace: "acme",
		Sources: []ManifestSource{
			{Type: "ast", Path: "./src", Language: "go", Watch: true},
			{Type: "url", URLs: []string{"https://docs.acme.com"}, PollInterval: "300s"},
		},
		Timestamp: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ManifestPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Namespace != payload.Namespace {
		t.Errorf("namespace = %q, want %q", decoded.Namespace, payload.Namespace)
	}
	if len(decoded.Sources) != 2 {
		t.Fatalf("sources count = %d, want 2", len(decoded.Sources))
	}
	if decoded.Sources[0].Type != "ast" {
		t.Errorf("sources[0].type = %q, want %q", decoded.Sources[0].Type, "ast")
	}
	if decoded.Sources[0].Language != "go" {
		t.Errorf("sources[0].language = %q, want %q", decoded.Sources[0].Language, "go")
	}
	if decoded.Sources[1].URLs[0] != "https://docs.acme.com" {
		t.Errorf("sources[1].urls[0] = %q, want %q", decoded.Sources[1].URLs[0], "https://docs.acme.com")
	}
}

func TestManifestPayload_Schema(t *testing.T) {
	p := &ManifestPayload{}
	schema := p.Schema()
	if schema.Domain != "semsource" {
		t.Errorf("domain = %q, want %q", schema.Domain, "semsource")
	}
	if schema.Category != "manifest" {
		t.Errorf("category = %q, want %q", schema.Category, "manifest")
	}
	if schema.Version != "v1" {
		t.Errorf("version = %q, want %q", schema.Version, "v1")
	}
}

func TestComponent_Meta(t *testing.T) {
	cfg := Config{Namespace: "acme", Ports: DefaultConfig().Ports}
	rawCfg, _ := json.Marshal(cfg)
	comp, err := NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}

	meta := comp.Meta()
	if meta.Name != "source-manifest" {
		t.Errorf("meta.Name = %q, want %q", meta.Name, "source-manifest")
	}
	if meta.Type != "processor" {
		t.Errorf("meta.Type = %q, want %q", meta.Type, "processor")
	}
}

func TestComponent_Health_NotStarted(t *testing.T) {
	cfg := Config{Namespace: "acme", Ports: DefaultConfig().Ports}
	rawCfg, _ := json.Marshal(cfg)
	comp, err := NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}

	health := comp.(*Component).Health()
	if health.Healthy {
		t.Error("expected unhealthy when not started")
	}
	if health.Status != "stopped" {
		t.Errorf("status = %q, want %q", health.Status, "stopped")
	}
}

func TestComponent_OutputPorts(t *testing.T) {
	cfg := Config{Namespace: "acme", Ports: DefaultConfig().Ports}
	rawCfg, _ := json.Marshal(cfg)
	comp, err := NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}

	ports := comp.(*Component).OutputPorts()
	if len(ports) != 3 {
		t.Fatalf("output ports count = %d, want 3", len(ports))
	}
	if ports[0].Name != "graph.ingest" {
		t.Errorf("port name = %q, want %q", ports[0].Name, "graph.ingest")
	}
}

func TestHandleSources_GET(t *testing.T) {
	c := &Component{
		config: Config{Namespace: "acme"},
		responseData: mustMarshal(t, &ManifestPayload{
			Namespace: "acme",
			Sources: []ManifestSource{
				{Type: "ast", Path: "./src", Language: "go", Watch: true},
			},
			Timestamp: time.Now(),
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/source-manifest/sources", nil)
	rec := httptest.NewRecorder()
	c.handleSources(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var payload ManifestPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Namespace != "acme" {
		t.Errorf("namespace = %q, want %q", payload.Namespace, "acme")
	}
	if len(payload.Sources) != 1 {
		t.Fatalf("sources count = %d, want 1", len(payload.Sources))
	}
	if payload.Sources[0].Type != "ast" {
		t.Errorf("sources[0].type = %q, want %q", payload.Sources[0].Type, "ast")
	}
}

func TestHandleSources_MethodNotAllowed(t *testing.T) {
	c := &Component{responseData: []byte(`{}`)}

	req := httptest.NewRequest(http.MethodPost, "/source-manifest/sources", nil)
	rec := httptest.NewRecorder()
	c.handleSources(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRegisterHTTPHandlers(t *testing.T) {
	c := &Component{
		config:       Config{Namespace: "acme"},
		responseData: []byte(`{"namespace":"acme","sources":[]}`),
		logger:       newTestLogger(),
	}

	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("/source-manifest", mux)

	req := httptest.NewRequest(http.MethodGet, "/source-manifest/sources", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newTestLogger() *slog.Logger {
	return slog.Default()
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Ports == nil {
		t.Fatal("default config ports is nil")
	}
	if len(cfg.Ports.Outputs) != 3 {
		t.Fatalf("outputs count = %d, want 3", len(cfg.Ports.Outputs))
	}
	if cfg.Ports.Outputs[0].Subject != "graph.ingest.manifest" {
		t.Errorf("output subject = %q, want %q", cfg.Ports.Outputs[0].Subject, "graph.ingest.manifest")
	}
}

func TestStatusAggregator_KeysByInstanceName(t *testing.T) {
	agg := newStatusAggregator(3)

	agg.update(&SourceStatusReport{
		InstanceName: "ast-source-repo-a",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  100,
	})
	agg.update(&SourceStatusReport{
		InstanceName: "ast-source-repo-b",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  200,
	})
	agg.update(&SourceStatusReport{
		InstanceName: "git-source-repo-a",
		SourceType:   "git",
		Phase:        SourcePhaseWatching,
		EntityCount:  50,
	})

	// All 3 instances reported — should be ready.
	if !agg.allReported() {
		t.Error("allReported() = false, want true after 3 unique instances reported")
	}

	status := agg.buildStatus("acme")
	if status.Phase != PhaseReady {
		t.Errorf("phase = %q, want %q", status.Phase, PhaseReady)
	}
	if status.TotalEntities != 350 {
		t.Errorf("total_entities = %d, want 350", status.TotalEntities)
	}
	if len(status.Sources) != 3 {
		t.Errorf("sources count = %d, want 3", len(status.Sources))
	}

	// Verify instance names are preserved in output.
	instanceNames := map[string]bool{}
	for _, s := range status.Sources {
		instanceNames[s.InstanceName] = true
	}
	for _, want := range []string{"ast-source-repo-a", "ast-source-repo-b", "git-source-repo-a"} {
		if !instanceNames[want] {
			t.Errorf("missing instance %q in status sources", want)
		}
	}
}

func TestStatusAggregator_SameTypeDoesNotCollide(t *testing.T) {
	agg := newStatusAggregator(2)

	agg.update(&SourceStatusReport{
		InstanceName: "ast-source-alpha",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  500,
	})

	if agg.allReported() {
		t.Error("allReported() = true after 1 of 2 instances reported")
	}

	agg.update(&SourceStatusReport{
		InstanceName: "ast-source-beta",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  300,
	})

	if !agg.allReported() {
		t.Error("allReported() = false, want true after 2 unique instances reported")
	}

	status := agg.buildStatus("test")
	if status.TotalEntities != 800 {
		t.Errorf("total_entities = %d, want 800 (both instances counted)", status.TotalEntities)
	}
}

func TestStatusAggregator_PhaseTransitions(t *testing.T) {
	agg := newStatusAggregator(2)

	// Initially seeding.
	status := agg.buildStatus("ns")
	if status.Phase != PhaseSeeding {
		t.Errorf("initial phase = %q, want %q", status.Phase, PhaseSeeding)
	}

	// First instance reports ingesting.
	agg.update(&SourceStatusReport{
		InstanceName: "git-source-main",
		SourceType:   "git",
		Phase:        SourcePhaseIngesting,
		EntityCount:  0,
	})
	status = agg.buildStatus("ns")
	if status.Phase != PhaseSeeding {
		t.Errorf("after 1 of 2: phase = %q, want %q", status.Phase, PhaseSeeding)
	}

	// First instance transitions to watching.
	agg.update(&SourceStatusReport{
		InstanceName: "git-source-main",
		SourceType:   "git",
		Phase:        SourcePhaseWatching,
		EntityCount:  50,
	})
	status = agg.buildStatus("ns")
	if status.Phase != PhaseSeeding {
		t.Errorf("still 1 of 2: phase = %q, want %q", status.Phase, PhaseSeeding)
	}

	// Second instance reports watching.
	agg.update(&SourceStatusReport{
		InstanceName: "ast-source-main",
		SourceType:   "ast",
		Phase:        SourcePhaseWatching,
		EntityCount:  200,
	})
	status = agg.buildStatus("ns")
	if status.Phase != PhaseReady {
		t.Errorf("after all reported: phase = %q, want %q", status.Phase, PhaseReady)
	}
}

func TestStatusAggregator_MarkDegraded(t *testing.T) {
	agg := newStatusAggregator(3)

	agg.update(&SourceStatusReport{
		InstanceName: "git-source-a",
		SourceType:   "git",
		Phase:        SourcePhaseWatching,
		EntityCount:  10,
	})

	// Only 1 of 3 reported — normally still seeding.
	status := agg.buildStatus("ns")
	if status.Phase != PhaseSeeding {
		t.Errorf("before degraded: phase = %q, want %q", status.Phase, PhaseSeeding)
	}

	// Force degraded (simulating seed timeout).
	degraded := agg.markDegraded("ns")
	if degraded.Phase != PhaseDegraded {
		t.Errorf("degraded phase = %q, want %q", degraded.Phase, PhaseDegraded)
	}
	if len(degraded.Sources) != 1 {
		t.Errorf("degraded sources = %d, want 1", len(degraded.Sources))
	}
}

func TestStatusPayload_RoundTrip(t *testing.T) {
	payload := &StatusPayload{
		Namespace: "acme",
		Phase:     PhaseReady,
		Sources: []SourceStatus{
			{
				InstanceName: "ast-source-myrepo",
				SourceType:   "ast",
				Phase:        SourcePhaseWatching,
				EntityCount:  500,
				ErrorCount:   2,
			},
		},
		TotalEntities: 500,
		Timestamp:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded StatusPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Phase != PhaseReady {
		t.Errorf("phase = %q, want %q", decoded.Phase, PhaseReady)
	}
	if len(decoded.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(decoded.Sources))
	}
	src := decoded.Sources[0]
	if src.InstanceName != "ast-source-myrepo" {
		t.Errorf("instance_name = %q, want %q", src.InstanceName, "ast-source-myrepo")
	}
	if src.SourceType != "ast" {
		t.Errorf("source_type = %q, want %q", src.SourceType, "ast")
	}
	if src.EntityCount != 500 {
		t.Errorf("entity_count = %d, want 500", src.EntityCount)
	}
}

func TestStatusPayload_Schema(t *testing.T) {
	p := &StatusPayload{}
	schema := p.Schema()
	if schema.Domain != "semsource" {
		t.Errorf("domain = %q, want %q", schema.Domain, "semsource")
	}
	if schema.Category != "status" {
		t.Errorf("category = %q, want %q", schema.Category, "status")
	}
	if schema.Version != "v1" {
		t.Errorf("version = %q, want %q", schema.Version, "v1")
	}
}
