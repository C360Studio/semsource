package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

func inputConfig(t *testing.T, inputs ...component.PortDefinition) semconfig.ComponentConfigs {
	t.Helper()
	raw, err := json.Marshal(struct {
		Ports component.PortConfig `json:"ports"`
	}{component.PortConfig{Inputs: inputs}})
	if err != nil {
		t.Fatal(err)
	}
	return semconfig.ComponentConfigs{"graph-ingest": {Name: "graph-ingest", Enabled: true, Config: raw}}
}
func TestGraphInputsAdmittedConsumerIdentity(t *testing.T) {
	ports := []component.PortDefinition{{Name: "entity_stream", Config: component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.entity", "graph.ingest.filtered"}}}}
	got, err := GraphInputs(inputConfig(t, ports...))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Consumer != "graph-ingest-graph-ingest-entity" || got[0].Filters[0] != "graph.ingest.entity" || got[1].Consumer != "graph-ingest-graph-ingest-filtered" {
		t.Fatalf("wrong admitted identity: %+v", got)
	}
	for name, cfg := range map[string]semconfig.ComponentConfigs{
		"missing": {}, "disabled": {"graph-ingest": {Enabled: false}}, "corrupt": {"graph-ingest": {Enabled: true, Config: json.RawMessage(`{`)}},
		"none":      inputConfig(t),
		"no_stream": inputConfig(t, component.PortDefinition{Name: "entity", Config: component.JetStreamPort{Subjects: []string{"graph.ingest.entity"}}}),
		"wildcard":  inputConfig(t, component.PortDefinition{Name: "entity", Config: component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.>"}}}),
		"core_only": inputConfig(t, component.PortDefinition{Name: "query", Config: component.NATSRequestPort{Subject: "graph.query"}}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := GraphInputs(cfg); err == nil {
				t.Fatal("unprovable input accepted")
			}
		})
	}
}
func TestProjectionErrorsRetainTypedCause(t *testing.T) {
	if ProjectionError(nil) != nil {
		t.Fatal("nil became failure")
	}
	typed := &sourceintent.Blocker{Code: sourceintent.CodeMutation, Message: "conflict", Retryable: true}
	if got := ProjectionError(typed); got.Code != typed.Code || !got.Retryable {
		t.Fatalf("lost typed cause: %+v", got)
	}
	if got := ProjectionError(context.Canceled); got.Code != sourceintent.CodeCanceled {
		t.Fatalf("lost cancellation: %+v", got)
	}
	if got := ProjectionError(errors.New("storage offline")); got.Code != sourceintent.CodeStorage {
		t.Fatalf("lost storage cause: %+v", got)
	}
}
func TestCoordinatorCancellationAndUnavailableStatus(t *testing.T) {
	c, _, j, _ := coordinatorFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal("empty scan need not acquire work", err)
	}
	if err := c.Published(context.Background(), sourceintent.Publication{}); err == nil {
		t.Fatal("nil publication accepted")
	}
	bad := testRecord()
	bad.Binding.Handle = "docs"
	bad.Version = 100
	raw, _ := json.Marshal(bad)
	key, _ := j.recordKey("docs")
	j.kv.(*memoryKV).set(key, raw)
	if _, err := c.Status(context.Background()); err == nil {
		t.Fatal("corrupt journal appeared healthy")
	}
	if c.ReconcileOnce(context.Background()) == nil {
		t.Fatal("corrupt scan succeeded")
	}
	if _, err := NewCoordinator(CoordinatorConfig{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
	if _, err := NewCoordinator(CoordinatorConfig{Authority: j.authority}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
func TestScopeFamiliesAndMalformedIdentity(t *testing.T) {
	a := testRecord().Binding.Authority
	for _, test := range []struct{ factory, config, system, domain string }{
		{"cfgfile-source", `{"project":"Configs"}`, "configs", "config"},
		{"url-source", `{"urls":["https://Example.com/a"]}`, "example-com", "web"},
		{"objectstore-source", `{"bucket":"manuals","version":"v1"}`, "manuals-v1", "web"},
		{"objectstore-source", `{"bucket":"ignored","project":"manuals"}`, "manuals", "web"},
		{"git-source", `{"repo_path":"/repos/source.repo"}`, "source-repo", "git"},
		{"git-source", `{"repo_url":"https://github.com/acme/repo.git"}`, "github-com-acme-repo", "git"},
		{"image-source", `{"paths":["/images"]}`, "images", "media"},
		{"audio-source", `{"paths":["/audio"]}`, "audio", "media"},
		{"video-source", `{"paths":["/video"]}`, "video", "media"},
	} {
		t.Run(test.factory+test.system, func(t *testing.T) {
			scope, err := ScopeFor(a, "source", types.ComponentConfig{Name: test.factory, Config: json.RawMessage(test.config)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(scope.Selectors) != 1 || scope.Selectors[0].System != test.system || scope.Selectors[0].Domain != test.domain {
				t.Fatalf("scope=%+v", scope)
			}
		})
	}
	for _, test := range []struct{ factory, config string }{{"unknown", `{}`}, {"doc-source", `{}`}, {"url-source", `{"urls":["not-a-url"]}`}, {"doc-source", `{"paths":1}`}, {"doc-source", `{`}} {
		if _, err := ScopeFor(a, "source", types.ComponentConfig{Name: test.factory, Config: json.RawMessage(test.config)}, nil); err == nil {
			t.Fatalf("malformed scope accepted: %+v", test)
		}
	}
	if IsSourceFactory("supersession") {
		t.Fatal("operational component accepted as source")
	}
}
