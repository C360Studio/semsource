package python

import "github.com/c360studio/semsource/entityid"

func testAuthority(org string) entityid.Authority {
	return entityid.Authority{Org: org, Platform: "test-platform"}
}
