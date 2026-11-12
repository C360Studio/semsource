package sourceauthority

import (
	"github.com/c360studio/semengine/types"
	"testing"
)

func TestResolveRequiresInjectedLocalAuthority(t *testing.T) {
	local := types.PlatformMeta{Org: "acme", Platform: "deployment-a1b2c3"}
	for _, tc := range []struct {
		name     string
		platform types.PlatformMeta
		org      string
		wantErr  bool
	}{
		{"inherits local", local, "", false},
		{"matching configured org", local, "acme", false},
		{"foreign org cannot override", local, "foreign", true},
		{"missing injected platform", types.PlatformMeta{Org: "acme"}, "acme", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.platform, tc.org)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Resolve authority=%+v error=%v", got, err)
			}
			if !tc.wantErr && (got.Org != local.Org || got.Platform != local.Platform) {
				t.Fatalf("authority changed: %+v", got)
			}
		})
	}
}
