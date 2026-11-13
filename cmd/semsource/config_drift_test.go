package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/types"
)

// driftRegistry decodes a model registry whose embedding endpoint is at url.
func driftRegistry(t *testing.T, url string) *model.Registry {
	t.Helper()
	var reg model.Registry
	doc := `{"endpoints": {"semembed": {"provider": "openai", "url": "` + url + `", "model": "embed-s"}},
		"capabilities": {"embedding": {"preferred": ["semembed"]}}}`
	if err := json.Unmarshal([]byte(doc), &reg); err != nil {
		t.Fatal(err)
	}
	return &reg
}

// driftFixture is a small composition: one service, two components, a model
// registry, and the NATS settings.
func driftFixture(t *testing.T) *semconfig.Config {
	t.Helper()
	return &semconfig.Config{
		Version:       "3.0.0",
		ModelRegistry: driftRegistry(t, "http://old:8081/v1"),
		NATS:          semconfig.NATSConfig{JetStream: semconfig.JetStreamConfig{Enabled: true}},
		Services: types.ServiceConfigs{
			"metrics": {Enabled: true, Config: json.RawMessage(`{"port":9091,"path":"/metrics"}`)},
		},
		Components: semconfig.ComponentConfigs{
			"doc-source-guide": {
				Name: "doc-source", Type: types.ComponentTypeProcessor, Enabled: true,
				Config: json.RawMessage(`{"paths":["./guide"],"watch_enabled":true}`),
			},
			"source-manifest": {
				Name: "source-manifest", Type: types.ComponentTypeProcessor, Enabled: true,
				Config: json.RawMessage(`{"sources":[{"type":"docs","paths":["./guide"]}]}`),
			},
		},
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
func TestRetainedConfigDrift(t *testing.T) {
	tests := []struct {
		name                 string
		mutate               func(effective *semconfig.Config)
		wantDiffering        []string
		wantRetainedOnly     []string
		wantRetainedDisabled []string
	}{
		{name: "identical", mutate: func(*semconfig.Config) {}},
		{
			name: "re-encoded through the bucket",
			mutate: func(c *semconfig.Config) {
				c.Services["metrics"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{ "path": "/metrics", "port": 9091 }`)}
				guide := c.Components["doc-source-guide"]
				guide.Config = json.RawMessage("{\n  \"watch_enabled\": true,\n  \"paths\": [\"./guide\"]\n}")
				c.Components["doc-source-guide"] = guide
				c.ModelRegistry = driftRegistry(t, "http://old:8081/v1")
			},
		},
		{
			name: "component config edited",
			mutate: func(c *semconfig.Config) {
				guide := c.Components["doc-source-guide"]
				guide.Config = json.RawMessage(`{"paths":["./guide","./more"],"watch_enabled":true}`)
				c.Components["doc-source-guide"] = guide
			},
			wantDiffering: []string{"components.doc-source-guide"},
		},
		{
			name: "component the file enables held disabled",
			mutate: func(c *semconfig.Config) {
				guide := c.Components["doc-source-guide"]
				guide.Enabled = false
				c.Components["doc-source-guide"] = guide
			},
			wantRetainedDisabled: []string{"components.doc-source-guide"},
		},
		{
			name: "component held disabled with its config edited too",
			mutate: func(c *semconfig.Config) {
				guide := c.Components["doc-source-guide"]
				guide.Enabled = false
				guide.Config = json.RawMessage(`{"paths":["./other"]}`)
				c.Components["doc-source-guide"] = guide
			},
			wantRetainedDisabled: []string{"components.doc-source-guide"},
		},
		{
			name: "component only in the retained configuration",
			mutate: func(c *semconfig.Config) {
				c.Components["doc-source-runtime"] = types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{}`)}
			},
			wantRetainedOnly: []string{"components.doc-source-runtime"},
		},
		{
			name:          "component only in the file",
			mutate:        func(c *semconfig.Config) { delete(c.Components, "source-manifest") },
			wantDiffering: []string{"components.source-manifest"},
		},
		{
			name: "service edited",
			mutate: func(c *semconfig.Config) {
				c.Services["metrics"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{"port":9092,"path":"/metrics"}`)}
			},
			wantDiffering: []string{"services.metrics"},
		},
		{
			name: "service only in the retained configuration",
			mutate: func(c *semconfig.Config) {
				c.Services["extra"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{}`)}
			},
			wantRetainedOnly: []string{"services.extra"},
		},
		{
			name:          "model registry endpoint edited",
			mutate:        func(c *semconfig.Config) { c.ModelRegistry = driftRegistry(t, "http://new:9999/v1") },
			wantDiffering: []string{"model_registry"},
		},
		{
			name:          "model registry absent from the retained configuration",
			mutate:        func(c *semconfig.Config) { c.ModelRegistry = nil },
			wantDiffering: []string{"model_registry"},
		},
		{
			name:          "NATS settings edited",
			mutate:        func(c *semconfig.Config) { c.NATS.URLs = []string{"nats://retained:4222"} },
			wantDiffering: []string{"nats"},
		},
		{
			name: "several, sorted",
			mutate: func(c *semconfig.Config) {
				delete(c.Services, "metrics")
				c.Components["a-runtime"] = types.ComponentConfig{Name: "doc-source"}
				c.Components["z-runtime"] = types.ComponentConfig{Name: "doc-source"}
				delete(c.Components, "source-manifest")
				c.ModelRegistry = driftRegistry(t, "http://new:9999/v1")
			},
			wantDiffering:    []string{"components.source-manifest", "model_registry", "services.metrics"},
			wantRetainedOnly: []string{"components.a-runtime", "components.z-runtime"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			effective := driftFixture(t)
			tc.mutate(effective)
			got := retainedConfigDrift(driftFixture(t), effective)
			if !sameNames(got.differing, tc.wantDiffering) {
				t.Errorf("differing = %v, want %v", got.differing, tc.wantDiffering)
			}
			if !sameNames(got.retainedOnly, tc.wantRetainedOnly) {
				t.Errorf("retainedOnly = %v, want %v", got.retainedOnly, tc.wantRetainedOnly)
			}
			if !sameNames(got.retainedDisabled, tc.wantRetainedDisabled) {
				t.Errorf("retainedDisabled = %v, want %v", got.retainedDisabled, tc.wantRetainedDisabled)
			}
		})
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
func TestWarnRetainedConfigDrift(t *testing.T) {
	t.Run("equal is silent", func(t *testing.T) {
		logs := &recordedLogs{}
		warnRetainedConfigDrift(slog.New(logs), driftFixture(t), driftFixture(t))
		if got := logs.withMessage(retainedConfigDriftMessage); len(got) != 0 {
			t.Fatalf("logged %d drift warnings for equal configurations", len(got))
		}
	})
	t.Run("differing only: capped, with the plain hint", func(t *testing.T) {
		fromFile, effective := driftFixture(t), driftFixture(t)
		var want []string
		for i := range 12 {
			name := fmt.Sprintf("edited-%02d", i)
			fromFile.Components[name] = types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"paths":["./a"]}`)}
			effective.Components[name] = types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"paths":["./b"]}`)}
			if i < 10 {
				want = append(want, "components."+name)
			}
		}
		rec := oneDriftWarning(t, fromFile, effective)
		if entries := recordAttr(rec, "entries"); !reflect.DeepEqual(entries, want) {
			t.Errorf("entries = %v, want %v", entries, want)
		}
		if n := recordAttr(rec, "differing"); n != int64(12) {
			t.Errorf("differing = %v, want 12", n)
		}
		if n := recordAttr(rec, "retained_only_count"); n != int64(0) {
			t.Errorf("retained_only_count = %v, want 0", n)
		}
		hint, _ := recordAttr(rec, "hint").(string)
		if hint != retainedHint {
			t.Errorf("hint = %q, want the plain hint", hint)
		}
		for _, promise := range []string{"for the file to apply", "drops", "re-enables"} {
			if strings.Contains(hint, promise) {
				t.Errorf("plain hint %q says %q", hint, promise)
			}
		}
		if !strings.Contains(hint, "retained configuration stays in effect until") || !strings.Contains(hint, "README") {
			t.Errorf("plain hint %q does not say the retained configuration wins until the revision is higher and point at the README", hint)
		}
	})
	t.Run("retained-only entries are named separately and the hint warns they drop", func(t *testing.T) {
		effective := driftFixture(t)
		var want []string
		for i := range 11 {
			name := fmt.Sprintf("runtime-%02d", i)
			effective.Components[name] = types.ComponentConfig{Name: "doc-source"}
			if i < 10 {
				want = append(want, "components."+name)
			}
		}
		rec := oneDriftWarning(t, driftFixture(t), effective)
		if entries := recordAttr(rec, "entries"); len(entries.([]string)) != 0 {
			t.Errorf("entries = %v, want none: retained-only names belong to retained_only", entries)
		}
		if got := recordAttr(rec, "retained_only"); !reflect.DeepEqual(got, want) {
			t.Errorf("retained_only = %v, want %v", got, want)
		}
		if n := recordAttr(rec, "retained_only_count"); n != int64(11) {
			t.Errorf("retained_only_count = %v, want 11", n)
		}
		hint, _ := recordAttr(rec, "hint").(string)
		if !strings.Contains(hint, "drops the retained_only components") || !strings.Contains(hint, "add them to the file first") {
			t.Errorf("hint %q does not warn that making the file newest drops the retained-only components", hint)
		}
	})
	t.Run("a retained-only service gets the plain hint: a newer file does not remove it", func(t *testing.T) {
		effective := driftFixture(t)
		effective.Services["extra"] = types.ServiceConfig{Enabled: true}
		rec := oneDriftWarning(t, driftFixture(t), effective)
		if got := recordAttr(rec, "retained_only"); !reflect.DeepEqual(got, []string{"services.extra"}) {
			t.Errorf("retained_only = %v, want [services.extra]", got)
		}
		if hint := recordAttr(rec, "hint"); hint != retainedHint {
			t.Errorf("hint = %q, want the plain hint", hint)
		}
	})
	t.Run("retained-disabled components are named separately and the hint warns they re-enable", func(t *testing.T) {
		effective := driftFixture(t)
		guide := effective.Components["doc-source-guide"]
		guide.Enabled = false
		effective.Components["doc-source-guide"] = guide
		rec := oneDriftWarning(t, driftFixture(t), effective)
		if entries := recordAttr(rec, "entries"); len(entries.([]string)) != 0 {
			t.Errorf("entries = %v, want none: retained-disabled names belong to retained_disabled", entries)
		}
		if got := recordAttr(rec, "retained_disabled"); !reflect.DeepEqual(got, []string{"components.doc-source-guide"}) {
			t.Errorf("retained_disabled = %v, want [components.doc-source-guide]", got)
		}
		if n := recordAttr(rec, "retained_disabled_count"); n != int64(1) {
			t.Errorf("retained_disabled_count = %v, want 1", n)
		}
		hint, _ := recordAttr(rec, "hint").(string)
		if !strings.Contains(hint, "re-enables the retained_disabled components") || strings.Contains(hint, "drops") {
			t.Errorf("hint %q does not warn that making the file newest re-enables the retained-disabled component, and only that", hint)
		}
	})
	t.Run("names entries, never values", func(t *testing.T) {
		effective := driftFixture(t)
		effective.ModelRegistry = driftRegistry(t, "http://secret-host:9999/v1")
		rec := oneDriftWarning(t, driftFixture(t), effective)
		var text strings.Builder
		rec.Attrs(func(a slog.Attr) bool {
			fmt.Fprintf(&text, "%s=%v ", a.Key, a.Value.Any())
			return true
		})
		if strings.Contains(text.String(), "secret-host") || strings.Contains(text.String(), "old:8081") {
			t.Errorf("warning carries configuration values: %s", text.String())
		}
	})
}

func oneDriftWarning(t *testing.T, fromFile, effective *semconfig.Config) slog.Record {
	t.Helper()
	logs := &recordedLogs{}
	warnRetainedConfigDrift(slog.New(logs), fromFile, effective)
	got := logs.withMessage(retainedConfigDriftMessage)
	if len(got) != 1 {
		t.Fatalf("logged %d drift warnings, want 1", len(got))
	}
	if got[0].Level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", got[0].Level)
	}
	return got[0]
}

// sameNames treats a nil and an empty list as equal.
func sameNames(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

// recordedLogs is a slog.Handler that keeps every record.
type recordedLogs struct {
	mu      sync.Mutex
	records []slog.Record
}

func (r *recordedLogs) Enabled(context.Context, slog.Level) bool { return true }

func (r *recordedLogs) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Clone())
	return nil
}

func (r *recordedLogs) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *recordedLogs) WithGroup(string) slog.Handler { return r }

func (r *recordedLogs) withMessage(msg string) []slog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []slog.Record
	for _, rec := range r.records {
		if rec.Message == msg {
			out = append(out, rec)
		}
	}
	return out
}

func recordAttr(rec slog.Record, key string) any {
	var v any
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v = a.Value.Any()
			return false
		}
		return true
	})
	return v
}
