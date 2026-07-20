package svelte

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func parseSvelte(t *testing.T, src string) []*ast.CodeEntity {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Component.svelte")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewParser("test-org", "test-project", dir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	var got []*ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeConst || e.Type == ast.TypeVar {
			got = append(got, e)
		}
	}
	return got
}

func names(entities []*ast.CodeEntity) []string {
	out := make([]string, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.Name)
	}
	return out
}

// TestSvelteDestructuring_Props is the case that makes this defect matter more
// in Svelte than in plain TS: `let { a, b } = $props()` is the canonical Svelte
// 5 runes idiom for declaring component props, so naming the entity after the
// whole pattern meant essentially every component's props were unsearchable,
// collapsed into one meaningless entity.
func TestSvelteDestructuring_Props(t *testing.T) {
	got := parseSvelte(t, `<script>
	let { data, onUpdate, disabled = false } = $props();
</script>

<div>{data}</div>
`)
	want := map[string]bool{"data": true, "onUpdate": true, "disabled": true}
	if len(got) != len(want) {
		t.Fatalf("got %d entities %v, want %d %v", len(got), names(got), len(want), want)
	}
	for _, e := range got {
		if !want[e.Name] {
			t.Errorf("unexpected entity %q", e.Name)
		}
	}
}

// TestSvelteDestructuring_ShapesAndPlainDeclarations covers the rename and
// array forms alongside a plain declaration, so the fix cannot regress the
// single-identifier path it shares.
func TestSvelteDestructuring_ShapesAndPlainDeclarations(t *testing.T) {
	got := parseSvelte(t, `<script>
	const { renamed: localName } = config;
	const [first, second] = pair;
	const plain = 1;
</script>
`)
	want := map[string]bool{"localName": true, "first": true, "second": true, "plain": true}
	if len(got) != len(want) {
		t.Fatalf("got %d entities %v, want %d %v", len(got), names(got), len(want), want)
	}
	for _, e := range got {
		if !want[e.Name] {
			t.Errorf("unexpected entity %q (property keys must not become entities)", e.Name)
		}
	}
}
