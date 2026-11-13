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
}

// fingerprintFixture is a fixed configuration with every field the generator
// reads set explicitly, so nothing comes from applyDefaults' environment and
// home-directory lookups. The git source names its branch, so no remote
// default-branch lookup runs. It covers each source type the generator builds
// without I/O, on the default graph subsystem.
//
// shortcut: one fixture on the default branches; a generator change reached
// only through embedder_type none, clustering, metrics, stream overrides, or
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
	ssCfg, err := buildSemstreamsConfig(fingerprintFixture(), "acme")
	if err != nil {
		t.Fatalf("buildSemstreamsConfig: %v", err)
	}
	got := compositionFingerprint(t, ssCfg)
	recorded, ok := compositionFingerprints[compositionGeneration]
	if !ok {
		t.Fatalf("no fingerprint recorded for composition generation %d: record %q under %d in compositionFingerprints",
			compositionGeneration, got, compositionGeneration)
	}
	if got != recorded {
		t.Fatalf("the generated composition changed without a generation bump (fingerprint %s, recorded %s for generation %d). "+
			"A retained store keeps the previous wiring until the composition version is newer: "+
			"bump compositionGeneration to %d in cmd/semsource/run.go and record %q under %d in compositionFingerprints",
			got, recorded, compositionGeneration, compositionGeneration+1, got, compositionGeneration+1)
	}
}

// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The composition version orders the binary and the operator
func TestBuildSemstreamsConfigVersion(t *testing.T) {
	// Generation 3. A generation bump updates these expectations with it.
	tests := []struct {
		name     string
		revision int
		want     string
	}{
		{name: "no revision keeps the generation's version", revision: 0, want: "3.0.0"},
		{name: "revision is the minor position", revision: 2, want: "3.2.0"},
		{name: "multi-digit revision", revision: 12, want: "3.12.0"},
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
