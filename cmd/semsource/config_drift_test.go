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
	"github.com/c360studio/semengine/types"
)

// driftFixture is a small composition: one service and two components.
func driftFixture() *semconfig.Config {
	return &semconfig.Config{
		Version: "3.0.0",
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
		name   string
		mutate func(effective *semconfig.Config)
		want   []string
	}{
		{name: "identical", mutate: func(*semconfig.Config) {}},
		{
			name: "re-encoded through the bucket",
			mutate: func(c *semconfig.Config) {
				c.Services["metrics"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{ "path": "/metrics", "port": 9091 }`)}
				guide := c.Components["doc-source-guide"]
				guide.Config = json.RawMessage("{\n  \"watch_enabled\": true,\n  \"paths\": [\"./guide\"]\n}")
				c.Components["doc-source-guide"] = guide
			},
		},
		{
			name: "component config edited",
			mutate: func(c *semconfig.Config) {
				guide := c.Components["doc-source-guide"]
				guide.Config = json.RawMessage(`{"paths":["./guide","./more"],"watch_enabled":true}`)
				c.Components["doc-source-guide"] = guide
			},
			want: []string{"components.doc-source-guide"},
		},
		{
			name: "component disabled",
			mutate: func(c *semconfig.Config) {
				guide := c.Components["doc-source-guide"]
				guide.Enabled = false
				c.Components["doc-source-guide"] = guide
			},
			want: []string{"components.doc-source-guide"},
		},
		{
			name: "component only in the retained configuration",
			mutate: func(c *semconfig.Config) {
				c.Components["doc-source-runtime"] = types.ComponentConfig{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{}`)}
			},
			want: []string{"components.doc-source-runtime"},
		},
		{
			name:   "component only in the file",
			mutate: func(c *semconfig.Config) { delete(c.Components, "source-manifest") },
			want:   []string{"components.source-manifest"},
		},
		{
			name: "service edited",
			mutate: func(c *semconfig.Config) {
				c.Services["metrics"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{"port":9092,"path":"/metrics"}`)}
			},
			want: []string{"services.metrics"},
		},
		{
			name: "several, sorted",
			mutate: func(c *semconfig.Config) {
				delete(c.Services, "metrics")
				c.Components["a-runtime"] = types.ComponentConfig{Name: "doc-source"}
				delete(c.Components, "source-manifest")
			},
			want: []string{"components.a-runtime", "components.source-manifest", "services.metrics"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			effective := driftFixture()
			tc.mutate(effective)
			got := retainedConfigDrift(driftFixture(), effective)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("retainedConfigDrift = %v, want %v", got, tc.want)
			}
		})
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
func TestWarnRetainedConfigDrift(t *testing.T) {
	t.Run("equal is silent", func(t *testing.T) {
		logs := &recordedLogs{}
		warnRetainedConfigDrift(slog.New(logs), driftFixture(), driftFixture())
		if got := logs.withMessage(retainedConfigDriftMessage); len(got) != 0 {
			t.Fatalf("logged %d drift warnings for equal configurations", len(got))
		}
	})
	t.Run("one capped warning", func(t *testing.T) {
		effective := driftFixture()
		var want []string
		for i := range 12 {
			name := fmt.Sprintf("runtime-%02d", i)
			effective.Components[name] = types.ComponentConfig{Name: "doc-source"}
			if i < 10 {
				want = append(want, "components."+name)
			}
		}
		logs := &recordedLogs{}
		warnRetainedConfigDrift(slog.New(logs), driftFixture(), effective)
		got := logs.withMessage(retainedConfigDriftMessage)
		if len(got) != 1 {
			t.Fatalf("logged %d drift warnings, want 1", len(got))
		}
		rec := got[0]
		if rec.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", rec.Level)
		}
		if entries := recordAttr(rec, "entries"); !reflect.DeepEqual(entries, want) {
			t.Errorf("entries = %v, want %v", entries, want)
		}
		if n := recordAttr(rec, "differing"); n != int64(12) {
			t.Errorf("differing = %v, want 12", n)
		}
		if hint, _ := recordAttr(rec, "hint").(string); !strings.Contains(hint, "config_revision") {
			t.Errorf("hint %q does not name config_revision", hint)
		}
	})
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
