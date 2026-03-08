package cfgfile_test

import (
	"testing"

	cfgfile "github.com/c360studio/semsource/handler/cfgfile"
)

func TestParseGoMod(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantModule   string
		wantGoVer    string
		wantDepCount int
		wantErr      bool
	}{
		{
			name: "full go.mod",
			content: `module github.com/example/app

go 1.21

require (
	github.com/some/dep v1.2.3
	github.com/another/lib v0.5.0 // indirect
)
`,
			wantModule:   "github.com/example/app",
			wantGoVer:    "1.21",
			wantDepCount: 2,
		},
		{
			name: "minimal go.mod",
			content: `module example.com/simple

go 1.20
`,
			wantModule:   "example.com/simple",
			wantGoVer:    "1.20",
			wantDepCount: 0,
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cfgfile.ParseGoMod([]byte(tt.content))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGoMod: %v", err)
			}
			if result.Module != tt.wantModule {
				t.Errorf("Module = %q, want %q", result.Module, tt.wantModule)
			}
			if result.GoVersion != tt.wantGoVer {
				t.Errorf("GoVersion = %q, want %q", result.GoVersion, tt.wantGoVer)
			}
			if len(result.Deps) != tt.wantDepCount {
				t.Errorf("len(Deps) = %d, want %d", len(result.Deps), tt.wantDepCount)
			}
		})
	}
}

func TestParsePackageJSON(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantName     string
		wantVersion  string
		wantDepCount int
		wantErr      bool
	}{
		{
			name: "full package.json",
			content: `{
  "name": "my-app",
  "version": "2.0.0",
  "dependencies": {
    "react": "^18.0.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "jest": "^29.0.0"
  }
}`,
			wantName:     "my-app",
			wantVersion:  "2.0.0",
			wantDepCount: 3,
		},
		{
			name: "no deps",
			content: `{
  "name": "simple",
  "version": "1.0.0"
}`,
			wantName:     "simple",
			wantVersion:  "1.0.0",
			wantDepCount: 0,
		},
		{
			name:    "invalid json",
			content: `not valid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cfgfile.ParsePackageJSON([]byte(tt.content))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePackageJSON: %v", err)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", result.Version, tt.wantVersion)
			}
			if len(result.Deps) != tt.wantDepCount {
				t.Errorf("len(Deps) = %d, want %d", len(result.Deps), tt.wantDepCount)
			}
		})
	}
}

func TestParsePOM_BasicProject(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <groupId>org.example</groupId>
  <artifactId>my-app</artifactId>
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

	result, err := cfgfile.ParsePOM([]byte(content))
	if err != nil {
		t.Fatalf("ParsePOM: %v", err)
	}
	if result.GroupID != "org.example" {
		t.Errorf("GroupID = %q, want %q", result.GroupID, "org.example")
	}
	if result.ArtifactID != "my-app" {
		t.Errorf("ArtifactID = %q, want %q", result.ArtifactID, "my-app")
	}
	if result.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Packaging != "jar" {
		t.Errorf("Packaging = %q, want %q", result.Packaging, "jar")
	}
	if len(result.Deps) != 2 {
		t.Fatalf("len(Deps) = %d, want 2", len(result.Deps))
	}
	// First dep has an explicit scope.
	if result.Deps[0].Scope != "test" {
		t.Errorf("Deps[0].Scope = %q, want %q", result.Deps[0].Scope, "test")
	}
	// Second dep has no scope — should default to "compile".
	if result.Deps[1].Scope != "compile" {
		t.Errorf("Deps[1].Scope = %q, want %q (default)", result.Deps[1].Scope, "compile")
	}
}

func TestParsePOM_MultiModule(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>parent</artifactId>
  <version>2.0.0</version>
  <packaging>pom</packaging>
  <modules>
    <module>child-a</module>
    <module>child-b</module>
  </modules>
</project>`

	result, err := cfgfile.ParsePOM([]byte(content))
	if err != nil {
		t.Fatalf("ParsePOM: %v", err)
	}
	if len(result.Modules) != 2 {
		t.Fatalf("len(Modules) = %d, want 2", len(result.Modules))
	}
	if result.Modules[0] != "child-a" {
		t.Errorf("Modules[0] = %q, want %q", result.Modules[0], "child-a")
	}
	if result.Modules[1] != "child-b" {
		t.Errorf("Modules[1] = %q, want %q", result.Modules[1], "child-b")
	}
	if len(result.Deps) != 0 {
		t.Errorf("expected no deps in parent POM, got %d", len(result.Deps))
	}
}

func TestParsePOM_NoDependencies(t *testing.T) {
	content := `<?xml version="1.0"?>
<project>
  <groupId>io.example</groupId>
  <artifactId>minimal</artifactId>
  <version>0.1.0</version>
</project>`

	result, err := cfgfile.ParsePOM([]byte(content))
	if err != nil {
		t.Fatalf("ParsePOM: %v", err)
	}
	if len(result.Deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(result.Deps))
	}
	// Packaging defaults to empty string when not declared.
	if result.Packaging != "" {
		t.Errorf("Packaging = %q, want empty string when not declared", result.Packaging)
	}
}

func TestParsePOM_EmptyContent(t *testing.T) {
	_, err := cfgfile.ParsePOM([]byte(""))
	if err == nil {
		t.Error("expected error for empty content, got nil")
	}
}

func TestParseGradle_BasicDeps(t *testing.T) {
	content := `plugins {
    id 'java'
}

dependencies {
    implementation 'org.springframework:spring-core:5.3.21'
    testImplementation 'junit:junit:4.13'
    runtimeOnly 'com.h2database:h2:2.1.214'
}
`
	result, err := cfgfile.ParseGradle([]byte(content))
	if err != nil {
		t.Fatalf("ParseGradle: %v", err)
	}
	if len(result.Deps) != 3 {
		t.Fatalf("len(Deps) = %d, want 3", len(result.Deps))
	}

	dep0 := result.Deps[0]
	if dep0.Configuration != "implementation" {
		t.Errorf("Deps[0].Configuration = %q, want %q", dep0.Configuration, "implementation")
	}
	if dep0.Group != "org.springframework" {
		t.Errorf("Deps[0].Group = %q, want %q", dep0.Group, "org.springframework")
	}
	if dep0.Name != "spring-core" {
		t.Errorf("Deps[0].Name = %q, want %q", dep0.Name, "spring-core")
	}
	if dep0.Version != "5.3.21" {
		t.Errorf("Deps[0].Version = %q, want %q", dep0.Version, "5.3.21")
	}

	dep1 := result.Deps[1]
	if dep1.Configuration != "testImplementation" {
		t.Errorf("Deps[1].Configuration = %q, want %q", dep1.Configuration, "testImplementation")
	}
}

func TestParseGradle_MixedQuotes(t *testing.T) {
	content := `dependencies {
    implementation 'com.google.guava:guava:31.1-jre'
    api "com.fasterxml.jackson.core:jackson-databind:2.14.0"
    compileOnly 'org.projectlombok:lombok:1.18.26'
}
`
	result, err := cfgfile.ParseGradle([]byte(content))
	if err != nil {
		t.Fatalf("ParseGradle: %v", err)
	}
	if len(result.Deps) != 3 {
		t.Fatalf("len(Deps) = %d, want 3", len(result.Deps))
	}
	// Single-quoted entry.
	if result.Deps[0].Group != "com.google.guava" {
		t.Errorf("Deps[0].Group = %q, want %q", result.Deps[0].Group, "com.google.guava")
	}
	// Double-quoted entry.
	if result.Deps[1].Group != "com.fasterxml.jackson.core" {
		t.Errorf("Deps[1].Group = %q, want %q", result.Deps[1].Group, "com.fasterxml.jackson.core")
	}
	if result.Deps[2].Configuration != "compileOnly" {
		t.Errorf("Deps[2].Configuration = %q, want %q", result.Deps[2].Configuration, "compileOnly")
	}
}

func TestParseGradle_EmptyFile(t *testing.T) {
	result, err := cfgfile.ParseGradle([]byte(""))
	if err != nil {
		t.Fatalf("ParseGradle on empty file: %v", err)
	}
	if len(result.Deps) != 0 {
		t.Errorf("expected 0 deps from empty file, got %d", len(result.Deps))
	}
}

func TestParseDockerfile(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantImageCount  int
		wantExposeCount int
		wantErr         bool
	}{
		{
			name: "multi-stage",
			content: `FROM golang:1.21-alpine AS builder
WORKDIR /app
RUN go build .

FROM alpine:3.18
EXPOSE 8080
CMD ["/app"]
`,
			wantImageCount:  2,
			wantExposeCount: 1,
		},
		{
			name: "single stage",
			content: `FROM ubuntu:22.04
EXPOSE 3000
EXPOSE 8080
`,
			wantImageCount:  1,
			wantExposeCount: 2,
		},
		{
			name:           "empty dockerfile",
			content:        "",
			wantImageCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cfgfile.ParseDockerfile([]byte(tt.content))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDockerfile: %v", err)
			}
			if len(result.BaseImages) != tt.wantImageCount {
				t.Errorf("len(BaseImages) = %d, want %d", len(result.BaseImages), tt.wantImageCount)
			}
			if len(result.ExposedPorts) != tt.wantExposeCount {
				t.Errorf("len(ExposedPorts) = %d, want %d", len(result.ExposedPorts), tt.wantExposeCount)
			}
		})
	}
}
