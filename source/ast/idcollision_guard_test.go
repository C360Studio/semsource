package ast_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
	_ "github.com/c360studio/semsource/source/ast/c"
	_ "github.com/c360studio/semsource/source/ast/cpp"
	_ "github.com/c360studio/semsource/source/ast/golang"
	_ "github.com/c360studio/semsource/source/ast/java"
	_ "github.com/c360studio/semsource/source/ast/python"
	_ "github.com/c360studio/semsource/source/ast/svelte"
	_ "github.com/c360studio/semsource/source/ast/ts"
)

// guardLanguages are the parsers whose declarations must never share an entity
// ID (entity-identity-safety, "One entity ID per declaration within a parse
// pass). C precedes C++ so ".h" is read as C, the routing that folds C++
// overloads in a C-routed header (#338).
var guardLanguages = []string{"go", "java", "typescript", "javascript", "svelte", "python", "c", "cpp"}

// duplicateIDs parses every file under root with the guarded parsers and
// returns, per entity ID declared more than once within one file, the
// "path:line" of each declaration.
func duplicateIDs(t *testing.T, root string, skipDir func(name string) bool) map[string][]string {
	t.Helper()
	parsers := map[string]ast.FileParser{}
	byExt := map[string]string{}
	for _, lang := range guardLanguages {
		p, err := ast.DefaultRegistry.CreateParser(lang, entityid.Authority{Org: "guard", Platform: "test"}, "guard", root)
		if err != nil {
			t.Fatalf("create %s parser: %v", lang, err)
		}
		parsers[lang] = p
		for _, ext := range ast.DefaultRegistry.GetExtensionsForParser(lang) {
			if _, taken := byExt[ext]; !taken {
				byExt[ext] = lang
			}
		}
	}

	dups := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		lang, ok := byExt[filepath.Ext(path)]
		if !ok {
			return nil
		}
		res, err := parsers[lang].ParseFile(context.Background(), path)
		if err != nil || res == nil {
			return nil // parse failures are other tests' concern
		}
		rel, _ := filepath.Rel(root, path)
		seen := map[string][]string{}
		for _, e := range res.Entities {
			seen[e.ID] = append(seen[e.ID], rel+":"+strconv.Itoa(e.StartLine))
		}
		for id, at := range seen {
			if len(at) > 1 {
				dups[id] = at
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return dups
}

func reportDuplicates(t *testing.T, dups map[string][]string) {
	t.Helper()
	ids := make([]string, 0, len(dups))
	for id := range dups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t.Errorf("entity ID %s is declared %d times: %s", id, len(dups[id]), strings.Join(dups[id], ", "))
	}
}

// TestParseResultIDsAreUnique parses one fixture per known collision class
// (#329: Java overloads, TS block-scoped locals and accessors, Go blanks and
// init; #338: Python accessors, overload stubs and redefinitions, C/C++
// prototypes, #if twins and redefined macros) and fails on any entity ID declared twice in one file.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestParseResultIDsAreUnique(t *testing.T) {
	for _, class := range []string{"java", "ts", "golang", "python", "c", "cpp"} {
		t.Run(class, func(t *testing.T) {
			root := filepath.Join("testdata", "idcollisions", class)
			reportDuplicates(t, duplicateIDs(t, root, func(string) bool { return false }))
		})
	}
}

// TestRepositoryTreeIDsAreUnique runs the same check over this repository's
// own source, the dogfood corpus.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestRepositoryTreeIDsAreUnique(t *testing.T) {
	if testing.Short() {
		t.Skip("parses the whole repository")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found at %s: %v", root, err)
	}
	skip := func(name string) bool {
		switch name {
		case "testdata", "node_modules", "vendor", "dist", "build":
			return true
		}
		return strings.HasPrefix(name, ".") // .git, .svelte-kit, .worktrees
	}
	reportDuplicates(t, duplicateIDs(t, root, skip))
}
