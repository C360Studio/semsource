package ast

import (
	"reflect"
	"testing"
)

func TestFoldDeclarations(t *testing.T) {
	file := &CodeEntity{ID: "file", Contains: []string{"f", "g", "f"}}
	proto := &CodeEntity{ID: "f", StartLine: 1, Contains: []string{"a"}}
	g := &CodeEntity{ID: "g", StartLine: 2}
	def := &CodeEntity{ID: "f", StartLine: 3, Contains: []string{"b", "a"}}
	isDef := map[*CodeEntity]bool{def: true}

	got := FoldDeclarations([]*CodeEntity{file, proto, g, def}, func(c, k *CodeEntity) bool {
		return isDef[c] && !isDef[k]
	})

	if want := []*CodeEntity{file, def, g}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fold kept %v, want the definition in the prototype's slot", got)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(def.Contains, want) {
		t.Errorf("survivor contains %v, want %v (kept's children first, deduplicated)", def.Contains, want)
	}
	if want := []string{"f", "g"}; !reflect.DeepEqual(file.Contains, want) {
		t.Errorf("file contains %v, want %v", file.Contains, want)
	}
}

func TestFoldDeclarationsKeepsFirstWhenPolicyDeclines(t *testing.T) {
	first := &CodeEntity{ID: "m", StartLine: 1}
	second := &CodeEntity{ID: "m", StartLine: 5, Contains: []string{"c"}}
	got := FoldDeclarations([]*CodeEntity{first, second}, func(_, _ *CodeEntity) bool { return false })
	if len(got) != 1 || got[0] != first {
		t.Fatalf("fold kept %v, want only the first", got)
	}
	if want := []string{"c"}; !reflect.DeepEqual(first.Contains, want) {
		t.Errorf("survivor contains %v, want the dropped twin's children %v", first.Contains, want)
	}
}
