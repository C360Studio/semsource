package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgfile "github.com/c360studio/semsource/handler/cfgfile"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// tripleObject returns the string Object of the first triple in state matching
// pred, or "" if not found or Object is not a string.
func tripleObject(state *handler.EntityState, pred string) string {
	for _, tr := range state.Triples {
		if tr.Predicate == pred {
			if s, ok := tr.Object.(string); ok {
				return s
			}
		}
	}
	return ""
}

// findStateByType returns the first EntityState whose 6-part ID has the given
// entity-type segment (index 4 in the dot-split ID).
func findStateByType(states []*handler.EntityState, entityType string) *handler.EntityState {
	for _, s := range states {
		parts := strings.Split(s.ID, ".")
		if len(parts) >= 5 && parts[4] == entityType {
			return s
		}
	}
	return nil
}

// collectTriplesByPred collects all string triple Objects across all states
// that match pred. Non-string Object values are skipped.
func collectTriplesByPred(states []*handler.EntityState, pred string) []string {
	var out []string
	for _, s := range states {
		for _, tr := range s.Triples {
			if tr.Predicate == pred {
				if str, ok := tr.Object.(string); ok {
					out = append(out, str)
				}
			}
		}
	}
	return out
}

// TestIngestEntityStates_GoMod verifies that go.mod produces entity states with
// correct 6-part IDs and vocabulary predicates.
func TestIngestEntityStates_GoMod(t *testing.T) {
	dir := t.TempDir()
	gomod := `module github.com/example/myapp

go 1.21

require (
	github.com/some/dep v1.2.3
	github.com/another/lib v0.5.0 // indirect
)
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("expected entity states from go.mod")
	}

	// Module entity — type segment is "gomod".
	modState := findStateByType(states, "gomod")
	if modState == nil {
		t.Fatal("expected a gomod entity state")
	}
	if !strings.HasPrefix(modState.ID, "acme.semsource.config.") {
		t.Errorf("module entity ID %q does not start with acme.semsource.config.", modState.ID)
	}
	if got := tripleObject(modState, source.ConfigModulePath); got != "github.com/example/myapp" {
		t.Errorf("ConfigModulePath = %q, want %q", got, "github.com/example/myapp")
	}
	if got := tripleObject(modState, source.ConfigModuleGoVer); got != "1.21" {
		t.Errorf("ConfigModuleGoVer = %q, want %q", got, "1.21")
	}

	// Requires relationship triples must appear on the module entity and point at
	// dependency entity IDs that exist in the state slice.
	ids := make(map[string]bool, len(states))
	for _, s := range states {
		ids[s.ID] = true
	}
	requiresTargets := collectTriplesByPred([]*handler.EntityState{modState}, source.ConfigRequires)
	if len(requiresTargets) < 2 {
		t.Errorf("expected at least 2 requires triples on module entity, got %d", len(requiresTargets))
	}
	for _, target := range requiresTargets {
		if !ids[target] {
			t.Errorf("requires target %q is not a known entity ID", target)
		}
	}

	// At least one dependency entity must carry ConfigDepIndirect.
	depStates := collectByType(states, "dependency")
	var hasIndirect bool
	for _, s := range depStates {
		if tripleObject(s, source.ConfigDepIndirect) != "" {
			hasIndirect = true
		}
	}
	if !hasIndirect {
		t.Error("expected at least one dependency entity with ConfigDepIndirect triple")
	}
}

// TestIngestEntityStates_PackageJSON verifies npm package entity states.
func TestIngestEntityStates_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
  "name": "my-app",
  "version": "2.0.0",
  "dependencies": {
    "react": "^18.0.0"
  },
  "devDependencies": {
    "eslint": "^8.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("expected entity states from package.json")
	}

	pkgState := findStateByType(states, "package")
	if pkgState == nil {
		t.Fatal("expected a package entity state")
	}
	if !strings.HasPrefix(pkgState.ID, "acme.semsource.config.") {
		t.Errorf("package entity ID %q wrong prefix", pkgState.ID)
	}
	if got := tripleObject(pkgState, source.ConfigPkgName); got != "my-app" {
		t.Errorf("ConfigPkgName = %q, want \"my-app\"", got)
	}
	if got := tripleObject(pkgState, source.ConfigPkgVersion); got != "2.0.0" {
		t.Errorf("ConfigPkgVersion = %q, want \"2.0.0\"", got)
	}

	// Package must have depends relationship triples.
	dependsTargets := collectTriplesByPred([]*handler.EntityState{pkgState}, source.ConfigDepends)
	if len(dependsTargets) < 2 {
		t.Errorf("expected at least 2 depends triples (1 prod + 1 dev), got %d", len(dependsTargets))
	}

	// At least one dependency must have kind=npm-dev.
	depStates := collectByType(states, "dependency")
	var hasDevKind bool
	for _, s := range depStates {
		if tripleObject(s, source.ConfigDepKind) == "npm-dev" {
			hasDevKind = true
		}
	}
	if !hasDevKind {
		t.Error("expected at least one dependency entity with kind=npm-dev")
	}
}

// TestIngestEntityStates_Dockerfile verifies Dockerfile image entity states.
func TestIngestEntityStates_Dockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM golang:1.21-alpine AS builder
FROM alpine:3.18
EXPOSE 8080
`
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	imageStates := collectByType(states, "image")
	if len(imageStates) < 2 {
		t.Errorf("expected at least 2 image entity states (2 FROM), got %d", len(imageStates))
	}
	for _, s := range imageStates {
		if tripleObject(s, source.ConfigImageName) == "" {
			t.Errorf("image entity %q missing ConfigImageName triple", s.ID)
		}
		if tripleObject(s, source.ConfigFilePath) == "" {
			t.Errorf("image entity %q missing ConfigFilePath triple", s.ID)
		}
	}
}

// TestIngestEntityStates_PomXml verifies Maven project entity states.
func TestIngestEntityStates_PomXml(t *testing.T) {
	dir := t.TempDir()
	pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>demo</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
  <dependencies>
    <dependency>
      <groupId>junit</groupId>
      <artifactId>junit</artifactId>
      <version>4.13</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
</project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	projState := findStateByType(states, "project")
	if projState == nil {
		t.Fatal("expected a project entity state from pom.xml")
	}

	for _, pred := range []string{
		source.ConfigProjectGroup,
		source.ConfigProjectArtifact,
		source.ConfigProjectVersion,
		source.ConfigProjectPackaging,
		source.ConfigFilePath,
	} {
		if tripleObject(projState, pred) == "" {
			t.Errorf("project entity missing predicate %q", pred)
		}
	}

	// Project entity requires relationship must reference the dependency.
	requiresTargets := collectTriplesByPred([]*handler.EntityState{projState}, source.ConfigRequires)
	if len(requiresTargets) < 1 {
		t.Errorf("expected at least 1 requires triple on project entity, got %d", len(requiresTargets))
	}

	// Dependency must carry scope=test.
	depStates := collectByType(states, "dependency")
	var hasTestScope bool
	for _, s := range depStates {
		if tripleObject(s, source.ConfigDepScope) == "test" {
			hasTestScope = true
		}
	}
	if !hasTestScope {
		t.Error("expected dependency entity with ConfigDepScope=test")
	}
}

// TestIngestEntityStates_BuildGradle verifies Gradle project entity states.
func TestIngestEntityStates_BuildGradle(t *testing.T) {
	dir := t.TempDir()
	gradle := `dependencies {
    implementation 'org.springframework:spring-core:5.3.21'
    testImplementation 'junit:junit:4.13'
}
`
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(gradle), 0644); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	projState := findStateByType(states, "project")
	if projState == nil {
		t.Fatal("expected a project entity state from build.gradle")
	}
	if tripleObject(projState, source.ConfigProjectBuild) != "gradle" {
		t.Errorf("project entity missing ConfigProjectBuild=gradle triple")
	}

	// Dependency entities must carry ConfigDepConfiguration.
	depStates := collectByType(states, "dependency")
	var hasConfig bool
	for _, s := range depStates {
		if tripleObject(s, source.ConfigDepConfiguration) != "" {
			hasConfig = true
		}
	}
	if !hasConfig {
		t.Error("expected at least one dependency entity with ConfigDepConfiguration triple")
	}

	// Requires relationship triples from project to dependencies.
	requiresTargets := collectTriplesByPred([]*handler.EntityState{projState}, source.ConfigRequires)
	if len(requiresTargets) < 2 {
		t.Errorf("expected at least 2 requires triples on project entity, got %d", len(requiresTargets))
	}
}

// TestIngestEntityStates_Deterministic verifies that calling IngestEntityStates
// twice on the same input produces identical entity IDs.
func TestIngestEntityStates_Deterministic(t *testing.T) {
	dir := t.TempDir()
	gomod := "module github.com/example/app\n\ngo 1.21\n\nrequire github.com/pkg/errors v0.9.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}

	states1, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates (first): %v", err)
	}
	states2, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates (second): %v", err)
	}

	if len(states1) != len(states2) {
		t.Fatalf("entity count mismatch: %d vs %d", len(states1), len(states2))
	}
	for i := range states1 {
		if states1[i].ID != states2[i].ID {
			t.Errorf("states[%d] ID not deterministic: %q vs %q", i, states1[i].ID, states2[i].ID)
		}
	}
}

// TestIngestEntityStates_OrgIsolation verifies that two different orgs produce
// distinct entity IDs from the same source file.
func TestIngestEntityStates_OrgIsolation(t *testing.T) {
	dir := t.TempDir()
	gomod := "module github.com/example/app\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}

	statesA, _ := h.IngestEntityStates(context.Background(), cfg, "acme")
	statesB, _ := h.IngestEntityStates(context.Background(), cfg, "beta")

	if len(statesA) == 0 || len(statesB) == 0 {
		t.Fatal("expected entity states for both orgs")
	}
	if statesA[0].ID == statesB[0].ID {
		t.Errorf("different orgs produced same entity ID %q", statesA[0].ID)
	}
	if !strings.HasPrefix(statesA[0].ID, "acme.") {
		t.Errorf("acme entity ID %q does not start with \"acme.\"", statesA[0].ID)
	}
	if !strings.HasPrefix(statesB[0].ID, "beta.") {
		t.Errorf("beta entity ID %q does not start with \"beta.\"", statesB[0].ID)
	}
}

// collectByType returns all EntityState values whose ID has the given entity
// type at position 4 (0-indexed) in the dot-split 6-part ID.
func collectByType(states []*handler.EntityState, entityType string) []*handler.EntityState {
	var out []*handler.EntityState
	for _, s := range states {
		parts := strings.Split(s.ID, ".")
		if len(parts) >= 5 && parts[4] == entityType {
			out = append(out, s)
		}
	}
	return out
}
