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
	"github.com/c360studio/semsource/internal/natstest"
)

// revisionBootFile is the file the deployment first boots from: one docs
// source whose component instance is doc-source-guide.
const revisionBootFile = `{
  "namespace": "acme",
  "platform_id": "revision",
  "sources": [{"type": "docs", "paths": ["./guide"]}]
}`

// revisionEditedFile changes that source's paths, which keeps its instance
// name and changes its config and the manifest's.
const revisionEditedFile = `{
  "namespace": "acme",
  "platform_id": "revision",
  %s
  "sources": [{"type": "docs", "paths": ["./guide", "./more"]}]
}`

// bootResult is what one boot left behind.
type bootResult struct {
	effective *semconfig.Config
	drift     []slog.Record
	version   string // the bucket's version key after the boot
	docRev    uint64 // the bucket revision of components.doc-source-guide
}

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
	result := bootResult{effective: configMgr.GetConfig().Get()}
	if err := configMgr.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	result.drift = logs.withMessage(retainedConfigDriftMessage)

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
	doc, err := kv.Get(ctx, "components.doc-source-guide")
	if err != nil {
		t.Fatal(err)
	}
	result.docRev = doc.Revision()
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

// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: A retained configuration that differs from the file is reported
// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: The composition version orders the binary and the operator
// spec: openspec/changes/operator-config-revision/specs/runtime-configuration/spec.md#Requirement: Source commands that write the configuration file advance its revision
func TestIntegrationConfigRevisionArbitration(t *testing.T) {
	tests := []struct {
		name string
		// edit rewrites the file between the first and second boot.
		edit        func(t *testing.T, path string)
		wantDrift   []string // entries the second boot's single warning names; nil: no warning
		wantVersion string
		wantPaths   []string // the doc source's effective paths after the second boot
		wantPushed  bool     // the second boot wrote the file's composition to the bucket
		// wantManifest is the effective manifest's source types, in order.
		wantManifest []string
	}{
		{
			name:         "unchanged file is silent",
			edit:         func(*testing.T, string) {},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
		},
		{
			name: "edit without a revision bump is reported and not applied",
			edit: func(t *testing.T, path string) {
				writeFile(t, path, fmt.Sprintf(revisionEditedFile, ""))
			},
			wantDrift:    []string{"components.doc-source-guide", "components.source-manifest"},
			wantVersion:  "3.0.0",
			wantPaths:    []string{"./guide"},
			wantManifest: []string{"docs"},
		},
		{
			name: "edit with a revision bump is applied",
			edit: func(t *testing.T, path string) {
				writeFile(t, path, fmt.Sprintf(revisionEditedFile, `"config_revision": 1,`))
			},
			wantVersion:  "3.1.0",
			wantPaths:    []string{"./guide", "./more"},
			wantPushed:   true,
			wantManifest: []string{"docs"},
		},
		{
			name: "source added with the CLI is applied",
			edit: func(t *testing.T, path string) {
				term := cli.NewTerm(strings.NewReader(""), io.Discard)
				if err := cli.Add(term, path, []string{"ast", "--path", "./wsB", "--language", "go"}); err != nil {
					t.Fatal(err)
				}
			},
			wantVersion:  "3.1.0",
			wantPaths:    []string{"./guide"},
			wantPushed:   true,
			wantManifest: []string{"docs", "ast"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			broker := natstest.New(t, natstest.WithJetStream())
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "semsource.json")
			writeFile(t, path, revisionBootFile)

			first := bootConfigFile(ctx, t, broker.URL, path)
			if len(first.drift) != 0 {
				t.Fatalf("first boot logged %d drift warnings, want none", len(first.drift))
			}
			tc.edit(t, path)
			second := bootConfigFile(ctx, t, broker.URL, path)

			switch {
			case tc.wantDrift == nil && len(second.drift) != 0:
				t.Errorf("second boot warned %v, want no warning", recordAttr(second.drift[0], "entries"))
			case tc.wantDrift != nil && len(second.drift) != 1:
				t.Errorf("second boot logged %d drift warnings, want 1", len(second.drift))
			case tc.wantDrift != nil:
				if got := recordAttr(second.drift[0], "entries"); !reflect.DeepEqual(got, tc.wantDrift) {
					t.Errorf("warning names %v, want %v", got, tc.wantDrift)
				}
			}
			if second.version != tc.wantVersion {
				t.Errorf("bucket version = %q, want %q", second.version, tc.wantVersion)
			}
			if pushed := second.docRev != first.docRev; pushed != tc.wantPushed {
				t.Errorf("components.doc-source-guide revision %d -> %d: pushed = %t, want %t",
					first.docRev, second.docRev, pushed, tc.wantPushed)
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
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
