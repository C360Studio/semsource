package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShippedConfigsLoad pins compose-packaging-hardening D1: every config file
// shipped in configs/ (including configs/tiers/) must load through the same
// path `semsource run --config` uses — a shipped example that fails validation
// is a documented crash loop. (The audit found the tier configs pointed at a
// mount the compose stack never creates; paths are now compose-true, and this
// gate keeps future examples loadable.)
func TestShippedConfigsLoad(t *testing.T) {
	root := "../configs"
	var found int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		found++
		if _, lerr := LoadConfig(path); lerr != nil {
			t.Errorf("shipped config %s does not load: %v", path, lerr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if found == 0 {
		t.Fatal("no shipped configs found — wrong path?")
	}
}

// TestDocsReferenceResolvableConfigs pins D1's doc half: every
// `SEMSOURCE_CONFIG=<value>` instruction in the README and tier docs must name
// a file that actually exists under configs/ (the compose command resolves
// /etc/semsource/<value> against the configs/ mount).
func TestDocsReferenceResolvableConfigs(t *testing.T) {
	for _, doc := range []string{"../README.md", "../configs/tiers/README.md"} {
		data, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("read %s: %v", doc, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			idx := strings.Index(line, "SEMSOURCE_CONFIG=")
			if idx < 0 {
				continue
			}
			val := line[idx+len("SEMSOURCE_CONFIG="):]
			// The value ends at whitespace or a backtick.
			if end := strings.IndexAny(val, " \t`"); end >= 0 {
				val = val[:end]
			}
			if val == "" || strings.Contains(val, "<") { // placeholder like tiers/<file>.json
				continue
			}
			if _, serr := os.Stat(filepath.Join("../configs", val)); serr != nil {
				t.Errorf("%s instructs SEMSOURCE_CONFIG=%s but configs/%s does not exist", doc, val, val)
			}
		}
	}
}
