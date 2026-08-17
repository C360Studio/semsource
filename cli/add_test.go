package cli

import (
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestAddASTWithExplicitIdentity pins #189: the CLI can declare the
// project/version pair — registering the same project at two versions is
// the shape code_changes diffs, previously reachable only by hand-editing
// the config file.
func TestAddASTWithExplicitIdentity(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "docs", Paths: []string{"docs/"}, Watch: true},
	})

	term, _ := newTestTerm("")
	for _, args := range [][]string{
		{"ast", "--path", "./depA-1.9.0", "--language", "go", "--project", "depA", "--version", "1.9.0"},
		{"ast", "--path", "./depA-1.10.0", "--language", "go", "--project", "depA", "--version", "1.10.0"},
	} {
		if err := Add(term, cfgPath, args); err != nil {
			t.Fatalf("Add %v: %v", args, err)
		}
	}

	cfg := loadConfig(t, cfgPath)
	if len(cfg.Sources) != 3 {
		t.Fatalf("sources = %d, want 3", len(cfg.Sources))
	}
	for i, want := range []struct{ path, version string }{
		{"./depA-1.9.0", "1.9.0"},
		{"./depA-1.10.0", "1.10.0"},
	} {
		s := cfg.Sources[i+1]
		if s.Project != "depA" || s.Version != want.version || s.Path != want.path {
			t.Errorf("entry %d = %+v, want project=depA version=%s path=%s", i+1, s, want.version, want.path)
		}
	}
}

// TestAddRepoWithExplicitIdentity: the remote form carries the pair too —
// expansion applies it to the expanded code entries like a config-file
// declaration would.
func TestAddRepoWithExplicitIdentity(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "docs", Paths: []string{"docs/"}, Watch: true},
	})

	term, _ := newTestTerm("")
	err := Add(term, cfgPath, []string{
		"repo", "--url", "https://github.com/org/dep",
		"--project", "org-dep", "--version", "b1256521ee39",
	})
	if err != nil {
		t.Fatalf("Add repo: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	if len(cfg.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(cfg.Sources))
	}
	s := cfg.Sources[1]
	if s.Project != "org-dep" || s.Version != "b1256521ee39" {
		t.Errorf("repo entry = %+v, want explicit project/version", s)
	}
}

// TestAddWithoutIdentityFlagsUnchanged: omission writes no identity keys —
// existing invocations stay byte-for-byte compatible and derivation rules
// keep applying.
func TestAddWithoutIdentityFlagsUnchanged(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "docs", Paths: []string{"docs/"}, Watch: true},
	})

	term, _ := newTestTerm("")
	if err := Add(term, cfgPath, []string{"ast", "--path", "./src", "--language", "go"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	s := cfg.Sources[1]
	if s.Project != "" || s.Version != "" {
		t.Errorf("identity fields set without flags: %+v", s)
	}
}
