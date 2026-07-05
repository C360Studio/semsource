package python

import (
	"path"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/source/ast"
)

// importBinding records where a locally-bound name came from, so a later type
// reference to that name can be resolved to the entity ID of its definition in
// the DEFINING module's file (task #44 / code-reference-resolution).
//
//	from pkg.base import BaseClient        -> binding{module:"pkg.base", origin:"BaseClient", level:0}
//	from pkg.base import BaseClient as B   -> B      -> {module:"pkg.base", origin:"BaseClient"}
//	from .base import BaseClient           -> {module:"base", origin:"BaseClient", level:1}
//	import pkg.base                        -> "pkg.base" -> {module:"pkg.base"}   (dotted usage)
//	import pkg.base as pb                  -> "pb"       -> {module:"pkg.base"}
type importBinding struct {
	module string // dotted source module ("" for a bare relative `from . import x`)
	origin string // original name in that module ("" for a plain `import module`)
	level  int    // relative-import level: 0 = absolute, 1 = ".", 2 = "..", …
}

// extractImportBindings walks a file's top-level import statements into a
// localName -> importBinding map. Star imports (`from m import *`) are skipped —
// they cannot be resolved without the target's definitions (a non-goal).
func extractImportBindings(root *sitter.Node, content []byte) map[string]importBinding {
	bindings := make(map[string]importBinding)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "import_statement":
			addImportStatement(child, content, bindings)
		case "import_from_statement":
			addImportFromStatement(child, content, bindings)
		}
	}
	return bindings
}

// addImportStatement handles `import a.b.c` and `import a.b.c as x`. The binding is
// keyed by the name used at the reference site: the full dotted path for a plain
// import (`a.b.c.Foo`), or the alias (`x.Foo`).
func addImportStatement(node *sitter.Node, content []byte, bindings map[string]importBinding) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "dotted_name":
			module := nodeText(child, content)
			bindings[module] = importBinding{module: module}
		case "aliased_import":
			name := child.ChildByFieldName("name")
			alias := child.ChildByFieldName("alias")
			if name != nil && alias != nil {
				bindings[nodeText(alias, content)] = importBinding{module: nodeText(name, content)}
			}
		}
	}
}

// addImportFromStatement handles `from module import N [as A]`, including relative
// imports (`from .pkg import N`). The imported names bind to the module.
func addImportFromStatement(node *sitter.Node, content []byte, bindings map[string]importBinding) {
	moduleNode := node.ChildByFieldName("module_name")
	module, level := "", 0
	if moduleNode != nil {
		module, level = parseModuleName(moduleNode, content)
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		// Skip the module_name child and any wildcard (`*`) import.
		if moduleNode != nil && child.Equal(moduleNode) {
			continue
		}
		switch child.Type() {
		case "dotted_name":
			name := nodeText(child, content)
			bindings[name] = importBinding{module: module, origin: name, level: level}
		case "aliased_import":
			nameNode := child.ChildByFieldName("name")
			aliasNode := child.ChildByFieldName("alias")
			if nameNode != nil && aliasNode != nil {
				bindings[nodeText(aliasNode, content)] = importBinding{
					module: module, origin: nodeText(nameNode, content), level: level,
				}
			}
		}
	}
}

// parseModuleName returns the dotted module and the relative-import level. For an
// absolute `dotted_name` the level is 0; for a `relative_import` the level is the
// number of leading dots and the module is the dotted part after them (may be "").
func parseModuleName(node *sitter.Node, content []byte) (module string, level int) {
	if node.Type() != "relative_import" {
		return nodeText(node, content), 0
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "import_prefix":
			level = strings.Count(nodeText(child, content), ".")
		case "dotted_name":
			module = nodeText(child, content)
		}
	}
	return module, level
}

// moduleToRelPath resolves a dotted module to a repo-root-relative file path,
// probing `<parts>.py` then `<parts>/__init__.py` under repoRoot. Relative imports
// (level > 0) resolve against fromRelPath's package, walking up level-1 parents.
// Returns ok=false for out-of-tree modules (stdlib / third-party) — the caller
// then leaves the reference inert rather than resolving it to a wrong entity.
func (p *Parser) moduleToRelPath(module, fromRelPath string, level int) (string, bool) {
	var baseParts []string
	if level > 0 {
		// Package dir of the referrer, then up (level-1) parents.
		pkg := path.Dir(filepath.ToSlash(fromRelPath))
		for i := 1; i < level && pkg != "." && pkg != "/" && pkg != ""; i++ {
			pkg = path.Dir(pkg)
		}
		if pkg != "." && pkg != "" {
			baseParts = strings.Split(pkg, "/")
		}
	}
	if module != "" {
		baseParts = append(baseParts, strings.Split(module, ".")...)
	}
	if len(baseParts) == 0 {
		return "", false
	}

	joined := filepath.Join(baseParts...)
	for _, cand := range []string{joined + ".py", filepath.Join(joined, "__init__.py")} {
		if ast.FileExists(filepath.Join(p.repoRoot, cand)) {
			return cand, true
		}
	}
	return "", false
}

// lookupBinding maps a referenced name to (module, origin, level). A bare name
// resolves via a from-import binding; a dotted name (module.Name or alias.Name)
// resolves via an import/alias binding on its head, taking the last segment as the
// referenced name.
func lookupBinding(typeName string, imports map[string]importBinding) (module, origin string, level int, ok bool) {
	if idx := strings.LastIndex(typeName, "."); idx > 0 {
		head, name := typeName[:idx], typeName[idx+1:]
		if b, found := imports[head]; found {
			mod := b.module
			if mod == "" {
				mod = head
			}
			return mod, name, b.level, true
		}
		return "", "", 0, false
	}
	if b, found := imports[typeName]; found {
		return b.module, b.origin, b.level, true
	}
	return "", "", 0, false
}

// nodeText returns the source text spanned by a node.
func nodeText(n *sitter.Node, content []byte) string {
	return string(content[n.StartByte():n.EndByte()])
}
