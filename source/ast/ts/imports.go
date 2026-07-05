package ts

import (
	"path"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/source/ast"
)

// tsExtensions is the probe order for resolving a relative module specifier to a
// file: a same-named source file first, then a directory index. Mirrors the
// extensions ParseFile accepts.
var tsExtensions = []string{".ts", ".tsx", ".d.ts", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}

// tsBinding records where a locally-bound imported name came from, so a type
// reference to that name resolves to the entity ID of its DEFINITION in the
// imported module's file (task #46, code-reference-resolution).
//
//	import { Base } from './base'        -> Base -> {spec:"./base", origin:"Base"}
//	import { Base as B } from './base'   -> B    -> {spec:"./base", origin:"Base"}
//	import Def from './def'              -> Def  -> {spec:"./def",  origin:"Def"}
type tsBinding struct {
	spec   string // module specifier ('./base', 'react', …)
	origin string // name as defined in that module (the aliased-from name)
}

// extractImportBindings walks a file's import statements into a
// localName -> tsBinding map. Namespace imports (`import * as NS`) are skipped —
// a `NS.Type` reference is dotted and handled as external.
func extractImportBindings(root *sitter.Node, source []byte) map[string]tsBinding {
	bindings := make(map[string]tsBinding)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "import_statement" {
			addImportStatement(n, source, bindings)
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return bindings
}

func addImportStatement(node *sitter.Node, source []byte, bindings map[string]tsBinding) {
	spec := ""
	if s := node.ChildByFieldName("source"); s != nil {
		spec = strings.Trim(nodeText(s, source), `'"`)
	}
	if spec == "" {
		return
	}
	clause := findChild(node, "import_clause")
	if clause == nil {
		return
	}
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(i)
		switch child.Type() {
		case "identifier": // default import: `import Def from '...'`
			// A default import's origin is taken as the local name: the module's
			// real default-export symbol name is not knowable without parsing the
			// target. The common `import Base from './base'` (matching the exported
			// name) resolves; a *renamed* default import binds a name the definition
			// does not use, so it stays inert (dropped) — never a wrong edge.
			bindings[nodeText(child, source)] = tsBinding{spec: spec, origin: nodeText(child, source)}
		case "named_imports":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				spec2 := child.NamedChild(j)
				if spec2.Type() != "import_specifier" {
					continue
				}
				addNamedImport(spec2, source, spec, bindings)
			}
		}
	}
}

// addNamedImport binds one `import_specifier` (a `name` and optional `alias`) to
// its module and original name.
func addNamedImport(spec2 *sitter.Node, source []byte, spec string, bindings map[string]tsBinding) {
	name := spec2.ChildByFieldName("name")
	alias := spec2.ChildByFieldName("alias")
	if name == nil {
		// Grammar exposes name/alias as positional identifiers on some versions.
		var ids []*sitter.Node
		for k := 0; k < int(spec2.NamedChildCount()); k++ {
			if spec2.NamedChild(k).Type() == "identifier" {
				ids = append(ids, spec2.NamedChild(k))
			}
		}
		if len(ids) == 0 {
			return
		}
		origin := nodeText(ids[0], source)
		local := origin
		if len(ids) > 1 {
			local = nodeText(ids[1], source)
		}
		bindings[local] = tsBinding{spec: spec, origin: origin}
		return
	}
	origin := nodeText(name, source)
	local := origin
	if alias != nil {
		local = nodeText(alias, source)
	}
	bindings[local] = tsBinding{spec: spec, origin: origin}
}

// extractLocalKinds pre-scans a file's top-level type declarations into a
// name -> entity-kind table (D5), so a same-file unknown-kind reference (a
// parameter/return type) resolves to that type's real kind.
func extractLocalKinds(root *sitter.Node, source []byte) map[string]ast.CodeEntityType {
	kinds := make(map[string]ast.CodeEntityType)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		var kind ast.CodeEntityType
		switch n.Type() {
		case "class_declaration":
			kind = ast.TypeClass
		case "interface_declaration":
			kind = ast.TypeInterface
		case "type_alias_declaration":
			kind = ast.TypeType
		case "enum_declaration":
			kind = ast.TypeEnum
		}
		if kind != "" {
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				kinds[nodeText(nameNode, source)] = kind
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return kinds
}

// hierarchyRefID builds the entity ID for an `extends`/`implements` target whose
// kind is fixed by syntactic position (D2), routed through NewCodeEntity so the
// kind and SystemSlug segments match the definition (D1). The defining file is
// resolved via a relative import (D3); a bare/`node_modules` import stays
// external; an unimported name falls back to the current file (same-file base
// type), staying inert if no such entity exists.
func (p *Parser) hierarchyRefID(name string, kind ast.CodeEntityType, fromRelPath string) string {
	name = cleanTSTypeName(name)
	if name == "" {
		return ""
	}
	if isTSBuiltinType(name) {
		return "builtin:" + name
	}
	if strings.Contains(name, ".") {
		return "external:" + name // e.g. React.Component / NS.Type
	}
	if rel, origin, ok := p.resolveTSImport(name, fromRelPath); ok {
		return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, kind, origin, rel).ID
	}
	if _, imported := p.importBindings[name]; imported {
		return "external:" + name // bare specifier or unresolved relative import
	}
	return ast.NewCodeEntity(p.org, p.detectLanguage(fromRelPath), p.project, kind, name, fromRelPath).ID
}

// typeRefID builds the entity ID for an unknown-kind reference (a parameter or
// return type). Its kind is not knowable from position, so it resolves only
// against a SAME-FILE definition via the local-kind table (D5); anything else
// keeps the historical local construction (inert if no entity exists).
func (p *Parser) typeRefID(name, fromRelPath string) string {
	name = cleanTSTypeName(name)
	if name == "" {
		return ""
	}
	if isTSBuiltinType(name) {
		return "builtin:" + name
	}
	if strings.Contains(name, ".") {
		return "external:" + name
	}
	lang := p.detectLanguage(fromRelPath)
	if kind, ok := p.localKinds[name]; ok {
		return ast.NewCodeEntity(p.org, lang, p.project, kind, name, fromRelPath).ID
	}
	return ast.NewCodeEntity(p.org, lang, p.project, ast.TypeType, name, fromRelPath).ID
}

// resolveTSImport resolves an imported name to (definingRelPath, originName) when
// it is bound to an in-tree relative module. Returns ok=false for unimported
// names and for bare/`node_modules` specifiers (which stay external/inert).
func (p *Parser) resolveTSImport(name, fromRelPath string) (relPath, origin string, ok bool) {
	b, imported := p.importBindings[name]
	if !imported || !strings.HasPrefix(b.spec, ".") {
		return "", "", false
	}
	dir := path.Dir(filepath.ToSlash(fromRelPath))
	joined := path.Join(dir, b.spec)
	for _, ext := range tsExtensions {
		if cand := joined + ext; ast.FileExists(filepath.Join(p.repoRoot, filepath.FromSlash(cand))) {
			return filepath.FromSlash(cand), b.origin, true
		}
	}
	for _, ext := range tsExtensions {
		if cand := path.Join(joined, "index"+ext); ast.FileExists(filepath.Join(p.repoRoot, filepath.FromSlash(cand))) {
			return filepath.FromSlash(cand), b.origin, true
		}
	}
	return "", "", false
}

// cleanTSTypeName trims whitespace, a leading `:` (return-type annotations), and
// generic type parameters, leaving the bare type name.
func cleanTSTypeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, ":")
	name = strings.TrimSpace(name)
	if idx := strings.IndexAny(name, "<[|&"); idx > 0 {
		name = strings.TrimSpace(name[:idx])
	}
	return name
}

// findChild returns the first direct child of the given type, or nil.
func findChild(node *sitter.Node, typ string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if node.NamedChild(i).Type() == typ {
			return node.NamedChild(i)
		}
	}
	return nil
}
