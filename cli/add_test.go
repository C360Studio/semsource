package cli

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// seedSource is an unrelated existing entry, because a config with no sources
// at all does not load.
func seedSource() []config.SourceEntry {
	return []config.SourceEntry{{Type: "docs", Paths: []string{"docs/"}, Watch: true}}
}

// stubObjectStoreVerify replaces the reachability probe for the duration of a
// test. Restored on cleanup so a failing case cannot leak into the next one.
func stubObjectStoreVerify(t *testing.T, err error) {
	t.Helper()

	original := verifySource
	verifySource = func(entry *config.SourceEntry) error {
		if entry.Type != "s3" {
			return nil
		}
		return err
	}
	t.Cleanup(func() { verifySource = original })
}

func TestAddS3_ExplicitIdentity(t *testing.T) {
	stubObjectStoreVerify(t, nil)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, seedSource())

	term, _ := newTestTerm("")
	args := []string{
		"s3",
		"--bucket", "artifacts",
		"--prefix", "reports/",
		"--endpoint", "https://garage.internal:3900",
		"--region", "garage",
		"--path-style",
		"--project", "quarterly-reports",
		"--version", "2026-q3",
	}
	if err := Add(term, cfgPath, args); err != nil {
		t.Fatalf("Add: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	if len(cfg.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(cfg.Sources))
	}

	got := cfg.Sources[1]
	if got.Type != "s3" || got.Bucket != "artifacts" || got.Prefix != "reports/" {
		t.Errorf("entry = %+v", got)
	}
	if got.Endpoint != "https://garage.internal:3900" || got.Region != "garage" || !got.PathStyle {
		t.Errorf("connection settings lost: %+v", got)
	}
	if got.Project != "quarterly-reports" || got.Version != "2026-q3" {
		t.Errorf("identity lost: project=%q version=%q", got.Project, got.Version)
	}
}

// TestAddS3_OmittedIdentityFallsBackToTheBucket keeps the common case free of
// ceremony: a bucket is enough, and identity derives from it.
func TestAddS3_OmittedIdentityFallsBackToTheBucket(t *testing.T) {
	stubObjectStoreVerify(t, nil)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, seedSource())

	term, _ := newTestTerm("")
	if err := Add(term, cfgPath, []string{"s3", "--bucket", "artifacts"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got := loadConfig(t, cfgPath).Sources[1]
	if got.Project != "" || got.Version != "" {
		t.Errorf("identity was invented: project=%q version=%q", got.Project, got.Version)
	}
	if got.Endpoint != "" {
		t.Errorf("endpoint was invented: %q — empty means the store's default", got.Endpoint)
	}
	if !got.Watch {
		t.Error("watching should default on")
	}
}

// TestAddS3_TwoPrefixesAreDistinctProjects is the shape an adopter reaches for
// when one bucket holds more than one corpus.
func TestAddS3_TwoPrefixesAreDistinctProjects(t *testing.T) {
	stubObjectStoreVerify(t, nil)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, seedSource())

	term, _ := newTestTerm("")
	for _, args := range [][]string{
		{"s3", "--bucket", "artifacts", "--prefix", "reports/", "--project", "quarterly-reports"},
		{"s3", "--bucket", "artifacts", "--prefix", "memos/", "--project", "internal-memos"},
	} {
		if err := Add(term, cfgPath, args); err != nil {
			t.Fatalf("Add %v: %v", args, err)
		}
	}

	sources := loadConfig(t, cfgPath).Sources
	if len(sources) != 3 {
		t.Fatalf("sources = %d, want 3", len(sources))
	}
	if sources[1].Project == sources[2].Project {
		t.Errorf("both prefixes registered as project %q", sources[1].Project)
	}
	if sources[1].Bucket != sources[2].Bucket {
		t.Error("the two entries should share a bucket")
	}
}

func TestAddS3_RequiresABucket(t *testing.T) {
	stubObjectStoreVerify(t, nil)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, seedSource())

	term, _ := newTestTerm("")
	err := Add(term, cfgPath, []string{"s3", "--prefix", "reports/"})
	if err == nil {
		t.Fatal("expected an error with no bucket")
	}
	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("error should name the flag, got: %v", err)
	}
	if len(loadConfig(t, cfgPath).Sources) != 1 {
		t.Error("a rejected entry was written to the config file")
	}
}

// TestAddS3_UnreachableBucketLeavesTheConfigUnchanged is the whole point of
// probing before writing. An operator who mistypes an endpoint should find out
// now, not at the next `semsource run`, and should not have to undo a write to
// try again.
func TestAddS3_UnreachableBucketLeavesTheConfigUnchanged(t *testing.T) {
	probeErr := errors.New(`cannot reach bucket "artifacts" at https://garage.internal:3900: connection refused`)
	stubObjectStoreVerify(t, probeErr)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "docs", Paths: []string{"docs/"}, Watch: true},
	})
	before := readFile(t, cfgPath)

	term, _ := newTestTerm("")
	err := Add(term, cfgPath, []string{
		"s3", "--bucket", "artifacts", "--endpoint", "https://garage.internal:3900",
	})
	if err == nil {
		t.Fatal("expected an error for an unreachable bucket")
	}

	// Endpoint, bucket, and cause — without all three the operator cannot tell
	// a typo in the bucket from a credential that is not exported.
	for _, want := range []string{"artifacts", "garage.internal:3900", "connection refused"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the config file changed despite the failure:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestVerifyObjectStore_NamesEverythingNeeded drives the real probe against a
// port nothing is listening on, so the message an operator actually sees is
// the one under test rather than a stub's.
func TestVerifyObjectStore_NamesEverythingNeeded(t *testing.T) {
	t.Setenv("SEMSOURCE_S3_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("SEMSOURCE_S3_SECRET_ACCESS_KEY", "wJalrXUtnFEMI-K7MDENG-bPxRfiCYEXAMPLEKEY")

	// A listener opened and immediately closed hands back a port that is
	// definitely free, so the probe fails on connection rather than on a
	// stranger's service.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	endpoint := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	probeErr := verifyObjectStore(&config.SourceEntry{
		Type:      "s3",
		Bucket:    "artifacts",
		Endpoint:  endpoint,
		Region:    "us-east-1",
		PathStyle: true,
	})
	if probeErr == nil {
		t.Fatal("expected an error probing a closed port")
	}

	message := probeErr.Error()
	for _, want := range []string{"artifacts", endpoint, "SEMSOURCE_S3_ACCESS_KEY_ID"} {
		if !strings.Contains(message, want) {
			t.Errorf("error should mention %q, got: %v", want, probeErr)
		}
	}
	// The secret is in the environment; it must not be in the message.
	if strings.Contains(message, "wJalrXUtnFEMI-K7MDENG-bPxRfiCYEXAMPLEKEY") {
		t.Errorf("the probe error leaked a credential: %v", probeErr)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
