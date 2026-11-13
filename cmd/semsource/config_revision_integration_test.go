//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semsource/cli"
	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/natstest"
)

// revisionFile renders a configuration file for the "acme"/"revision"
// deployment: members is inserted before the sources and may set
// config_revision, graph and model_registry; sources is the sources array.
func revisionFile(members, sources string) string {
	return `{"namespace": "acme", "platform_id": "revision", ` + members + ` "sources": ` + sources + `}`
}

const (
	// guideSource's component instance is doc-source-guide.
	guideSource = `[{"type": "docs", "paths": ["./guide"]}]`
	// guideEdited keeps guideSource's instance name and changes its config
	// and the manifest's.
	guideEdited = `[{"type": "docs", "paths": ["./guide", "./more"]}]`
)

// semanticMembers is a tier 1 composition whose embedding endpoint is at url.
func semanticMembers(url string) string {
	return `"graph": {"embedder_type": "http"},
	  "model_registry": {"endpoints": {"semembed": {"provider": "openai", "url": "` + url + `", "model": "embed-s"}},
	                     "capabilities": {"embedding": {"preferred": ["semembed"]}}},`
}

// bootResult is what one boot left behind.
type bootResult struct {
	effective *semconfig.Config
	logs      *recordedLogs
	version   string            // the bucket's version key after the boot
	revisions map[string]uint64 // the bucket revision of every components.* key after the boot
}

// drift returns the configuration-difference warnings the boot logged.
func (b bootResult) drift() []slog.Record { return b.logs.withMessage(retainedConfigDriftMessage) }

// bootConfigFile runs the boot path from loading the file through config
// arbitration — loadAndExpandConfig, setupNATS, startConfigManager — then stops.
func bootConfigFile(ctx context.Context, t *testing.T, natsURL, path string) bootResult {
	t.Helper()
	logs := &recordedLogs{}
	logger := slog.New(logs)
	cfg, _, err := loadAndExpandConfig(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	nc, ssCfg, err := setupNATS(ctx, natsURL, cfg, logger, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := nc.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)
	if err != nil {
		t.Fatal(err)
	}
	result := bootResult{effective: configMgr.GetConfig().Get(), logs: logs, revisions: map[string]uint64{}}
	if err := configMgr.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	bucket, err := semconfig.BucketName("acme", "revision")
	if err != nil {
		t.Fatal(err)
	}
	kv, err := nc.GetKeyValueBucket(ctx, bucket)
	if err != nil {
		t.Fatal(err)
	}
	version, err := kv.Get(ctx, "version")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(version.Value(), &result.version); err != nil {
		t.Fatal(err)
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "components.") {
			continue
		}
		entry, err := kv.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		result.revisions[key] = entry.Revision()
	}
	return result
}

// componentField decodes one member of a component's config.
func componentField(t *testing.T, cfg *semconfig.Config, component, field string, into any) {
	t.Helper()
	cc, ok := cfg.Components[component]
	if !ok {
		t.Fatalf("effective configuration has no component %q", component)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(cc.Config, &fields); err != nil {
		t.Fatalf("decode %s config: %v", component, err)
	}
	if err := json.Unmarshal(fields[field], into); err != nil {
		t.Fatalf("decode %s.%s: %v", component, field, err)
	}
}

// embeddingURL is the effective model registry's semembed endpoint URL.
func embeddingURL(t *testing.T, cfg *semconfig.Config) string {
	t.Helper()
	if cfg.ModelRegistry == nil || cfg.ModelRegistry.Endpoints["semembed"] == nil {
		t.Fatal("effective configuration has no semembed endpoint")
	}
	return cfg.ModelRegistry.Endpoints["semembed"].URL
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func addWithCLI(t *testing.T, path string, args ...string) {
	t.Helper()
	if err := cli.Add(cli.NewTerm(strings.NewReader(""), io.Discard), path, args); err != nil {
		t.Fatal(err)
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: The composition version orders the binary and the operator
// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: Source commands that write the configuration file advance its revision
func TestIntegrationConfigRevisionArbitration(t *testing.T) {
	tests := []struct {
		name    string
		initial string // the file the first boot reads
		// edit rewrites the file between the first and second boot.
		edit        func(t *testing.T, path string)
		wantDrift   []string // entries the second boot's single warning names; nil: no warning
		wantVersion string
		wantPaths   []string // the doc source's effective paths after the second boot
		wantPushed  bool     // the second boot wrote the file's composition to the bucket
		// wantManifest is the effective manifest's source types, in order.
		wantManifest []string
		// wantURL is the effective embedding endpoint, for files with a model registry.
		wantURL string
	}{
		{
			name:         "unchanged file is silent",
			initial:      revisionFile("", guideSource),
			edit:         func(*testing.T, string) {},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
		},
		{
			name:    "edit without a revision bump is reported and not applied",
			initial: revisionFile("", guideSource),
			edit: func(t *testing.T, path string) {
				writeFile(t, path, revisionFile("", guideEdited))
			},
			wantDrift:    []string{"components.doc-source-guide", "components.source-manifest"},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
		},
		{
			name:    "edit with a revision bump is applied",
			initial: revisionFile("", guideSource),
			edit: func(t *testing.T, path string) {
				writeFile(t, path, revisionFile(`"config_revision": 1,`, guideEdited))
			},
			wantVersion:  "3.1.0",
			wantPaths:    []string{"./guide", "./more"},
			wantPushed:   true,
			wantManifest: []string{"docs"},
		},
		{
			name:    "source added with the CLI is applied",
			initial: revisionFile("", guideSource),
			edit: func(t *testing.T, path string) {
				addWithCLI(t, path, "ast", "--path", "./wsB", "--language", "go")
			},
			wantVersion:  "3.1.0",
			wantPaths:    []string{"./guide"},
			wantPushed:   true,
			wantManifest: []string{"docs", "ast"},
		},
		{
			name:         "unchanged file with a model registry is silent",
			initial:      revisionFile(semanticMembers("http://old:8081/v1"), guideSource),
			edit:         func(*testing.T, string) {},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
			wantURL:      "http://old:8081/v1",
		},
		{
			name:    "model registry edit without a revision bump is reported and not applied",
			initial: revisionFile(semanticMembers("http://old:8081/v1"), guideSource),
			edit: func(t *testing.T, path string) {
				writeFile(t, path, revisionFile(semanticMembers("http://new:9999/v1"), guideSource))
			},
			wantDrift:    []string{"model_registry"},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
			wantURL:      "http://old:8081/v1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			broker := natstest.New(t, natstest.WithJetStream())
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "semsource.json")
			writeFile(t, path, tc.initial)

			first := bootConfigFile(ctx, t, broker.URL, path)
			if n := len(first.drift()); n != 0 {
				t.Fatalf("first boot logged %d drift warnings, want none", n)
			}
			tc.edit(t, path)
			second := bootConfigFile(ctx, t, broker.URL, path)

			drift := second.drift()
			switch {
			case tc.wantDrift == nil && len(drift) != 0:
				t.Errorf("second boot warned %v, want no warning", recordAttr(drift[0], "entries"))
			case tc.wantDrift != nil && len(drift) != 1:
				t.Errorf("second boot logged %d drift warnings, want 1", len(drift))
			case tc.wantDrift != nil:
				if got := recordAttr(drift[0], "entries"); !reflect.DeepEqual(got, tc.wantDrift) {
					t.Errorf("warning names %v, want %v", got, tc.wantDrift)
				}
			}
			if second.version != tc.wantVersion {
				t.Errorf("bucket version = %q, want %q", second.version, tc.wantVersion)
			}
			const doc = "components.doc-source-guide"
			if pushed := second.revisions[doc] != first.revisions[doc]; pushed != tc.wantPushed {
				t.Errorf("%s revision %d -> %d: pushed = %t, want %t",
					doc, first.revisions[doc], second.revisions[doc], pushed, tc.wantPushed)
			}
			var paths []string
			componentField(t, second.effective, "doc-source-guide", "paths", &paths)
			if !reflect.DeepEqual(paths, tc.wantPaths) {
				t.Errorf("effective doc source paths = %v, want %v", paths, tc.wantPaths)
			}
			var manifest []struct {
				Type string `json:"type"`
			}
			componentField(t, second.effective, "source-manifest", "sources", &manifest)
			var types []string
			for _, src := range manifest {
				types = append(types, src.Type)
			}
			if !reflect.DeepEqual(types, tc.wantManifest) {
				t.Errorf("effective manifest source types = %v, want %v", types, tc.wantManifest)
			}
			if tc.wantURL != "" {
				if got := embeddingURL(t, second.effective); got != tc.wantURL {
					t.Errorf("effective embedding endpoint = %q, want %q", got, tc.wantURL)
				}
			}
		})
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A newer file replaces the retained components
func TestIntegrationNewerFileReplacesRetainedComponents(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		edit    func(t *testing.T, path string)
		present string // the component every boot after the edit runs
		absent  string // the component no boot after the edit runs or retains
	}{
		{
			name:    "source removed with the CLI stays removed",
			initial: revisionFile("", `[{"type": "docs", "paths": ["./guide"]}, {"type": "docs", "paths": ["./other"]}]`),
			edit: func(t *testing.T, path string) {
				if err := cli.Remove(cli.NewTerm(strings.NewReader(""), io.Discard), path, 1); err != nil {
					t.Fatal(err)
				}
			},
			present: "doc-source-guide",
			absent:  "doc-source-other",
		},
		{
			name:    "corrected source path does not bring the old source back",
			initial: revisionFile("", guideSource),
			edit: func(t *testing.T, path string) {
				writeFile(t, path, revisionFile(`"config_revision": 1,`, `[{"type": "docs", "paths": ["./manual"]}]`))
			},
			present: "doc-source-manual",
			absent:  "doc-source-guide",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			broker := natstest.New(t, natstest.WithJetStream())
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "semsource.json")
			writeFile(t, path, tc.initial)

			first := bootConfigFile(ctx, t, broker.URL, path)
			if _, ok := first.effective.Components[tc.absent]; !ok {
				t.Fatalf("first boot does not run %s; the fixture does not exercise its removal", tc.absent)
			}
			tc.edit(t, path)
			for boot := 2; boot <= 3; boot++ {
				got := bootConfigFile(ctx, t, broker.URL, path)
				if n := len(got.drift()); n != 0 {
					t.Errorf("boot %d logged %d drift warnings, want none: %v", boot, n, recordAttr(got.drift()[0], "retained_only"))
				}
				if _, ok := got.effective.Components[tc.present]; !ok {
					t.Errorf("boot %d does not run %s", boot, tc.present)
				}
				if _, ok := got.effective.Components[tc.absent]; ok {
					t.Errorf("boot %d runs %s, which the file no longer defines", boot, tc.absent)
				}
				if _, ok := got.revisions["components."+tc.absent]; ok {
					t.Errorf("boot %d left components.%s in the config bucket", boot, tc.absent)
				}
				pruned := got.logs.withMessage(prunedComponentsMessage)
				switch {
				case boot == 2 && (len(pruned) != 1 || !reflect.DeepEqual(recordAttr(pruned[0], "components"), []string{tc.absent})):
					t.Errorf("boot 2 removal records = %d, want one naming %s", len(pruned), tc.absent)
				case boot == 3 && len(pruned) != 0:
					t.Errorf("boot 3 removed %v; nothing was left to remove", recordAttr(pruned[0], "components"))
				}
			}
		})
	}
}

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
func TestIntegrationRetainedOnlyComponentIsNamedSeparately(t *testing.T) {
	broker := natstest.New(t, natstest.WithJetStream())
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "semsource.json")
	writeFile(t, path, revisionFile("", guideSource))
	bootConfigFile(ctx, t, broker.URL, path)

	// A source accepted at runtime is written to the bucket only, the way the
	// add handler records it for the next boot.
	nc, ssCfg, err := setupNATS(ctx, broker.URL, mustLoad(ctx, t, path), slog.New(&recordedLogs{}), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := nc.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	configMgr, err := startConfigManager(ctx, ssCfg, nc, slog.New(&recordedLogs{}))
	if err != nil {
		t.Fatal(err)
	}
	runtime := configMgr.GetConfig().Get().Components["doc-source-guide"]
	runtime.Config = json.RawMessage(`{"paths":["./runtime"]}`)
	if err := configMgr.PutComponentToKV(ctx, "doc-source-runtime", runtime); err != nil {
		t.Fatal(err)
	}
	if err := configMgr.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	got := bootConfigFile(ctx, t, broker.URL, path)
	drift := got.drift()
	if len(drift) != 1 {
		t.Fatalf("logged %d drift warnings, want 1", len(drift))
	}
	if entries := recordAttr(drift[0], "entries"); len(entries.([]string)) != 0 {
		t.Errorf("entries = %v, want none", entries)
	}
	if only := recordAttr(drift[0], "retained_only"); !reflect.DeepEqual(only, []string{"components.doc-source-runtime"}) {
		t.Errorf("retained_only = %v, want [components.doc-source-runtime]", only)
	}
	if hint, _ := recordAttr(drift[0], "hint").(string); !strings.Contains(hint, "drops the retained_only entries") {
		t.Errorf("hint %q does not say that making the file newest drops the retained-only entry", hint)
	}
	if _, ok := got.effective.Components["doc-source-runtime"]; !ok {
		t.Error("the retained runtime source is not in effect at an equal version")
	}
}

func mustLoad(ctx context.Context, t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, _, err := loadAndExpandConfig(ctx, path)
	if err != nil {
		t.Fatal(fmt.Errorf("load %s: %w", path, err))
	}
	return cfg
}
