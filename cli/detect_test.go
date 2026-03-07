package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrgFromRemoteHTTPS(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"https://github.com/acme/repo.git", "acme"},
		{"https://github.com/acme/repo", "acme"},
		{"https://gitlab.com/my-org/project.git", "my-org"},
	}
	for _, tt := range tests {
		got := orgFromRemote(tt.remote)
		if got != tt.want {
			t.Errorf("orgFromRemote(%q) = %q, want %q", tt.remote, got, tt.want)
		}
	}
}

func TestOrgFromRemoteSSH(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"git@github.com:acme/repo.git", "acme"},
		{"git@gitlab.com:my-org/project.git", "my-org"},
	}
	for _, tt := range tests {
		got := orgFromRemote(tt.remote)
		if got != tt.want {
			t.Errorf("orgFromRemote(%q) = %q, want %q", tt.remote, got, tt.want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Acme Corp", "acme-corp"},
		{"my_project", "my-project"},
		{"  spaces  ", "spaces"},
		{"already-clean", "already-clean"},
	}
	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDetectProjectFindsGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)

	info := DetectProject(dir)

	if info.Language != "go" {
		t.Errorf("expected language 'go', got %q", info.Language)
	}
	if !info.HasDocs {
		t.Error("expected HasDocs to be true")
	}
	if len(info.ConfigFiles) == 0 {
		t.Error("expected at least one config file")
	}
	found := false
	for _, f := range info.ConfigFiles {
		if f == "go.mod" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected go.mod in ConfigFiles, got %v", info.ConfigFiles)
	}
}

func TestDetectProjectFindsDocsDir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "docs"), 0755)

	info := DetectProject(dir)

	if !info.HasDocs {
		t.Error("expected HasDocs true when docs/ exists")
	}
	found := false
	for _, p := range info.DocPaths {
		if p == "docs/" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'docs/' in DocPaths, got %v", info.DocPaths)
	}
}

func TestDetectProjectNamespaceFallback(t *testing.T) {
	dir := t.TempDir()
	info := DetectProject(dir)

	// Should fall back to dir name.
	if info.Namespace == "" {
		t.Error("expected non-empty namespace from dir name fallback")
	}
}
