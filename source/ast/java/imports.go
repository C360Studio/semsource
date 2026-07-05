package java

import (
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/source/ast"
)

// resolver state is refreshed per ParseFile (see Parser.pkg / importMap /
// localKinds). It lets a type reference resolve to the entity ID of its
// DEFINITION — same-file, same-package (sibling file), or an imported type in
// another package — so `extends` / `implements` / field-type edges connect
// instead of dangling (task #46, code-reference-resolution).

// extractImportMap builds simpleName -> fully-qualified name from a file's
// `import a.b.C;` declarations. Wildcard imports (`import a.b.*;`) are skipped —
// they cannot bind a specific simple name (a non-goal, left inert).
func extractImportMap(root *sitter.Node, content []byte) map[string]string {
	m := make(map[string]string)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() != "import_declaration" {
			continue
		}
		raw := string(content[child.StartByte():child.EndByte()])
		if strings.Contains(raw, "*") {
			continue // wildcard import — no specific simple name to bind
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			n := child.NamedChild(j)
			if n.Type() != "scoped_identifier" && n.Type() != "identifier" {
				continue
			}
			fqn := string(content[n.StartByte():n.EndByte()])
			if idx := strings.LastIndex(fqn, "."); idx >= 0 {
				m[fqn[idx+1:]] = fqn
			} else {
				m[fqn] = fqn
			}
		}
	}
	return m
}

// extractLocalKinds pre-scans a file's top-level type declarations into a
// name -> entity-kind table, so an unknown-kind reference (a field/return type)
// that names a type defined in the SAME file resolves to that type's real kind
// (D5). Built before edge extraction so it is order-independent within the file.
func extractLocalKinds(root *sitter.Node, content []byte) map[string]ast.CodeEntityType {
	kinds := make(map[string]ast.CodeEntityType)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		var kind ast.CodeEntityType
		switch child.Type() {
		case "class_declaration":
			kind = ast.TypeClass
		case "interface_declaration":
			kind = ast.TypeInterface
		case "enum_declaration":
			kind = ast.TypeEnum
		case "record_declaration":
			kind = ast.TypeStruct
		default:
			continue
		}
		if nameNode := child.ChildByFieldName("name"); nameNode != nil {
			kinds[string(content[nameNode.StartByte():nameNode.EndByte()])] = kind
		}
	}
	return kinds
}

// hierarchyRefID builds the entity ID for an `extends`/`implements` target whose
// kind is fixed by syntactic position (D2): a class-extends targets a class, an
// implements targets an interface, an interface-extends targets an interface. The
// ID is built through NewCodeEntity — the definition's own path — so the kind and
// SystemSlug segments match (D1). The defining file is resolved via imports or
// same-package layout (D3); an unresolved name falls back to the current file
// (the common in-file base type), staying inert if no such entity exists.
func (p *Parser) hierarchyRefID(name string, kind ast.CodeEntityType, fromRelPath string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if isBuiltinType(name) {
		return "builtin:" + name
	}
	if strings.Contains(name, ".") {
		return "external:" + name // already-qualified reference (stdlib/third-party)
	}
	if rel, ext, ok := p.resolveJavaType(name, fromRelPath); ok {
		return ast.NewCodeEntity(p.org, "java", p.project, kind, name, rel).ID
	} else if ext != "" {
		return "external:" + ext
	}
	// Unknown: assume the type is defined in the current file (a same-file base
	// type). Builds the definition ID when true; an inert (dropped) target if not.
	return ast.NewCodeEntity(p.org, "java", p.project, kind, name, fromRelPath).ID
}

// typeRefID builds the entity ID for an unknown-kind reference (a field, return,
// or parameter type). Its kind is not knowable from position, so it resolves only
// against a SAME-FILE definition via the local-kind table (D5); a cross-file
// unknown-kind reference stays inert (a non-goal) rather than guess a kind.
func (p *Parser) typeRefID(name, fromRelPath string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if isBuiltinType(name) {
		return "builtin:" + name
	}
	if strings.Contains(name, ".") {
		return "external:" + name
	}
	if kind, ok := p.localKinds[name]; ok {
		return ast.NewCodeEntity(p.org, "java", p.project, kind, name, fromRelPath).ID
	}
	// Unresolved cross-file / unknown-kind: keep the historical local construction
	// (now via NewCodeEntity, so SystemSlug is applied). Inert if no entity exists.
	return ast.NewCodeEntity(p.org, "java", p.project, ast.TypeType, name, fromRelPath).ID
}

// resolveJavaType resolves an unqualified type name to the repo-root-relative
// path of the file that defines it. It returns (relPath, "", true) for an in-tree
// definition; ("", fqn, false) for an imported but out-of-tree type (caller emits
// external:fqn); and ("", "", false) when the name is unknown (caller may assume
// same-file). Import binding takes precedence over same-package layout.
func (p *Parser) resolveJavaType(name, fromRelPath string) (relPath, external string, ok bool) {
	if fqn, imported := p.importMap[name]; imported {
		if rel, found := p.fqnToRelPath(fqn, fromRelPath); found {
			return rel, "", true
		}
		return "", fqn, false // imported from another (out-of-tree) package
	}
	// Same package: a top-level type lives in a sibling file named after it.
	dir := filepath.ToSlash(filepath.Dir(fromRelPath))
	cand := name + ".java"
	if dir != "." && dir != "" {
		cand = dir + "/" + cand
	}
	if fileExists(filepath.Join(p.repoRoot, filepath.FromSlash(cand))) {
		return filepath.FromSlash(cand), "", true
	}
	return "", "", false
}

// fqnToRelPath maps a fully-qualified type name to its file, probing the source
// tree. The source-root prefix is derived by stripping the referrer's package
// path from its directory (so `src/main/java/a/B.java` in package `a` yields the
// prefix `src/main/java/`); the FQN's dotted path is joined under that prefix,
// with a repoRoot-relative probe as a fallback.
func (p *Parser) fqnToRelPath(fqn, fromRelPath string) (string, bool) {
	fqnPath := filepath.FromSlash(strings.ReplaceAll(fqn, ".", "/")) + ".java"
	for _, cand := range []string{
		filepath.Join(p.sourceRootPrefix(fromRelPath), fqnPath),
		fqnPath,
	} {
		if fileExists(filepath.Join(p.repoRoot, cand)) {
			return cand, true
		}
	}
	return "", false
}

// sourceRootPrefix returns the referrer's directory with its package path suffix
// removed — the on-disk root that fully-qualified names are resolved against.
// Returns "" (repoRoot) when the layout does not mirror the package.
func (p *Parser) sourceRootPrefix(fromRelPath string) string {
	dir := filepath.Dir(fromRelPath)
	if p.pkg == "" {
		return dir
	}
	pkgPath := filepath.FromSlash(strings.ReplaceAll(p.pkg, ".", "/"))
	if dir == pkgPath {
		return ""
	}
	if trimmed := strings.TrimSuffix(dir, string(filepath.Separator)+pkgPath); trimmed != dir {
		return trimmed
	}
	return ""
}

// fileExists reports whether path names an existing regular file.
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
