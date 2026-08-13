package mcpgateway

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
)

// toolsListBudgetBytes caps the serialized tools/list payload (#126). Every
// registered tool ships its name, description, and full derived JSON Schema to
// the client on session init and rides in the model's context for the entire
// session — this is standing per-session overhead, paid whether or not a tool
// is used. Measured 2026-08-12: 9 tools, ~7.1 KB. The ceiling leaves ~1 KB of
// headroom so a growing description trips review before it trips an agent's
// context budget; raising it must be a deliberate act with the per-tool
// breakdown from this test's log in hand, never a silent drift.
const toolsListBudgetBytes = 8192

// TestToolsListSchemaBudget measures the serialized size of every registered
// tool (as an MCP client receives it) and fails when the total exceeds the
// budget. The per-tool breakdown is logged largest-first so the offender is
// named, not hunted.
func TestToolsListSchemaBudget(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("no tools registered")
	}

	type row struct {
		name  string
		total int
		desc  int
	}
	rows := make([]row, 0, len(res.Tools))
	total := 0
	for _, tool := range res.Tools {
		b, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("marshal tool %s: %v", tool.Name, err)
		}
		rows = append(rows, row{name: tool.Name, total: len(b), desc: len(tool.Description)})
		total += len(b)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].total > rows[j].total })

	for _, r := range rows {
		t.Logf("%-16s %5d B (description %4d B, schema+envelope %4d B)", r.name, r.total, r.desc, r.total-r.desc)
	}
	t.Logf("tools/list total: %d bytes across %d tools (budget %d)", total, len(rows), toolsListBudgetBytes)

	if total > toolsListBudgetBytes {
		t.Errorf("tools/list = %d bytes, over the %d-byte budget: trim the largest tools above or raise the budget deliberately (#126)",
			total, toolsListBudgetBytes)
	}
}
