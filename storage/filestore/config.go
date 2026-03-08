package filestore

import (
	"errors"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the filestore storage component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// RootDir is the root directory under which all keys are stored as file paths.
	// Required when the component is used; may be empty when the store is created
	// directly via New rather than through the component system.
	RootDir string `json:"root_dir" schema:"type:string,description:Root directory for stored files,category:basic,required:true"`

	// CreateIfMissing controls whether the root directory is created when it
	// does not exist. When false an error is returned if RootDir is missing.
	CreateIfMissing bool `json:"create_if_missing" schema:"type:bool,description:Create root directory if it does not exist,category:basic,default:true"`
}

// Validate checks that the configuration is complete and consistent.
func (c *Config) Validate() error {
	if c.RootDir == "" {
		return errors.New("root_dir is required")
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
// No port configuration is needed — filestore is a pure storage backend.
func DefaultConfig() Config {
	return Config{
		CreateIfMissing: true,
	}
}
