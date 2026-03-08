package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cfgfile "github.com/c360studio/semsource/handler/cfgfile"
	"github.com/c360studio/semsource/handler"
)

// stubSourceConfig adapts test values to handler.SourceConfig.
type stubSourceConfig struct {
	sourceType string
	path       string
	watch      bool
}

func (s *stubSourceConfig) GetType() string            { return s.sourceType }
func (s *stubSourceConfig) GetPath() string            { return s.path }
func (s *stubSourceConfig) GetPaths() []string         { return nil }
func (s *stubSourceConfig) GetURL() string             { return "" }
func (s *stubSourceConfig) GetBranch() string          { return "" }
func (s *stubSourceConfig) IsWatchEnabled() bool       { return s.watch }
func (s *stubSourceConfig) GetKeyframeMode() string    { return "" }
func (s *stubSourceConfig) GetKeyframeInterval() string { return "" }
func (s *stubSourceConfig) GetSceneThreshold() float64 { return 0 }

// stubMultiPathSourceConfig is like stubSourceConfig but returns multiple paths
// from GetPaths() and returns "" from GetPath(), exercising the multi-path code path.
type stubMultiPathSourceConfig struct {
	sourceType string
	paths      []string
	watch      bool
}

func (s *stubMultiPathSourceConfig) GetType() string             { return s.sourceType }
func (s *stubMultiPathSourceConfig) GetPath() string             { return "" }
func (s *stubMultiPathSourceConfig) GetPaths() []string          { return s.paths }
func (s *stubMultiPathSourceConfig) GetURL() string              { return "" }
func (s *stubMultiPathSourceConfig) GetBranch() string           { return "" }
func (s *stubMultiPathSourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s *stubMultiPathSourceConfig) GetKeyframeMode() string     { return "" }
func (s *stubMultiPathSourceConfig) GetKeyframeInterval() string { return "" }
func (s *stubMultiPathSourceConfig) GetSceneThreshold() float64  { return 0 }

var _ handler.SourceHandler = (*cfgfile.ConfigHandler)(nil)

func TestConfigHandler_SourceType(t *testing.T) {
	h := cfgfile.New(nil)
	if got := h.SourceType(); got != handler.SourceTypeConfig {
		t.Errorf("SourceType() = %q, want %q", got, handler.SourceTypeConfig)
	}
}

func TestConfigHandler_Supports(t *testing.T) {
	h := cfgfile.New(nil)

	tests := []struct {
		name string
		cfg  handler.SourceConfig
		want bool
	}{
		{
			name: "config type with existing path",
			cfg:  &stubSourceConfig{sourceType: "config", path: t.TempDir()},
			want: true,
		},
		{
			name: "wrong type",
			cfg:  &stubSourceConfig{sourceType: "git", path: t.TempDir()},
			want: false,
		},
		{
			name: "empty path",
			cfg:  &stubSourceConfig{sourceType: "config", path: ""},
			want: false,
		},
		{
			name: "nonexistent path",
			cfg:  &stubSourceConfig{sourceType: "config", path: "/tmp/semsource-test-nonexistent-path-xyz"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.Supports(tt.cfg); got != tt.want {
				t.Errorf("Supports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigHandler_Ingest_GoMod(t *testing.T) {
	dir := t.TempDir()
	gomod := `module github.com/example/myapp

go 1.21

require (
	github.com/some/dep v1.2.3
	github.com/another/lib v0.5.0
)
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity from go.mod")
	}

	// Should have a module entity
	var modEntity *handler.RawEntity
	for i := range entities {
		if entities[i].EntityType == "module" {
			modEntity = &entities[i]
			break
		}
	}
	if modEntity == nil {
		t.Fatal("expected a 'module' entity from go.mod")
	}
	if modEntity.Domain != handler.DomainConfig {
		t.Errorf("Domain = %q, want %q", modEntity.Domain, handler.DomainConfig)
	}
	if modEntity.Instance == "" {
		t.Error("Instance must not be empty")
	}
	if modEntity.System == "" {
		t.Error("System must not be empty")
	}

	// Should have dependency entities
	var depCount int
	for _, e := range entities {
		if e.EntityType == "dependency" {
			depCount++
		}
	}
	if depCount < 2 {
		t.Errorf("expected at least 2 dependency entities, got %d", depCount)
	}
}

func TestConfigHandler_Ingest_PackageJSON(t *testing.T) {
	dir := t.TempDir()
	pkgJSON := `{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.0.0",
    "typescript": "^5.0.0"
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
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity from package.json")
	}

	// Should have a package entity
	var pkgEntity *handler.RawEntity
	for i := range entities {
		if entities[i].EntityType == "package" {
			pkgEntity = &entities[i]
			break
		}
	}
	if pkgEntity == nil {
		t.Fatal("expected a 'package' entity from package.json")
	}
	if pkgEntity.Instance == "" {
		t.Error("Instance must not be empty")
	}

	// Should have dependency entities
	var depCount int
	for _, e := range entities {
		if e.EntityType == "dependency" {
			depCount++
		}
	}
	if depCount < 3 {
		t.Errorf("expected at least 3 dependency entities (2 prod + 1 dev), got %d", depCount)
	}
}

func TestConfigHandler_Ingest_Dockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfile := `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /app/server ./cmd/server

FROM alpine:3.18
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]
`
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity from Dockerfile")
	}

	// Should have image entities (FROM directives)
	var imageCount int
	for _, e := range entities {
		if e.EntityType == "image" {
			imageCount++
		}
	}
	if imageCount < 2 {
		t.Errorf("expected at least 2 image entities (2 FROM), got %d", imageCount)
	}
}

func TestConfigHandler_Ingest_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities from empty dir, got %d", len(entities))
	}
}

func TestConfigHandler_Ingest_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	// Write a go.mod so there's something to find
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.21\n"), 0644)

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	// Should not panic; may return empty or partial results
	_, err := h.Ingest(ctx, cfg)
	// Cancelled context is not necessarily an error for local file reads
	_ = err
}

func TestConfigHandler_Ingest_EntityIDs_Deterministic(t *testing.T) {
	dir := t.TempDir()
	gomod := "module github.com/example/app\n\ngo 1.21\n\nrequire github.com/pkg/errors v0.9.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}

	entities1, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest (first): %v", err)
	}
	entities2, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest (second): %v", err)
	}

	if len(entities1) != len(entities2) {
		t.Errorf("entity count mismatch: %d vs %d", len(entities1), len(entities2))
	}
	for i, e1 := range entities1 {
		e2 := entities2[i]
		if e1.Instance != e2.Instance {
			t.Errorf("entity[%d] Instance not deterministic: %q vs %q", i, e1.Instance, e2.Instance)
		}
	}
}

func TestConfigHandler_Ingest_PomXml(t *testing.T) {
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
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>5.3.21</version>
    </dependency>
  </dependencies>
</project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Expect a project entity.
	var projectEntity *handler.RawEntity
	for i := range entities {
		if entities[i].EntityType == "project" {
			projectEntity = &entities[i]
			break
		}
	}
	if projectEntity == nil {
		t.Fatal("expected a 'project' entity from pom.xml")
	}
	if projectEntity.Instance == "" {
		t.Error("project entity Instance must not be empty")
	}
	if projectEntity.Domain != handler.DomainConfig {
		t.Errorf("Domain = %q, want %q", projectEntity.Domain, handler.DomainConfig)
	}
	if projectEntity.System == "" {
		t.Error("System must not be empty")
	}

	// Expect two dependency entities.
	var depCount int
	for _, e := range entities {
		if e.EntityType == "dependency" {
			depCount++
		}
	}
	if depCount != 2 {
		t.Errorf("expected 2 dependency entities, got %d", depCount)
	}

	// Edges should be on the project entity, not on dependencies.
	if len(projectEntity.Edges) < 2 {
		t.Errorf("project entity should have at least 2 edges, got %d", len(projectEntity.Edges))
	}
}

func TestConfigHandler_Ingest_BuildGradle(t *testing.T) {
	dir := t.TempDir()
	gradle := `plugins {
    id 'java'
}

dependencies {
    implementation 'org.springframework:spring-core:5.3.21'
    testImplementation 'junit:junit:4.13'
    api 'com.google.guava:guava:31.1-jre'
}
`
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(gradle), 0644); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Expect a project entity.
	var projectEntity *handler.RawEntity
	for i := range entities {
		if entities[i].EntityType == "project" {
			projectEntity = &entities[i]
			break
		}
	}
	if projectEntity == nil {
		t.Fatal("expected a 'project' entity from build.gradle")
	}
	if projectEntity.Instance == "" {
		t.Error("project entity Instance must not be empty")
	}

	// Expect three dependency entities.
	var depCount int
	for _, e := range entities {
		if e.EntityType == "dependency" {
			depCount++
		}
	}
	if depCount != 3 {
		t.Errorf("expected 3 dependency entities, got %d", depCount)
	}

	// Edges should be on the project entity, not on dependencies.
	if len(projectEntity.Edges) < 3 {
		t.Errorf("project entity should have at least 3 edges, got %d", len(projectEntity.Edges))
	}
}

func TestConfigHandler_Supports_PomXml(t *testing.T) {
	dir := t.TempDir()
	pom := `<?xml version="1.0"?><project><groupId>g</groupId><artifactId>a</artifactId><version>1</version></project>`
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0644); err != nil {
		t.Fatalf("write pom.xml: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	if !h.Supports(cfg) {
		t.Error("Supports() = false, want true when pom.xml exists in path")
	}
}

func TestConfigHandler_Watch_NilWhenDisabled(t *testing.T) {
	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: t.TempDir(), watch: false}
	ch, err := h.Watch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch != nil {
		t.Error("Watch should return nil channel when watch disabled")
	}
}

func TestConfigHandler_Ingest_MultiplePaths(t *testing.T) {
	// First root: contains a go.mod.
	dir1 := t.TempDir()
	gomod := "module github.com/example/alpha\n\ngo 1.21\n\nrequire github.com/pkg/errors v0.9.0\n"
	if err := os.WriteFile(filepath.Join(dir1, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Second root: contains a package.json.
	dir2 := t.TempDir()
	pkgJSON := `{"name":"beta","version":"2.0.0","dependencies":{"lodash":"^4.0.0"}}`
	if err := os.WriteFile(filepath.Join(dir2, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	h := cfgfile.New(nil)
	cfg := &stubMultiPathSourceConfig{
		sourceType: "config",
		paths:      []string{dir1, dir2},
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// Expect at least a module entity (from dir1) and a package entity (from dir2).
	var hasModule, hasPackage bool
	for _, e := range entities {
		switch e.EntityType {
		case "module":
			hasModule = true
		case "package":
			hasPackage = true
		}
	}
	if !hasModule {
		t.Error("expected a 'module' entity from dir1 (go.mod), none found")
	}
	if !hasPackage {
		t.Error("expected a 'package' entity from dir2 (package.json), none found")
	}
}
