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
