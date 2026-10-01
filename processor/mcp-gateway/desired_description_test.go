package mcpgateway

import (
	"strings"
	"testing"
)

func TestSourceToolDescriptionsAreDesiredOnly(t *testing.T) {
	tools, err := connect(t, newTestComponent(nil)).ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tool := range tools.Tools {
		if tool.Name != "add_source" && tool.Name != "remove_source" {
			continue
		}
		count++
		if !strings.Contains(tool.Description, "restart") || !strings.Contains(tool.Description, "unavailable") {
			t.Errorf("%s implies activation/projection: %s", tool.Name, tool.Description)
		}
	}
	if count != 2 {
		t.Fatalf("source tools=%d", count)
	}
}
