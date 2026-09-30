package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

func scopeComponent(factory, raw string) types.ComponentConfig {
	return types.ComponentConfig{Name: factory, Enabled: true, Config: json.RawMessage(raw)}
}

func TestSourceScopeSystemsIncludesEveryAdmittedSource(t *testing.T) {
	components := semconfig.ComponentConfigs{
		"ast":             scopeComponent("ast-source", `{"watch_paths":[{"project":"Core/API","version":"v1.2"},{"project":"Core/API","version":"v1.2"},{"project":"other","version":""}]}`),
		"docs-paths":      scopeComponent("doc-source", `{"paths":["/workspace/Manuals","/workspace/Guides"]}`),
		"docs-project":    scopeComponent("doc-source", `{"paths":["/ignored/Path"],"project":"Canonical/Docs"}`),
		"config-paths":    scopeComponent("cfgfile-source", `{"paths":["/workspace/PackageConfig"]}`),
		"config-project":  scopeComponent("cfgfile-source", `{"paths":["/ignored/Path"],"project":"Canonical/Config"}`),
		"urls":            scopeComponent("url-source", `{"urls":["https://EXAMPLE.org:8443/guide","https://docs.example.org/other","https://example.org/again"]}`),
		"objects-project": scopeComponent("objectstore-source", `{"bucket":"ignored","project":"Reports/API","version":"v2.0"}`),
		"objects-bucket":  scopeComponent("objectstore-source", `{"bucket":"ReportsBucket","version":"2026.09"}`),
		"git":             scopeComponent("git-source", `{"url":"https://example.org/ignored.git"}`),
		"media":           scopeComponent("image-source", `{"paths":["/ignored/media"]}`),
	}
	got, err := sourceScopeSystems(components)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"code": {"Core-API-v1-2", "other"},
		"docs": {"Canonical-Docs", "Guides", "Manuals", "Reports-API-v2-0", "ReportsBucket-2026-09", "canonical-config", "docs-example-org", "example-org", "packageconfig"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scope systems=%v; want %v", got, want)
	}
}

func TestSourceScopeSystemsUsesPersistedNextBootSources(t *testing.T) {
	// The file config need not know a source registered at runtime. The caller
	// supplies the effective desired map selected from KV after Manager.Start.
	desired := semconfig.ComponentConfigs{
		"file-docs":        scopeComponent("doc-source", `{"paths":["/workspace/manual"]}`),
		"registered-later": scopeComponent("ast-source", `{"watch_paths":[{"project":"runtime/service","version":"v3"}]}`),
	}
	persisted, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	var nextBoot semconfig.ComponentConfigs
	if err = json.Unmarshal(persisted, &nextBoot); err != nil {
		t.Fatal(err)
	}
	got, err := sourceScopeSystems(nextBoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["code"], []string{"runtime-service-v3"}) || !reflect.DeepEqual(got["docs"], []string{"manual"}) {
		t.Fatalf("persisted source absent from next-boot lens scopes: %v", got)
	}
	// Removing desired activation must not retain a stale source prefix. An
	// empty slice lets the factory install its explicit deny-all scope.
	disabled := nextBoot["registered-later"]
	disabled.Enabled = false
	nextBoot["registered-later"] = disabled
	got, err = sourceScopeSystems(nextBoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["code"]) != 0 {
		t.Fatalf("disabled source remains admitted: %v", got)
	}
}

func TestSourceScopeSystemsRejectsMalformedAdmittedConfig(t *testing.T) {
	for _, factory := range []string{"ast-source", "doc-source", "cfgfile-source", "url-source", "objectstore-source"} {
		t.Run(factory, func(t *testing.T) {
			_, err := sourceScopeSystems(semconfig.ComponentConfigs{"broken": scopeComponent(factory, `{"paths":`)})
			if err == nil {
				t.Fatal("malformed admitted config did not fail closed")
			}
		})
	}
}

func TestSourceScopeSystemsIgnoresNonAdmittedAndDisabledConfigs(t *testing.T) {
	disabled := scopeComponent("ast-source", `broken`)
	disabled.Enabled = false
	got, err := sourceScopeSystems(semconfig.ComponentConfigs{"disabled": disabled, "unrelated": scopeComponent("graph-query", `broken`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got["code"]) != 0 || len(got["docs"]) != 0 {
		t.Fatalf("non-admitted sources expanded scope: %v", got)
	}
}

func TestSourceScopeSystemsBoundsURLHostname(t *testing.T) {
	host := strings.Repeat("a", 60) + "." + strings.Repeat("b", 39) + ".example"
	raw, err := json.Marshal(map[string][]string{"urls": {"https://" + host + "/path"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := sourceScopeSystems(semconfig.ComponentConfigs{"url": scopeComponent("url-source", string(raw))})
	if err != nil {
		t.Fatal(err)
	}
	// Same frozen hostname/system pair asserted by the URL entity producer.
	want := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbb-1c9468"
	if !reflect.DeepEqual(got["docs"], []string{want}) {
		t.Fatalf("URL scope %v does not match producer system %q", got["docs"], want)
	}
}
