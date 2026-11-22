package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
)

// compositionFingerprints records, per composition generation, the fingerprint
// of what buildSemstreamsConfig generates from fingerprintFixture. Keep earlier
// generations' entries: they are the history of the generated wiring.
var compositionFingerprints = map[int]string{
	3: "0a998ae19901b05af18e26f70fbbb105aa49f94817485dfb7eb8e3268ed4ca08",
	4: "7115044e7482f2710561d33dcb41b11b31e33dba9a8eea31776aa00e31575075",
	5: "cb3737fb36bdc0d16cf26b02c85b79c5f584b958ffd4a1e1a438ff87e3d0bad9",
	// 6 added the opt-in tier 1 composition and left tier 0 unchanged, so it
	// records generation 5's fingerprint (TestTierZeroWiringUnchangedByTierOne).
	6: "cb3737fb36bdc0d16cf26b02c85b79c5f584b958ffd4a1e1a438ff87e3d0bad9",
}

// tierOneCompositionFingerprints records, per composition generation from 6,
// the fingerprint of what buildSemstreamsConfig generates from
// tierOneFingerprintFixture, so a change to the tier 1 wiring needs a
// generation bump as a tier 0 change does.
var tierOneCompositionFingerprints = map[int]string{
	6: "a5b28d6e85989ea743601a4b4f366e51c8588d91eb0ae4d74893fc794e865ed5",
}

// fingerprintFixture is a fixed configuration with every field the generator
// reads set explicitly, so nothing comes from applyDefaults' environment and
// home-directory lookups. The git source names its branch, so no remote
// default-branch lookup runs. It covers each source type the generator builds
// without I/O, on the default graph subsystem.
//
// shortcut: one fixture on the default branches; a generator change reached
// only through lexical_search false, clustering, metrics, stream overrides, or
// s3/media sources passes unseen. Add a fixture when one of those changes.
func fingerprintFixture() *config.Config {
	return &config.Config{
		Namespace:     "acme",
		PlatformID:    "semsource",
		WorkspaceDir:  "/var/lib/semsource/repos",
		HTTPPort:      8080,
		WebSocketBind: "0.0.0.0:7890",
		WebSocketPath: "/ws",
		Sources: []config.SourceEntry{
			{Type: "ast", Path: "./src", Language: "go", Watch: true},
			{Type: "git", URL: "github.com/acme/widgets", Branch: "main", Watch: true},
			{Type: "docs", Paths: []string{"docs/", "README.md"}, Watch: true},
			{Type: "config", Paths: []string{"go.mod"}, Watch: true},
			{Type: "url", URLs: []string{"https://docs.acme.example/widgets"}, PollInterval: "300s"},
		},
	}
}

// tierOneFingerprintFixture is fingerprintFixture with graph.semantic_search on
// and the embedding capability bound to semembed, as the shipped tier 1 config.
func tierOneFingerprintFixture() *config.Config {
	cfg := fingerprintFixture()
	on := true
	cfg.Graph = &config.GraphConfig{SemanticSearch: &on}
	cfg.ModelRegistry = &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"semembed": {Provider: "openai", URL: "http://semembed:8081/v1", Model: "Snowflake/snowflake-arctic-embed-s"},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			model.CapabilityEmbedding: {Preferred: []string{"semembed"}},
		},
	}
	return cfg
}

// compositionFingerprint hashes the sections of the generated composition that
// the config bucket retains and a later boot reads back in place of the file's:
// services, components, nats and model_registry. The version is left out
// because it is what the fingerprint guards. Platform is left out because the
// bucket keeps it only as a published mirror and never applies it. Streams and
// security are left out because they never enter the bucket, so the file's
// values apply on every boot whatever the version.
func compositionFingerprint(t *testing.T, ssCfg *semconfig.Config) string {
	t.Helper()
	retained := struct {
		Services      types.ServiceConfigs       `json:"services"`
		Components    semconfig.ComponentConfigs `json:"components"`
		NATS          semconfig.NATSConfig       `json:"nats"`
		ModelRegistry *model.Registry            `json:"model_registry"`
	}{ssCfg.Services, ssCfg.Components, ssCfg.NATS, ssCfg.ModelRegistry}
	// Decoding and re-encoding sorts every object's keys, including those
	// inside each component's raw config.
	decoded, ok := decodedJSON(retained)
	if !ok {
		t.Fatal("generated composition does not encode as JSON")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("encode canonical composition: %v", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// spec: openspec/specs/runtime-configuration/spec.md#Requirement: A change to the generated composition requires a new generation
func TestCompositionFingerprintMatchesGeneration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fixture  *config.Config
		recorded map[int]string
		mapName  string
	}{
		{name: "tier 0", fixture: fingerprintFixture(), recorded: compositionFingerprints, mapName: "compositionFingerprints"},
		{name: "tier 1", fixture: tierOneFingerprintFixture(), recorded: tierOneCompositionFingerprints,
			mapName: "tierOneCompositionFingerprints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssCfg, err := buildSemstreamsConfig(tc.fixture, "acme")
			if err != nil {
				t.Fatalf("buildSemstreamsConfig: %v", err)
			}
			got := compositionFingerprint(t, ssCfg)
			recorded, ok := tc.recorded[compositionGeneration]
			if !ok || recorded == "" {
				t.Fatalf("no fingerprint recorded for composition generation %d: record %q under %d in %s",
					compositionGeneration, got, compositionGeneration, tc.mapName)
			}
			if got != recorded {
				t.Fatalf("the generated composition changed without a generation bump (fingerprint %s, recorded %s for generation %d). "+
					"A retained store keeps the previous wiring until the composition version is newer: "+
					"bump compositionGeneration to %d in cmd/semsource/run.go and record %q under %d in %s",
					got, recorded, compositionGeneration, compositionGeneration+1, got, compositionGeneration+1, tc.mapName)
			}
		})
	}
}

// TestTierZeroWiringUnchangedByTierOne pins that adding tier 1 left the default
// composition byte-identical: a retained tier 0 store adopts generation 6 with
// the wiring it already runs.
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 is an explicit opt-in neural composition
func TestTierZeroWiringUnchangedByTierOne(t *testing.T) {
	if compositionFingerprints[6] != compositionFingerprints[5] {
		t.Fatalf("tier 0 fingerprint at generation 6 is %s, generation 5 recorded %s; tier 1 must not change the default composition",
			compositionFingerprints[6], compositionFingerprints[5])
	}
}

// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The composition version orders the binary and the operator
func TestBuildSemstreamsConfigVersion(t *testing.T) {
	// Generation 6. A generation bump updates these expectations with it.
	tests := []struct {
		name     string
		revision int
		want     string
	}{
		{name: "no revision keeps the generation's version", revision: 0, want: "6.0.0"},
		{name: "revision is the minor position", revision: 2, want: "6.2.0"},
		{name: "multi-digit revision", revision: 12, want: "6.12.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fingerprintFixture()
			cfg.ConfigRevision = tc.revision
			ssCfg, err := buildSemstreamsConfig(cfg, "acme")
			if err != nil {
				t.Fatalf("buildSemstreamsConfig: %v", err)
			}
			if ssCfg.Version != tc.want {
				t.Fatalf("Version = %q, want %q", ssCfg.Version, tc.want)
			}
		})
	}
}
