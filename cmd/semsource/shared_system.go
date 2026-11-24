package main

import (
	"fmt"

	semconfig "github.com/c360studio/semengine/config"

	"github.com/c360studio/semsource/internal/sourcespawn"
)

// checkBootCollisions refuses the component set this boot is about to spawn
// when two ast-source or two doc-source roots would mint IDs under one org and
// system (#349). components is the configuration in effect after discovery
// reconciled: file sources, the desired-manifest sources earlier add_source
// requests persisted, and the discovered branches and submodule pins. The pins
// are the names the discovery record files under kind "submodule": the
// component configs do not carry the InstanceSuffix that marks a pin, and the
// record is what discovery itself owns. Boot refuses rather than starting the
// rest (design D4).
func checkBootCollisions(components semconfig.ComponentConfigs) error {
	record, err := previousRecord(components)
	if err != nil {
		return err
	}
	pins := map[string]bool{}
	for _, r := range record {
		if r.Kind != "submodule" {
			continue
		}
		for _, name := range r.Components {
			pins[name] = true
		}
	}
	if err := sourcespawn.CheckSystemCollisions(sourcespawn.Planned(components, pins)); err != nil {
		return fmt.Errorf("refusing to boot: %w; remove one of the named sources from the config file, "+
			"or from the desired manifest with `semsource remove --manifest <instance>`", err)
	}
	return nil
}
