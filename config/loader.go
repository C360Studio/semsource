package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadConfig reads the JSON file at path, unmarshals it, applies defaults,
// and validates it. Returns a ready-to-use *Config or a descriptive error.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	return LoadConfigFromReader(f)
}

// LoadConfigFromReader parses JSON from r, applies defaults, and validates.
// Useful for testing without touching the filesystem.
func LoadConfigFromReader(r io.Reader) (*Config, error) {
	var cfg Config

	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&cfg); err != nil {
		if strings.Contains(err.Error(), `unknown field "git_token"`) {
			return nil, fmt.Errorf("config: git_token is not accepted in the configuration file; set %s instead: %w", EnvGitToken, err)
		}
		return nil, fmt.Errorf("config: parse JSON: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// WriteSources rewrites the file at path with sources and revision in place
// of its own, leaving every other key exactly as the file held it. The CLI
// edits files through this rather than marshalling a loaded Config, because
// loading applies defaults and environment values (including the git token)
// that must not be written back (#272). Callers load the file first, so a
// file that fails validation is never rewritten.
//
// shortcut: top-level keys are written in sorted order, so the first edit can
// reorder a hand-written file; upgrade to an order-preserving encoder if an
// operator relies on key order.
func WriteSources(path string, sources []SourceEntry, revision int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %q: %w", path, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("config: parse JSON: %w", err)
	}
	if doc["sources"], err = json.Marshal(sources); err != nil {
		return fmt.Errorf("config: marshal sources: %w", err)
	}
	if doc["config_revision"], err = json.Marshal(revision); err != nil {
		return fmt.Errorf("config: marshal config_revision: %w", err)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("config: write %q: %w", path, err)
	}
	return nil
}
