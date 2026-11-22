package mcpgateway

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGraphSearchRequiresAQuery: argument validation happens before the NATS
// request, so the caller gets a usable message instead of a transport failure.
func TestGraphSearchRequiresAQuery(t *testing.T) {
	cs := connect(t, newTestComponent(nil))

	res := callTool(t, cs, "graph_search", map[string]any{"query": ""})
	if !res.IsError {
		t.Fatalf("empty query was accepted: %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "query is required") {
		t.Errorf("error does not name the missing argument: %s", text)
	}
}

// TestGraphToolDescriptionsStayHonest pins the description contract. These tools
// are reachable on stacks that provide neither clustering nor an LLM, so a
// description that promises community reasoning or query understanding
// unconditionally is a claim the running configuration may not be able to keep.
func TestGraphToolDescriptionsStayHonest(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	descriptions := make(map[string]string, len(res.Tools))
	for _, tool := range res.Tools {
		descriptions[tool.Name] = tool.Description
	}

	search, ok := descriptions["graph_search"]
	if !ok {
		t.Fatal("graph_search is not registered")
	}
	// It must advertise the disclosure, or an agent has no reason to read it,
	// and say it searches words: the substrate understands no meaning, so a
	// description implying query understanding promises what it cannot keep.
	for _, want := range []string{"retrieval", "word search", "never LLM-synthesized"} {
		if !strings.Contains(search, want) {
			t.Errorf("graph_search description missing %q: %s", want, search)
		}
	}
	// It must draw the line against code_search, which is the tool an agent
	// would otherwise reach for with the same query.
	if !strings.Contains(search, "code_search") {
		t.Errorf("graph_search description does not distinguish itself from code_search: %s", search)
	}
}

// TestTierOneDescriptionsStateTheMergedScale: at tier 1 a score is a merged
// rank score, at most 2/61, not a word-index score, so a client thresholding
// on the word scale breaks. The descriptions must say so, and say where seeds
// came from and when a merge was not honoured.
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: graph_search discloses the merged answer at Tier 1
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Fusion tools disclose where Tier 1 seeds came from
func TestTierOneDescriptionsStateTheMergedScale(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	descriptions := make(map[string]string, len(res.Tools))
	for _, tool := range res.Tools {
		descriptions[tool.Name] = tool.Description
	}
	want := map[string][]string{
		"graph_search": {"tier 1", "2/61", "not the word scale", "word_rank", "vector_rank", "merge.honoured false"},
		"code_search":  {"Tier 1", "`provenance`", "`similarity`", "2/61"},
		"code_context": {"Tier 1", "`provenance`", "`similarity`", "2/61"},
		"doc_context":  {"Tier 1", "`provenance`", "`similarity`", "2/61"},
	}
	for tool, fragments := range want {
		for _, fragment := range fragments {
			if !strings.Contains(descriptions[tool], fragment) {
				t.Errorf("%s description missing %q: %s", tool, fragment, descriptions[tool])
			}
		}
	}
}
