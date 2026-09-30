//go:build integration

package governance

import "github.com/c360studio/semsource/entityid"

func fixtureAuthority() entityid.Authority {
	return entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}
}
