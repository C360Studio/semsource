package sourcemanifest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semstreams/types"
)

func mustComponentConfig(t *testing.T, name string, cfg map[string]any) types.ComponentConfig {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return types.ComponentConfig{Name: name, Config: raw}
}

func TestSystemsForComponentConfig_ASTSource(t *testing.T) {
	cc := mustComponentConfig(t, "ast-source", map[string]any{
		"watch_paths": []map[string]any{
			{"project": "acme-service", "version": ""},
		},
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] != "acme-service" {
		t.Errorf("systems = %v, want [acme-service]", got)
	}
}

func TestSystemsForComponentConfig_ASTSource_VersionScoped(t *testing.T) {
	cc := mustComponentConfig(t, "ast-source", map[string]any{
		"watch_paths": []map[string]any{
			{"project": "acme-dep", "version": "v1.9.0"},
		},
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] != "acme-dep-v1-9-0" {
		t.Errorf("systems = %v, want [acme-dep-v1-9-0]", got)
	}
}

func TestSystemsForComponentConfig_ASTSource_MultipleWatchPaths(t *testing.T) {
	cc := mustComponentConfig(t, "ast-source", map[string]any{
		"watch_paths": []map[string]any{
			{"project": "svc-a"},
			{"project": "svc-b"},
		},
	})
	got := systemsForComponentConfig(cc)
	want := []string{"svc-a", "svc-b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("systems = %v, want %v", got, want)
	}
}

func TestSystemsForComponentConfig_DocSource(t *testing.T) {
	cc := mustComponentConfig(t, "doc-source", map[string]any{
		"paths": []string{"/workspace/docs"},
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] == "" {
		t.Errorf("systems = %v, want one non-empty slug", got)
	}
}

func TestSystemsForComponentConfig_CfgFileSource(t *testing.T) {
	cc := mustComponentConfig(t, "cfgfile-source", map[string]any{
		"paths": []string{"/workspace/repo"},
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 {
		t.Errorf("systems = %v, want one slug", got)
	}
}

func TestSystemsForComponentConfig_URLSource(t *testing.T) {
	cc := mustComponentConfig(t, "url-source", map[string]any{
		"urls": []string{"https://docs.example.com/page"},
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] == "" {
		t.Errorf("systems = %v, want one non-empty slug", got)
	}
}

func TestSystemsForComponentConfig_GitSource(t *testing.T) {
	cc := mustComponentConfig(t, "git-source", map[string]any{
		"repo_url":    "https://github.com/acme/repo",
		"branch_slug": "",
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] != "github-com-acme-repo" {
		t.Errorf("systems = %v, want [github-com-acme-repo]", got)
	}
}

func TestSystemsForComponentConfig_GitSource_BranchScoped(t *testing.T) {
	cc := mustComponentConfig(t, "git-source", map[string]any{
		"repo_url":    "https://github.com/acme/repo",
		"branch_slug": "feature-x",
	})
	got := systemsForComponentConfig(cc)
	if len(got) != 1 || got[0] != "github-com-acme-repo-feature-x" {
		t.Errorf("systems = %v, want [github-com-acme-repo-feature-x]", got)
	}
}

func TestSystemsForComponentConfig_UnknownFactory(t *testing.T) {
	cc := mustComponentConfig(t, "vision", map[string]any{})
	if got := systemsForComponentConfig(cc); got != nil {
		t.Errorf("systems = %v, want nil for an unrecognized factory", got)
	}
}

func TestSystemsForRemovedInstance_UnknownInstance(t *testing.T) {
	if got := systemsForRemovedInstance("no-such-instance", nil); got != nil {
		t.Errorf("systems = %v, want nil for a nil store", got)
	}
}
