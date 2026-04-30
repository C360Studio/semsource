package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/types"
)

// stubStore satisfies sourcespawn.ConfigStore for tests that exercise the
// ingest handler's pre-flight checks without writing to KV.
type stubStore struct{}

func (stubStore) PutComponentToKV(_ context.Context, _ string, _ types.ComponentConfig) error {
	return nil
}
func (stubStore) DeleteComponentFromKV(_ context.Context, _ string) error { return nil }

func TestMapSpawnError_AllCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spawnCode sourcespawn.ErrorCode
		want      IngestErrorCode
	}{
		{sourcespawn.CodeValidationFailed, CodeValidationFailed},
		{sourcespawn.CodeInstanceExists, CodeInstanceExists},
		{sourcespawn.CodeKVWriteFailed, CodeKVWriteFailed},
		{sourcespawn.CodeUnsupportedType, CodeUnsupportedType},
	}
	for _, tc := range cases {
		t.Run(string(tc.spawnCode), func(t *testing.T) {
			t.Parallel()
			err := &sourcespawn.Error{Code: tc.spawnCode, Message: "test"}
			got := mapSpawnError(err)
			if got.Code != tc.want {
				t.Errorf("mapSpawnError(%q).Code = %q, want %q", tc.spawnCode, got.Code, tc.want)
			}
		})
	}
}

func TestMapSpawnError_UnknownError_FallsBackToInternal(t *testing.T) {
	t.Parallel()
	got := mapSpawnError(errors.New("bare error"))
	if got.Code != CodeInternalError {
		t.Errorf("fallback Code = %q, want %q", got.Code, CodeInternalError)
	}
}

func TestAddRequest_RoundTrip(t *testing.T) {
	t.Parallel()
	original := AddRequest{
		Source: config.SourceEntry{
			Type: "url",
			URLs: []string{"https://example.com"},
		},
		Provenance: Provenance{
			Actor:      "semteams.research-agent",
			OnBehalfOf: "user-123",
			TraceID:    "trace-abc",
		},
	}
	raw, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AddRequest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Source.Type != original.Source.Type {
		t.Errorf("Source.Type = %q, want %q", decoded.Source.Type, original.Source.Type)
	}
	if decoded.Provenance.Actor != original.Provenance.Actor {
		t.Errorf("Provenance.Actor = %q, want %q", decoded.Provenance.Actor, original.Provenance.Actor)
	}
}

func TestAddReply_ErrorEnvelope(t *testing.T) {
	t.Parallel()
	reply := AddReply{
		Error: &IngestError{
			Code:    CodeValidationFailed,
			Message: "url is required",
		},
	}
	raw, err := json.Marshal(&reply)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AddReply
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Error == nil {
		t.Fatal("Error nil after roundtrip")
	}
	if decoded.Error.Code != CodeValidationFailed {
		t.Errorf("Error.Code = %q, want %q", decoded.Error.Code, CodeValidationFailed)
	}
}

func TestSourceStatus_LastError_RoundTrip(t *testing.T) {
	t.Parallel()
	status := SourceStatus{
		InstanceName: "url-source-example-com",
		SourceType:   "url",
		Phase:        "errored",
		LastError: &SourceError{
			Code:    SourceUnreachable,
			Message: "connection refused",
		},
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SourceStatus
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.LastError == nil {
		t.Fatal("LastError nil after roundtrip")
	}
	if decoded.LastError.Code != SourceUnreachable {
		t.Errorf("LastError.Code = %q, want %q", decoded.LastError.Code, SourceUnreachable)
	}
}

func TestSourceStatus_NoLastError_OmittedFromJSON(t *testing.T) {
	t.Parallel()
	status := SourceStatus{
		InstanceName: "url-source-example-com",
		SourceType:   "url",
		Phase:        "watching",
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) == "" || containsLastError(raw) {
		t.Errorf("raw should omit last_error, got %s", raw)
	}
}

// TestAppendManifestSources_Idempotent guards H1: programmatic Adds mutate
// c.manifestSources under lock; a duplicate add for the same logical source
// must not produce two entries.
func TestAppendManifestSources_Idempotent(t *testing.T) {
	t.Parallel()
	c := &Component{name: "source-manifest", logger: slog.Default()}
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}

	c.appendManifestSources(src)
	c.appendManifestSources(src)

	c.manifestMu.RLock()
	defer c.manifestMu.RUnlock()
	if len(c.manifestSources) != 1 {
		t.Errorf("manifestSources len = %d, want 1 (idempotent dup-add)", len(c.manifestSources))
	}
	if c.manifestSources[0].Type != "url" || len(c.manifestSources[0].URLs) != 1 || c.manifestSources[0].URLs[0] != "https://example.com" {
		t.Errorf("manifestSources entry = %+v; not the URL source we appended", c.manifestSources[0])
	}
}

// TestRemoveManifestSourceByInstance_FindsViaBuild guards the H1 remove path:
// given an instance name the caller got back from Add, the manifest entry
// that produced it must be located via sourcespawn.Build and dropped.
func TestRemoveManifestSourceByInstance_FindsViaBuild(t *testing.T) {
	t.Parallel()
	c := &Component{name: "source-manifest", logger: slog.Default()}
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}
	c.appendManifestSources(src)

	// Compute the instance name the same way the runtime would.
	built, err := sourcespawn.Build(src, sourcespawn.Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var instanceName string
	for k := range built {
		instanceName = k
	}
	if instanceName == "" {
		t.Fatal("Build returned no instance name")
	}

	if !c.removeManifestSourceByInstance(instanceName, sourcespawn.Options{Org: "acme"}) {
		t.Fatal("removeManifestSourceByInstance returned false; want true")
	}
	c.manifestMu.RLock()
	defer c.manifestMu.RUnlock()
	if len(c.manifestSources) != 0 {
		t.Errorf("manifestSources len = %d after remove; want 0", len(c.manifestSources))
	}
}

// TestRegisterIngestHandlers_RefusesUnstartedComponent guards C1: a component
// that has not yet Started must reject RegisterIngestHandlers, otherwise
// subscriptions get appended to c.ingestSubs but Stop's early return on
// !running leaks them.
func TestRegisterIngestHandlers_RefusesUnstartedComponent(t *testing.T) {
	t.Parallel()
	c := &Component{
		name:   "source-manifest",
		logger: slog.Default(),
		// running stays false; client stays nil — guard runs before either matters
	}
	err := c.RegisterIngestHandlers(context.Background(), IngestHandlerConfig{
		Namespace: "acme",
		Store:     stubStore{},
	})
	if err == nil {
		t.Fatal("RegisterIngestHandlers on unstarted component returned nil; want error")
	}
}

// TestRegisterIngestHandlers_RejectsEmptyNamespace exercises the cfg-validation
// preflight check.
func TestRegisterIngestHandlers_RejectsEmptyNamespace(t *testing.T) {
	t.Parallel()
	c := &Component{name: "source-manifest", logger: slog.Default()}
	err := c.RegisterIngestHandlers(context.Background(), IngestHandlerConfig{
		Store: stubStore{},
	})
	if err == nil {
		t.Fatal("RegisterIngestHandlers with empty namespace returned nil; want error")
	}
}

// TestRegisterIngestHandlers_RejectsNilStore exercises the cfg-validation
// preflight check.
func TestRegisterIngestHandlers_RejectsNilStore(t *testing.T) {
	t.Parallel()
	c := &Component{name: "source-manifest", logger: slog.Default()}
	err := c.RegisterIngestHandlers(context.Background(), IngestHandlerConfig{
		Namespace: "acme",
	})
	if err == nil {
		t.Fatal("RegisterIngestHandlers with nil store returned nil; want error")
	}
}

func containsLastError(raw []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m["last_error"]
	return ok
}
