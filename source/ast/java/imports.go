package java

import (
	"path/filepath"
	"sort"
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
	return p.resolveJavaTypeIn(name, fromRelPath, p.importMap)
}

// resolveJavaTypeIn is resolveJavaType against an explicit import map, so a name
// written in another file (an ancestor's `extends` clause) is bound by THAT
// file's imports rather than the imports of the file currently being parsed.
func (p *Parser) resolveJavaTypeIn(name, fromRelPath string, importMap map[string]string) (relPath, external string, ok bool) {
	if fqn, imported := importMap[name]; imported {
		if rel, found := p.fqnToRelPath(fqn, fromRelPath); found {
			return rel, "", true
		}
		return "", fqn, false // imported from another (out-of-tree) package
	}
	// Same package: assume the standard one-public-type-per-eponymous-file layout,
	// so `Base` lives in a sibling `Base.java`. A package-private type sharing
	// another file (e.g. `Base` declared inside `Types.java`) is not found here and
	// the caller falls back to an inert same-file target — a missing edge, never a
	// wrong one. Resolving that would need a sibling-file scan (as Go does); out of
	// scope for the common Java case.
	dir := filepath.ToSlash(filepath.Dir(fromRelPath))
	cand := name + ".java"
	if dir != "." && dir != "" {
		cand = dir + "/" + cand
	}
	if ast.FileExists(filepath.Join(p.repoRoot, filepath.FromSlash(cand))) {
		return filepath.FromSlash(cand), "", true
	}
	return "", "", false
}

// fqnToRelPath maps a fully-qualified type name to its file, probing the source
// tree. The source-root prefix is derived by stripping the referrer's package
// path from its directory (so `src/main/java/a/B.java` in package `a` yields the
// prefix `src/main/java/`); the FQN's dotted path is joined under that prefix,
// with a repoRoot-relative probe as a fallback.
//
// A multi-module repo (Gradle/Maven) has one source root PER MODULE, so a type
// imported from a sibling module lives under a root the referrer's own prefix
// never reaches. Measured on OSH Core, that left 4,682 call targets marked
// `external:` despite naming an in-repo package. Sibling roots are therefore
// probed too — after the referrer's own root, which still wins outright, and
// only when the FQN is found under exactly ONE of them.
func (p *Parser) fqnToRelPath(fqn, fromRelPath string) (string, bool) {
	// One file resolves the same FQN once per call site, and each miss costs a
	// stat per source root. Memoized for this ParseFile only, so the filesystem
	// is still re-read on the next parse.
	//
	// The key includes the REFERRER, because the first probe is relative to the
	// referrer's own source root: an ancestor walk resolves names as written in
	// another file, and in a repo where two modules declare the same FQN each
	// referrer must legitimately get its own module's copy.
	key := fqn + "\x00" + fromRelPath
	if hit, ok := p.fqnMemo[key]; ok {
		return hit.relPath, hit.ok
	}
	relPath, found, ambiguous := p.probeFQN(fqn, fromRelPath)
	if p.fqnMemo == nil {
		p.fqnMemo = make(map[string]fqnResult)
	}
	p.fqnMemo[key] = fqnResult{relPath: relPath, ok: found, ambiguous: ambiguous}
	if ambiguous {
		p.ambiguousFQNs[fqn] = true
	}
	return relPath, found
}

// fqnResult is one memoized FQN lookup. `ambiguous` distinguishes "this name is
// in the tree more than once" from "this name is not in the tree at all" — the
// two are both unresolved, but only the latter is genuinely external.
type fqnResult struct {
	relPath   string
	ok        bool
	ambiguous bool
}

// fqnIsAmbiguous reports whether a previously-probed FQN matched several source
// roots. Tracked by FQN alone — independent of which file asked — because the
// question "is this name in the tree more than once?" is a property of the tree.
// Valid only after fqnToRelPath has been called for that FQN.
func (p *Parser) fqnIsAmbiguous(fqn string) bool {
	return p.ambiguousFQNs[fqn]
}

// probeFQN does the actual filesystem probing behind fqnToRelPath.
func (p *Parser) probeFQN(fqn, fromRelPath string) (relPath string, found, ambiguous bool) {
	fqnPath := filepath.FromSlash(strings.ReplaceAll(fqn, ".", "/")) + ".java"
	for _, cand := range []string{
		filepath.Join(p.sourceRootPrefix(fromRelPath), fqnPath),
		fqnPath,
	} {
		if ast.FileExists(filepath.Join(p.repoRoot, cand)) {
			return cand, true, false
		}
	}
	var hit string
	matches := 0
	for _, root := range p.siblingSourceRoots(fromRelPath) {
		cand := filepath.Join(root, fqnPath)
		if ast.FileExists(filepath.Join(p.repoRoot, cand)) {
			matches++
			hit = cand
		}
	}
	if matches == 1 {
		return hit, true, false
	}
	// Zero matches, or the same FQN path under several roots — the latter is
	// ambiguous, so it stays unresolved rather than resolving to an arbitrary
	// module, and callers must not mistake it for a third-party type.
	return "", false, matches > 1
}

// siblingSourceRoots lists the repo's other source roots that share the
// referrer's layout convention (e.g. every `*/src/main/java`). It is discovered
// by bounded globbing rather than a tree walk, and memoized for the duration of
// one ParseFile so the result cannot go stale across a watch-mode re-parse.
// A repo with no recognised layout yields none, leaving behavior unchanged.
func (p *Parser) siblingSourceRoots(fromRelPath string) []string {
	if p.sourceRootsDone {
		return p.sourceRoots
	}
	p.sourceRootsDone = true

	layout := layoutSuffix(p.sourceRootPrefix(fromRelPath))
	if layout == "" {
		return nil
	}
	var roots []string
	// Modules sit one or two levels below the repo root in practice
	// (`sensorhub-core/...`, `lib-ogc/swe-common-core/...`).
	for _, depth := range []string{"*", "*/*"} {
		pattern := filepath.Join(p.repoRoot, filepath.FromSlash(depth+"/"+layout))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if rel, relErr := filepath.Rel(p.repoRoot, m); relErr == nil {
				roots = append(roots, rel)
			}
		}
	}
	// Sorted so resolution never depends on filesystem enumeration order.
	sort.Strings(roots)
	p.sourceRoots = roots
	return roots
}

// layoutSuffix returns a glob for the build-tool layout a source root ends with,
// or "" for a flat layout. The middle segment stays a wildcard so a referrer
// under `src/test/java` still reaches `src/main/java` roots — the common case of
// a test calling the code it exercises.
func layoutSuffix(sourceRoot string) string {
	parts := strings.Split(filepath.ToSlash(sourceRoot), "/")
	if len(parts) < 3 {
		return ""
	}
	tail := parts[len(parts)-3:]
	if tail[0] == "src" && tail[2] == "java" {
		return "src/*/java"
	}
	return ""
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
