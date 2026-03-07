package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
		return nil, fmt.Errorf("config: parse JSON: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
