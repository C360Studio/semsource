package golang

import (
	"fmt"
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

// pkgTypesEntry caches one package directory's declaration maps behind a content
// signature so the directory is re-parsed only when a source file is added,
// removed, or edited — not once per file in the package. One scan harvests both
// type declarations and package-level functions (go-callgraph-recall D1): funcs
// maps a `Recv == nil` FuncDecl's name to its defining relPath so calls resolve
// to the definition's own entity ID, cross-file and cross-package alike.
type pkgTypesEntry struct {
	sig   string
	types map[string]goLocalType
	funcs map[string]string
}

// packageTypes returns the top-level type declarations of the package (= the
// directory) containing fromRelPath, mapping name -> (defining relPath, kind).
// A bare Go type name refers to a type in the SAME package, which may live in a
// different file, so resolution needs a sibling scan (go/parser, declarations
// only).
func (p *Parser) packageTypes(fromRelPath string) map[string]goLocalType {
	return p.packageScan(filepath.Dir(fromRelPath)).types
}

// packageFuncs returns the package-level function declarations (name -> defining
// relPath) of the package at the repo-relative directory dirRel. Methods are
// excluded — a bare or package-qualified call name can only denote a `Recv ==
// nil` function.
func (p *Parser) packageFuncs(dirRel string) map[string]string {
	return p.packageScan(dirRel).funcs
}

// packageScan parses the declarations of every production Go file in one
// package directory, harvesting types and package-level funcs in a single pass.
// The scan is cached per directory on the Parser behind a name+size+mtime
// signature: a bulk index of an N-file package re-parses each directory once (not
// N times), and a watch edit to any sibling changes the signature and rebuilds, so
// the maps also stay fresh. `_test.go` files are excluded — production code cannot
// reference a test-only declaration, and an external `p_test` declaration must
// never shadow the production definition it doubles.
func (p *Parser) packageScan(dir string) pkgTypesEntry {
	absDir := filepath.Join(p.repoRoot, dir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return pkgTypesEntry{}
	}

	var goFiles []os.DirEntry
	var sig strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		goFiles = append(goFiles, e)
		if info, ierr := e.Info(); ierr == nil {
			fmt.Fprintf(&sig, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}

	if p.pkgTypesCache == nil {
		p.pkgTypesCache = make(map[string]pkgTypesEntry)
	}
	if cached, ok := p.pkgTypesCache[dir]; ok && cached.sig == sig.String() {
		return cached
	}

	types := make(map[string]goLocalType)
	funcs := make(map[string]string)
	fset := token.NewFileSet()
	for _, e := range goFiles {
		abs := filepath.Join(absDir, e.Name())
		f, perr := parser.ParseFile(fset, abs, nil, parser.SkipObjectResolution)
		if perr != nil {
			continue
		}
		rel, rerr := filepath.Rel(p.repoRoot, abs)
		if rerr != nil {
			rel = abs
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *goast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts, ok := spec.(*goast.TypeSpec)
					if !ok {
						continue
					}
					if _, exists := types[ts.Name.Name]; !exists {
						types[ts.Name.Name] = goLocalType{relPath: rel, kind: goTypeKind(ts.Type)}
					}
				}
			case *goast.FuncDecl:
				if d.Recv != nil || d.Name == nil {
					continue
				}
				if _, exists := funcs[d.Name.Name]; !exists {
					funcs[d.Name.Name] = rel
				}
			}
		}
	}
	entry := pkgTypesEntry{sig: sig.String(), types: types, funcs: funcs}
	p.pkgTypesCache[dir] = entry
	return entry
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
