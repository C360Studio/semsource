package sourcespawn

import (
	"testing"

	"github.com/c360studio/semsource/config"
)

// TestAstComponentConfig_IndexVendoredJS pins that the semsource.json opt-in
// reaches the AST component's watch path, and that its absence leaves the
// watch path byte-identical to before.
func TestAstComponentConfig_IndexVendoredJS(t *testing.T) {
	for _, on := range []bool{false, true} {
		src := config.SourceEntry{Type: "ast", Path: "./src", Language: "javascript", IndexVendoredJS: on}
		_, compCfg, err := astComponentConfig(src, "acme")
		if err != nil {
			t.Fatalf("astComponentConfig: %v", err)
		}
		wp := compCfg["watch_paths"].([]map[string]any)[0]
		got, present := wp["index_vendored_js"]
		if on && got != true {
			t.Errorf("index_vendored_js=true not forwarded: %v", wp)
		}
		if !on && present {
			t.Errorf("index_vendored_js present when unset: %v", wp)
		}
	}
}
