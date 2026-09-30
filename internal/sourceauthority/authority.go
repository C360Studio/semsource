// Package sourceauthority validates the authority injected into source components.
package sourceauthority

import (
	"fmt"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semstreams/component"
)

// Resolve refuses per-source organization overrides outside the boot deployment.
func Resolve(platform component.PlatformMeta, org string) (entityid.Authority, error) {
	authority := entityid.Authority{Org: platform.Org, Platform: platform.Platform}
	if err := authority.Validate(); err != nil {
		return entityid.Authority{}, fmt.Errorf("source deployment authority: %w", err)
	}
	if org != "" && org != authority.Org {
		return entityid.Authority{}, fmt.Errorf("source org %q differs from deployment org %q; foreign subjects require an import lane", org, authority.Org)
	}
	return authority, nil
}
