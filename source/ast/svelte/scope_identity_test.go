package svelte

import (
	"sort"
	"testing"
)

// TestSvelteScriptLocalsAreNotEntities applies #331's local rule to a
// component script: only the script's top-level declarations are entities.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestSvelteScriptLocalsAreNotEntities(t *testing.T) {
	src := `<script lang="ts">
  const count = 1;
  function handle() {
    const inner = 2;
    return inner;
  }
  const onClick = () => { let local = 3; return local; };
</script>
<button on:click={onClick}>{count}</button>
`
	got := names(parseSvelte(t, src))
	sort.Strings(got)
	want := []string{"count"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("const/var entities = %v, want %v", got, want)
	}
}
