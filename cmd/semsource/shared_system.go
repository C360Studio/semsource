package main

import (
	"context"
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
func checkBootCollisions(ctx context.Context, components semconfig.ComponentConfigs) error {
	pins, err := sourcespawn.PinInstances(components)
	if err != nil {
		return err
	}
	return bootRefusal(sourcespawn.CheckSystemCollisions(ctx, sourcespawn.Planned(components, pins)))
}

func bootRefusal(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("refusing to boot: %w; remove one of the named sources from the config file, "+
		"or from the desired manifest with `semsource remove --manifest <instance>`", err)
}

// checkDiscoveryCollisions refuses discovery's writes before reconcile makes
// any (#349). The discovered set is what reconcile would put, without the
// names a runtime removal disabled; the retained set is what stays in effect,
// without what reconcile retires and without a discovered name the previous
// record owns, which discovery is rewriting as its own writer. A discovered
// component that would replace a retained one under its name from another
// root is thereby compared with it rather than collapsed into one survivor.
// Pins are the previous record's and this plan's submodule names.
func checkDiscoveryCollisions(ctx context.Context, effective semconfig.ComponentConfigs, plan *discoveryPlan, file semconfig.ComponentConfigs, owned, protected map[string]bool) error {
	pins, err := sourcespawn.PinInstances(effective)
	if err != nil {
		return err
	}
	for _, o := range plan.owners {
		if o.kind == "submodule" {
			for name := range o.names {
				pins[name] = true
			}
		}
	}
	retained, discovered := semconfig.ComponentConfigs{}, semconfig.ComponentConfigs{}
	for name, have := range effective {
		_, rewritten := plan.desired[name]
		if plan.retires(name, have, file, owned, protected) || (rewritten && owned[name]) {
			continue
		}
		retained[name] = have
	}
	for name, want := range plan.desired {
		if have, ok := effective[name]; ok && !have.Enabled {
			continue // reconcile leaves a runtime removal in place
		}
		discovered[name] = want
	}
	return bootRefusal(sourcespawn.CheckDiscoveredCollisions(ctx,
		sourcespawn.Planned(retained, pins), sourcespawn.Planned(discovered, pins)))
}
