package golang

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/source/ast"
)

// goLocalType records where a same-package type is defined and its real kind, so
// a bare type reference resolves to the entity ID of its DEFINITION — with the
// correct kind segment (struct/interface/type) and, when it lives in a sibling
// file, that file's path (task #46, code-reference-resolution).
type goLocalType struct {
	relPath string
	kind    ast.CodeEntityType
}

// packageTypes returns the top-level type declarations of the package (= the
// directory) containing fromRelPath, mapping name -> (defining relPath, kind).
// A bare Go type name refers to a type in the SAME package, which may live in a
// different file, so resolution needs a sibling scan. Built lazily once per
// ParseFile (p.pkgTypes is reset at parse start) so it stays fresh across watch
// re-parses; scans the directory with go/parser (declarations only, no bodies).
func (p *Parser) packageTypes(fromRelPath string) map[string]goLocalType {
	if p.pkgTypes != nil {
		return p.pkgTypes
	}
	types := make(map[string]goLocalType)
	p.pkgTypes = types

	absDir := filepath.Join(p.repoRoot, filepath.Dir(fromRelPath))
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return types
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		abs := filepath.Join(absDir, e.Name())
		f, err := parser.ParseFile(fset, abs, nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(p.repoRoot, abs)
		if err != nil {
			rel = abs
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*goast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*goast.TypeSpec)
				if !ok {
					continue
				}
				if _, exists := types[ts.Name.Name]; !exists {
					types[ts.Name.Name] = goLocalType{relPath: rel, kind: goTypeKind(ts.Type)}
				}
			}
		}
	}
	return types
}

// goTypeKind classifies a type spec's underlying expression, matching how
// extractTypeSpec classifies a definition (struct / interface / everything-else).
func goTypeKind(expr goast.Expr) ast.CodeEntityType {
	switch expr.(type) {
	case *goast.StructType:
		return ast.TypeStruct
	case *goast.InterfaceType:
		return ast.TypeInterface
	default:
		return ast.TypeType
	}
}
