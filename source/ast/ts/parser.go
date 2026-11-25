// Package ts provides TypeScript and JavaScript AST parsing and code entity extraction using tree-sitter.
package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/gitboundary"
	"github.com/c360studio/semsource/source/ast"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func init() {
	ast.DefaultRegistry.Register("typescript",
		[]string{".ts", ".tsx", ".mts", ".cts"},
		func(authority entityid.Authority, project, repoRoot string) ast.FileParser {
			return NewParser(authority, project, repoRoot)
		})
	ast.DefaultRegistry.Register("javascript",
		[]string{".js", ".jsx", ".mjs", ".cjs"},
		func(authority entityid.Authority, project, repoRoot string) ast.FileParser {
			return NewParser(authority, project, repoRoot)
		})
}

// Parser extracts code entities from TypeScript/JavaScript source files using tree-sitter
type Parser struct {
	authority entityid.Authority
	project   string
	repoRoot  string

	// Per-file resolver state, refreshed each ParseFile (see imports.go). Serialized
	// per source path by ast-source's parseFileWithWatcher lock (task #44 design D6).
	importBindings map[string]tsBinding          // local name -> imported module + origin
	localKinds     map[string]ast.CodeEntityType // same-file type name -> kind (D5)

	// Call-resolution state (see calls.go). localFuncs/namespaceImports/
	// defaultImports are refreshed each ParseFile alongside importBindings above.
	// moduleInfoMemo caches a resolved import target's parsed function set,
	// reallocated each ParseFile (mirrors python/calls.go's moduleFuncsMemo) so
	// an edited target module is re-read on the next parse rather than served
	// stale.
	localFuncs       map[string]bool   // same-file top-level function names
	namespaceImports map[string]string // local namespace name -> module specifier
	defaultImports   map[string]string // local default-import name -> module specifier
	moduleInfoMemo   map[string]moduleCallInfo

	// Walk state: the namespace chain enclosing the node being walked, and the
	// namespace bodies being walked. Pushed and popped by EnterNamespace; empty
	// at module level, so module-level IDs are built with no scope and do not
	// change. nsExported is refreshed per file with every namespace chain's
	// exported members, merged across all of that chain's declarations.
	nsScope    []string
	nsFrames   []nsFrame
	nsExported map[string]nsMembers
	nsDecls    map[string]int // namespace chain -> declarations with a body
}

// nsMembers is the functions and types a namespace body declares.
type nsMembers struct {
	funcs map[string]bool
	kinds map[string]ast.CodeEntityType
}

func newNSMembers() nsMembers {
	return nsMembers{funcs: make(map[string]bool), kinds: make(map[string]ast.CodeEntityType)}
}

// add records the function or type a direct child of a namespace body declares.
func (m nsMembers) add(node *sitter.Node, source []byte) {
	collectTopLevelFuncNode(node, source, m.funcs)
	collectTopLevelKind(node, source, m.kinds)
}

// nsFrame is one namespace body on the walk stack: its full scope chain and
// every member it declares, exported or not, since inside its own body a
// non-exported member is visible too.
//
// Merged-namespace privates (#329): when a chain is declared more than once
// in the file and the body is not ambient, each body's non-exported members
// are locals of that body. TypeScript scopes them per declaration, and two
// same-named ones share path, chain and name, so no intrinsic discriminator
// tells them apart (a source-order index would break rule 1). They are not
// emitted (IsNamespaceLocal), and a reference to one from inside its body is
// inert: it neither targets the dropped ID nor falls through to an outer or
// module-level namesake, which it shadows. Exported and ambient members keep
// their scoped IDs; a chain declared once keeps its private members.
type nsFrame struct {
	scope   []string
	own     nsMembers
	body    *sitter.Node
	ambient bool
	locals  *nsMembers // the body's non-exported members, when they are locals
}

// chainKey keys a namespace chain. The separator is not a legal identifier
// character, so `declare module "a.b"` (one segment) and `namespace a.b` (two)
// stay distinct chains.
func chainKey(chain []string) string { return strings.Join(chain, "\x00") }

// collectNamespaceExports pre-scans a file for every namespace or module
// declaration outside a local scope and merges, per full scope chain, the
// members other declarations of that chain can see: exported ones, or all of
// them in an ambient (`declare`) context, where members are implicitly
// exported. TypeScript merges same-chain declarations this way.
func collectNamespaceExports(root *sitter.Node, source []byte) (map[string]nsMembers, map[string]int) {
	out := make(map[string]nsMembers)
	decls := make(map[string]int)
	var walk func(n *sitter.Node, chain []string, ambient bool)
	walk = func(n *sitter.Node, chain []string, ambient bool) {
		if IsLocalScope(n) {
			return
		}
		if n.Type() == "ambient_declaration" {
			ambient = true
		}
		if segments := NamespaceSegments(n, source); segments != nil {
			chain = append(append([]string(nil), chain...), segments...)
			if body := n.ChildByFieldName("body"); body != nil {
				key := chainKey(chain)
				decls[key]++
				m, ok := out[key]
				if !ok {
					m = newNSMembers()
					out[key] = m
				}
				for i := 0; i < int(body.NamedChildCount()); i++ {
					if c := body.NamedChild(i); ambient || c.Type() == "export_statement" {
						m.add(c, source)
					}
				}
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i), chain, ambient)
		}
	}
	walk(root, nil, false)
	return out, decls
}

// NewParser creates a new TypeScript/JavaScript parser
func NewParser(authority entityid.Authority, project, repoRoot string) *Parser {
	return &Parser{
		authority: authority,
		project:   project,
		repoRoot:  repoRoot,
	}
}

// ParseFile parses a single TypeScript/JavaScript file and extracts code entities
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*ast.ParseResult, error) {
	// Check context before starting
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Check context after file read
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	hash := ast.ComputeHash(content)

	relPath, err := filepath.Rel(p.repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	lang := p.detectLanguage(filePath)

	// Get appropriate tree-sitter language
	tsLang := p.getTreeSitterLanguage(filePath)
	parser := sitter.NewParser()
	parser.SetLanguage(tsLang)

	// Parse the source code
	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	defer tree.Close()

	// Check context after parsing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	parseResult := &ast.ParseResult{
		Path:     relPath,
		Hash:     hash,
		Imports:  make([]string, 0),
		Entities: make([]*ast.CodeEntity, 0),
	}

	// Create file entity
	fileEntity := ast.NewCodeEntity(p.authority, lang, p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Hash = hash
	fileEntity.StartLine = 1
	fileEntity.EndLine = countLines(content)

	parseResult.FileEntity = fileEntity
	parseResult.Entities = append(parseResult.Entities, fileEntity)

	// Extract entities from the AST
	rootNode := tree.RootNode()

	// Refresh per-file resolver state used to point type references at the entity
	// ID of their DEFINITION (same-file or imported module) — see imports.go.
	p.importBindings = extractImportBindings(rootNode, content)
	p.localKinds = extractLocalKinds(rootNode, content)

	// Refresh call-resolution state (calls.go).
	p.localFuncs = collectModuleFuncNames(rootNode, content)
	p.namespaceImports = extractNamespaceImports(rootNode, content)
	p.defaultImports = extractDefaultImports(rootNode, content)
	p.moduleInfoMemo = make(map[string]moduleCallInfo)
	p.nsScope, p.nsFrames = nil, nil
	p.nsExported, p.nsDecls = collectNamespaceExports(rootNode, content)

	entities := p.extractEntities(rootNode, content, relPath, lang, fileEntity.ID)
	for _, entity := range entities {
		parseResult.Entities = append(parseResult.Entities, entity)
		fileEntity.Contains = append(fileEntity.Contains, entity.ID)
	}

	// Extract imports
	imports := p.extractImports(rootNode, content)
	fileEntity.Imports = imports
	parseResult.Imports = imports

	return parseResult, nil
}

// ParseDirectory parses all TypeScript/JavaScript files in a directory
func (p *Parser) ParseDirectory(ctx context.Context, dirPath string) ([]*ast.ParseResult, error) {
	var results []*ast.ParseResult

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() {
			base := filepath.Base(path)
			// Skip common non-source directories
			if base == "node_modules" || base == "dist" || base == ".next" ||
				base == "build" || base == "coverage" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if path != dirPath && gitboundary.IsBoundary(path) {
				return filepath.SkipDir
			}
			return nil
		}

		if !p.isTargetFile(path) {
			return nil
		}

		result, err := p.ParseFile(ctx, path)
		if err != nil {
			// Log error but continue with other files
			return nil
		}

		results = append(results, result)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return results, nil
}

// isTargetFile returns true if the file is a TypeScript/JavaScript file
func (p *Parser) isTargetFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs":
		return true
	}
	return false
}

// detectLanguage returns the language identifier for the file
func (p *Parser) detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	}
	return "javascript"
}

// getTreeSitterLanguage returns the tree-sitter language for the file type
func (p *Parser) getTreeSitterLanguage(filePath string) *sitter.Language {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".tsx":
		return tsx.GetLanguage()
	case ".ts", ".mts", ".cts":
		return typescript.GetLanguage()
	case ".jsx", ".js", ".mjs", ".cjs":
		return javascript.GetLanguage()
	default:
		return javascript.GetLanguage()
	}
}

// extractEntities extracts all code entities from the AST
func (p *Parser) extractEntities(node *sitter.Node, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	// Walk the tree and extract entities
	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	entities = append(entities, p.walkNode(cursor, source, filePath, lang, parentID)...)

	return entities
}

// walkNode recursively walks the AST and extracts entities
func (p *Parser) walkNode(cursor *sitter.TreeCursor, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	node := cursor.CurrentNode()
	nodeType := node.Type()
	if p.IsNamespaceLocal(node) {
		return nil
	}

	// Extract entity based on node type
	switch nodeType {
	case "class_declaration":
		entity := p.extractClass(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
			// Extract methods from the class body. Scope passes the class name
			// so two methods named "submit" in different classes within the
			// same file get distinct entity IDs.
			methods := p.extractClassMethods(node, source, filePath, lang, entity.ID, p.scopeWith(entity.Name))
			for _, method := range methods {
				entity.Contains = append(entity.Contains, method.ID)
				entities = append(entities, method)
			}
		}

	case "interface_declaration":
		if lang == "typescript" {
			entity := p.extractInterface(node, source, filePath, lang, parentID)
			if entity != nil {
				entities = append(entities, entity)
			}
		}

	case "type_alias_declaration":
		if lang == "typescript" {
			entity := p.extractTypeAlias(node, source, filePath, lang, parentID)
			if entity != nil {
				entities = append(entities, entity)
			}
		}

	case "enum_declaration":
		if lang == "typescript" {
			entity := p.extractEnum(node, source, filePath, lang, parentID)
			if entity != nil {
				entities = append(entities, entity)
			}
		}

	case "function_declaration":
		entity := p.extractFunction(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "lexical_declaration":
		// This handles const/let declarations
		varEntities := p.extractVariableDeclaration(node, source, filePath, lang, parentID)
		entities = append(entities, varEntities...)

	case "variable_declaration":
		// This handles var declarations
		varEntities := p.extractVariableDeclaration(node, source, filePath, lang, parentID)
		entities = append(entities, varEntities...)
	}

	// Everything below a local scope is not emitted (#331, #329): a
	// declaration's identity must not depend on which callback or block it
	// sits in, and nothing addresses a local from outside its scope. The node
	// itself was handled by the switch above.
	if IsLocalScope(node) {
		return entities
	}

	// A namespace's members are scoped by its namespace chain (#329).
	defer p.EnterNamespace(node, source)()

	// Recursively process children
	if cursor.GoToFirstChild() {
		for {
			entities = append(entities, p.walkNode(cursor, source, filePath, lang, parentID)...)
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}

	return entities
}

// localScopeTypes are the node types that open a scope whose declarations are
// local: functions of every form, methods, class static blocks (#331), and the
// module-level statements that open a block scope (#329) — if/else, every
// loop form, try/catch/finally, switch, with and labeled statements. Listing
// the statements, not only their statement_block bodies, also covers the
// unbraced forms (`if (a) var x = 1;`, a `case` clause's bare declarations)
// and loop-head and catch bindings.
//
// var is local here too, though JS hoists it to the module: the #329 ruling
// scopes identity by block, and a block's var is conditional, so it is no
// more addressable as a module symbol than the block's const.
var localScopeTypes = map[string]bool{
	"function_declaration":           true,
	"generator_function_declaration": true,
	"function_expression":            true,
	"function":                       true,
	"generator_function":             true,
	"arrow_function":                 true,
	"method_definition":              true,
	"class_static_block":             true,
	"if_statement":                   true,
	"for_statement":                  true,
	"for_in_statement":               true,
	"while_statement":                true,
	"do_statement":                   true,
	"try_statement":                  true,
	"switch_statement":               true,
	"with_statement":                 true,
	"labeled_statement":              true,
}

// IsLocalScope reports whether declarations beneath node are local to it, and
// so not emitted. A bare statement_block is local; one that is the body of a
// namespace, module or `declare global` is not, since its members are
// addressable from outside it. Shared with the Svelte script walk.
func IsLocalScope(node *sitter.Node) bool {
	if node.Type() != "statement_block" {
		return localScopeTypes[node.Type()]
	}
	parent := node.Parent()
	if parent == nil {
		return true
	}
	switch parent.Type() {
	case "internal_module", "module", "ambient_declaration":
		return false
	}
	return true
}

// NamespaceSegments returns the scope segments a namespace or module
// declaration adds for its members — `namespace A.B {}` adds A and B, and
// `declare module "m" {}` adds m — or nil when node is not one. A `declare
// global {}` block adds none: its members are globals. Shared with the Svelte
// script walk.
func NamespaceSegments(node *sitter.Node, source []byte) []string {
	if t := node.Type(); t != "internal_module" && t != "module" {
		return nil
	}
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nodeText(nameNode, source)
	if nameNode.Type() == "string" {
		// A module specifier is one name, dots and all. The ":" is never
		// ID-valid, so the segment always carries a hash of the raw
		// specifier: "foo.bar", "foo/bar" and namespace foo.bar stay apart
		// even though IDs map "." and "/" to "-" (#340).
		if name = strings.Trim(name, "\"'`"); name == "" {
			return nil
		}
		return []string{entityid.SanitizeSegment("module:" + name)}
	}
	if name == "" {
		return nil
	}
	return strings.Split(name, ".")
}

// scopeWith returns the current namespace chain extended by name, as a fresh
// slice, for a member scope such as a class inside a namespace.
func (p *Parser) scopeWith(names ...string) []string {
	return append(append([]string(nil), p.nsScope...), names...)
}

// EnterNamespace pushes node onto the walk's namespace stack when it is a
// namespace or module declaration, and returns the func that pops it (a no-op
// otherwise). While pushed, declarations are scoped by the namespace chain and
// bare calls and type names resolve against the namespace's own members
// first. Shared with the Svelte script walk, whose calls this Parser resolves.
func (p *Parser) EnterNamespace(node *sitter.Node, source []byte) func() {
	segments := NamespaceSegments(node, source)
	if segments == nil {
		return func() {}
	}
	prevScope, prevFrames := p.nsScope, p.nsFrames
	p.nsScope = p.scopeWith(segments...)
	frame := nsFrame{scope: p.nsScope, own: newNSMembers(), body: node.ChildByFieldName("body")}
	if parent := node.Parent(); parent != nil && parent.Type() == "ambient_declaration" {
		frame.ambient = true
	}
	if n := len(p.nsFrames); n > 0 && p.nsFrames[n-1].ambient {
		frame.ambient = true
	}
	if !frame.ambient && p.nsDecls[chainKey(p.nsScope)] > 1 {
		locals := newNSMembers()
		frame.locals = &locals
	}
	if frame.body != nil {
		for i := 0; i < int(frame.body.NamedChildCount()); i++ {
			child := frame.body.NamedChild(i)
			frame.own.add(child, source)
			if frame.locals != nil && child.Type() != "export_statement" {
				frame.locals.add(child, source)
			}
		}
	}
	p.nsFrames = append(append([]nsFrame(nil), p.nsFrames...), frame)
	return func() { p.nsScope, p.nsFrames = prevScope, prevFrames }
}

// NamespaceScope returns the namespace chain enclosing the node being walked,
// empty at module level.
func (p *Parser) NamespaceScope() []string { return p.nsScope }

// IsNamespaceLocal reports whether node is a non-exported member of the
// namespace body being walked whose chain is merged, and so a local that is
// not emitted (see nsFrame). Shared with the Svelte script walk.
func (p *Parser) IsNamespaceLocal(node *sitter.Node) bool {
	n := len(p.nsFrames)
	if n == 0 || p.nsFrames[n-1].locals == nil || node.Type() == "export_statement" {
		return false
	}
	parent := node.Parent()
	return parent != nil && p.nsFrames[n-1].body != nil && parent.Equal(p.nsFrames[n-1].body)
}

// namespaceChain finds the innermost enclosing namespace chain whose visible
// members satisfy has, and returns that chain. At each chain, innermost first,
// the visible members are the body being walked, if it declares that chain,
// plus the exported members of every declaration of the chain (merging).
// `namespace A.B {}` encloses chain A without walking an A body, so only A's
// exported members are visible there, as in TypeScript. inert reports a name
// bound by a merged body's local (see nsFrame): the reference resolves to
// nothing, and the caller must not fall back to an outer definition.
func (p *Parser) namespaceChain(has func(nsMembers) bool) (scope []string, found, inert bool) {
	for n := len(p.nsScope); n > 0; n-- {
		chain := p.nsScope[:n]
		key := chainKey(chain)
		for i := len(p.nsFrames) - 1; i >= 0; i-- {
			f := p.nsFrames[i]
			if chainKey(f.scope) != key {
				continue
			}
			if f.locals != nil && has(*f.locals) {
				return nil, false, true
			}
			if has(f.own) {
				return chain, true, false
			}
		}
		if m, ok := p.nsExported[key]; ok && has(m) {
			return chain, true, false
		}
	}
	return nil, false, false
}

// namespaceFunc finds name among the enclosing namespaces' visible functions.
func (p *Parser) namespaceFunc(name string) (scope []string, found, inert bool) {
	return p.namespaceChain(func(m nsMembers) bool { return m.funcs[name] })
}

// namespaceType finds name among the enclosing namespaces' visible types,
// returning the declaring chain and the type's kind.
func (p *Parser) namespaceType(name string) (scope []string, kind ast.CodeEntityType, found, inert bool) {
	scope, found, inert = p.namespaceChain(func(m nsMembers) bool {
		k, ok := m.kinds[name]
		if ok {
			kind = k
		}
		return ok
	})
	return scope, kind, found, inert
}

// appendRef appends a resolved reference, dropping an inert one ("").
func appendRef(refs []string, id string) []string {
	if id == "" {
		return refs
	}
	return append(refs, id)
}

// memberInstanceName is the identity a class member is built under: its name,
// plus a discriminator for the forms that may share a name in one class body.
// A getter and setter pair become name-get / name-set, and a static member
// name-static. Applied to every such member, never only to colliding ones, so
// adding a sibling never re-identifies an existing member (#331).
func memberInstanceName(node *sitter.Node, name string) string {
	instance := name
	for i := 0; i < int(node.ChildCount()); i++ {
		switch node.Child(i).Type() {
		case "static":
			instance += "-static"
		case "get":
			instance += "-get"
		case "set":
			instance += "-set"
		}
	}
	return instance
}

// isStaticMember reports a class member declared static.
func isStaticMember(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == "static" {
			return true
		}
	}
	return false
}

// extractClass extracts a class entity
func (p *Parser) extractClass(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeClass, p.nsScope, name, filePath)
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

	// Extract extends and implements clauses by walking all children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		if childType == "class_heritage" {
			// Walk through heritage clauses
			for j := 0; j < int(child.ChildCount()); j++ {
				heritage := child.Child(j)
				heritageType := heritage.Type()

				if heritageType == "extends_clause" {
					// A class extends a class.
					for k := 0; k < int(heritage.ChildCount()); k++ {
						typeNode := heritage.Child(k)
						if typeNode.Type() == "identifier" || typeNode.Type() == "type_identifier" {
							superClass := nodeText(typeNode, source)
							entity.Extends = appendRef(entity.Extends, p.hierarchyRefID(superClass, ast.TypeClass, filePath))
						}
					}
				} else if heritageType == "implements_clause" {
					// A class implements interfaces.
					for k := 0; k < int(heritage.ChildCount()); k++ {
						typeNode := heritage.Child(k)
						if typeNode.Type() == "identifier" || typeNode.Type() == "type_identifier" {
							iface := nodeText(typeNode, source)
							entity.Implements = appendRef(entity.Implements, p.hierarchyRefID(iface, ast.TypeInterface, filePath))
						}
					}
				}
			}
		}
	}

	// Extract decorators if present (rendered as supplemental metadata).
	var metadata string
	if decorators := p.extractDecorators(node, source); len(decorators) > 0 {
		metadata = "Decorators: " + strings.Join(decorators, ", ")
	}
	entity.DocComment = p.docCommentFor(node, source, metadata)

	return entity
}

// extractClassMethods extracts methods from a class. scope is the chain of
// containing class names (typically just the class itself for non-nested
// classes) and is propagated to every method so their instance IDs include
// the class qualifier.
func (p *Parser) extractClassMethods(classNode *sitter.Node, source []byte, filePath, lang, parentID string, scope []string) []*ast.CodeEntity {
	var methods []*ast.CodeEntity

	bodyNode := classNode.ChildByFieldName("body")
	if bodyNode == nil {
		return methods
	}

	// Computed once per class and threaded through so a this.-call inside any
	// of its methods resolves against the same own-class method set the
	// entities below were built from (calls.go).
	instanceMethods := classMethodNames(bodyNode, source, false)
	staticMethods := classMethodNames(bodyNode, source, true)

	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() == "method_definition" {
			// `this` in a static method is the class, so its this.-calls reach
			// the static members; in an instance method, the instance members.
			classMethods := instanceMethods
			if isStaticMember(child) {
				classMethods = staticMethods
			}
			method := p.extractMethod(child, source, filePath, lang, parentID, scope, classMethods)
			if method != nil {
				methods = append(methods, method)
			}
		}
	}

	return methods
}

// extractMethod extracts a method entity. classMethods maps the enclosing
// class's callable method names to their instance names (calls.go's
// classMethodNames), used to resolve this.-calls in the method body.
func (p *Parser) extractMethod(node *sitter.Node, source []byte, filePath, lang, parentID string, scope []string, classMethods map[string]string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)

	// Skip constructor
	if name == "constructor" {
		return nil
	}

	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeMethod, scope, memberInstanceName(node, name), filePath)
	entity.Name = name
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineMethodVisibility(node, source)

	var metadata string
	if p.hasModifier(node, "async") {
		metadata = "Async method"
	}
	entity.DocComment = p.docCommentFor(node, source, metadata)
	entity.Signature = ast.RenderTSFunctionSignature(node, source)

	// Extract parameters and return type
	p.extractFunctionSignature(node, source, entity)

	// Extract resolved call edges from the body (see calls.go).
	if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
		entity.Calls = p.extractCalls(node.ChildByFieldName("parameters"), bodyNode, source, filePath, lang, scope, classMethods)
	}

	return entity
}

// extractInterface extracts an interface entity
func (p *Parser) extractInterface(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeInterface, p.nsScope, name, filePath)
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)
	entity.DocComment = p.docCommentFor(node, source, "")

	// Extract extends by walking all children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "extends_clause" || child.Type() == "extends_type_clause" {
			// An interface extends interfaces.
			for j := 0; j < int(child.ChildCount()); j++ {
				typeNode := child.Child(j)
				nodeType := typeNode.Type()
				if nodeType == "identifier" || nodeType == "type_identifier" {
					extType := nodeText(typeNode, source)
					entity.Extends = appendRef(entity.Extends, p.hierarchyRefID(extType, ast.TypeInterface, filePath))
				}
			}
		}
	}

	return entity
}

// extractTypeAlias extracts a type alias entity
func (p *Parser) extractTypeAlias(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeType, p.nsScope, name, filePath)
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)
	entity.DocComment = p.docCommentFor(node, source, "")

	return entity
}

// extractEnum extracts an enum entity
func (p *Parser) extractEnum(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeEnum, p.nsScope, name, filePath)
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)
	entity.DocComment = p.docCommentFor(node, source, "")

	return entity
}

// extractFunction extracts a function entity
func (p *Parser) extractFunction(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeFunction, p.nsScope, name, filePath)
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

	var metadata string
	if p.hasModifier(node, "async") {
		metadata = "Async function"
	}
	entity.DocComment = p.docCommentFor(node, source, metadata)
	entity.Signature = ast.RenderTSFunctionSignature(node, source)

	// Extract parameters and return type
	p.extractFunctionSignature(node, source, entity)

	// Extract resolved call edges from the body (see calls.go). No enclosing
	// class for a top-level function, so scope/classMethods are nil — this.
	// calls cannot arise here.
	if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
		entity.Calls = p.extractCalls(node.ChildByFieldName("parameters"), bodyNode, source, filePath, lang, nil, nil)
	}

	return entity
}

// extractVariableDeclaration extracts const/let/var declarations
func (p *Parser) extractVariableDeclaration(node *sitter.Node, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	// Determine if it's const, let, or var
	kind := "const"
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "const" {
			kind = "const"
			break
		} else if child.Type() == "let" {
			kind = "let"
			break
		} else if child.Type() == "var" {
			kind = "var"
			break
		}
	}

	// Find variable declarators
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "variable_declarator" {
			entities = append(entities,
				p.extractVariableDeclarator(child, source, filePath, lang, parentID, kind, node)...)
		}
	}

	return entities
}

// extractVariableDeclarator extracts the entities a single variable declarator
// declares. Most declarators bind one name, but a destructuring pattern binds
// several — `const {a, b, c} = x` declares three symbols, not one — so this
// returns a slice.
func (p *Parser) extractVariableDeclarator(node *sitter.Node, source []byte, filePath, lang, parentID, kind string, declarationNode *sitter.Node) []*ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	if ast.IsDestructuringPattern(nameNode) {
		return p.destructuredEntities(nameNode, source, filePath, lang, parentID, kind, declarationNode)
	}

	entity := p.simpleDeclaratorEntity(node, nameNode, source, filePath, lang, parentID, kind, declarationNode)
	if entity == nil {
		return nil
	}
	return []*ast.CodeEntity{entity}
}

// destructuredEntities emits one entity per name bound by a destructuring
// pattern. Previously the pattern's entire source text became a single entity
// name, which made every bound symbol unsearchable and produced entity IDs long
// enough to be rejected by the graph outright.
//
// Each binding is a real declaration, so it gets the declaration's kind,
// visibility, and doc comment; line numbers come from the binding itself so a
// multi-line pattern points at the right place rather than at the whole
// statement.
func (p *Parser) destructuredEntities(patternNode *sitter.Node, source []byte, filePath, lang, parentID, kind string, declarationNode *sitter.Node) []*ast.CodeEntity {
	entityType := ast.TypeVar
	if kind == "const" {
		entityType = ast.TypeConst
	}
	visibility := p.determineVisibility(declarationNode, source)
	docComment := p.docCommentFor(declarationNode, source, "")

	var bindings []*sitter.Node
	ast.CollectPatternBindings(patternNode, &bindings)

	entities := make([]*ast.CodeEntity, 0, len(bindings))
	for _, binding := range bindings {
		name := nodeText(binding, source)
		if name == "" {
			continue
		}
		entity := ast.NewScopedCodeEntity(p.authority, lang, p.project, entityType, p.nsScope, name, filePath)
		entity.ContainedBy = parentID
		entity.StartLine = int(binding.StartPoint().Row) + 1
		entity.EndLine = int(binding.EndPoint().Row) + 1
		entity.Visibility = visibility
		entity.DocComment = docComment
		entities = append(entities, entity)
	}
	return entities
}

// simpleDeclaratorEntity builds the entity for a declarator that binds a single
// plain identifier.
func (p *Parser) simpleDeclaratorEntity(node, nameNode *sitter.Node, source []byte, filePath, lang, parentID, kind string, declarationNode *sitter.Node) *ast.CodeEntity {
	name := nodeText(nameNode, source)
	valueNode := node.ChildByFieldName("value")

	// Check if this is an arrow function
	isArrowFunc := false
	if valueNode != nil {
		if valueNode.Type() == "arrow_function" {
			isArrowFunc = true
		}
	}

	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	var entity *ast.CodeEntity
	var metadata string
	if isArrowFunc {
		// Create function entity for arrow functions
		entity = ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeFunction, p.nsScope, name, filePath)

		// Check for async arrow function
		if valueNode != nil {
			for i := 0; i < int(valueNode.ChildCount()); i++ {
				child := valueNode.Child(i)
				if child.Type() == "async" {
					metadata = "Async arrow function"
					break
				}
			}
		}

		// Extract function signature from arrow function
		if valueNode != nil {
			p.extractFunctionSignature(valueNode, source, entity)
			entity.Signature = ast.RenderTSArrowSignature(name, valueNode, source)
			// Resolved call edges from the arrow body (see calls.go). No class
			// scope is threaded here regardless of nesting depth: an arrow
			// declared inside a method is already covered by that method's own
			// extractCalls walk (which does not stop at nested-function
			// boundaries), so this only under-counts the arrow's OWN entity,
			// never over-counts or produces a wrong edge.
			if bodyNode := valueNode.ChildByFieldName("body"); bodyNode != nil {
				entity.Calls = p.extractCalls(ArrowParamsNode(valueNode), bodyNode, source, filePath, lang, nil, nil)
			}
		}
	} else if kind == "const" {
		entity = ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeConst, p.nsScope, name, filePath)
	} else {
		entity = ast.NewScopedCodeEntity(p.authority, lang, p.project, ast.TypeVar, p.nsScope, name, filePath)
	}

	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(declarationNode, source)
	entity.DocComment = p.docCommentFor(declarationNode, source, metadata)

	return entity
}

// extractImports extracts import statements
func (p *Parser) extractImports(node *sitter.Node, source []byte) []string {
	var imports []string
	seen := make(map[string]bool)

	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	p.walkImports(cursor, source, &imports, seen)

	return imports
}

// walkImports walks the tree to find import statements
func (p *Parser) walkImports(cursor *sitter.TreeCursor, source []byte, imports *[]string, seen map[string]bool) {
	node := cursor.CurrentNode()

	if node.Type() == "import_statement" {
		sourceNode := node.ChildByFieldName("source")
		if sourceNode != nil {
			importPath := strings.Trim(nodeText(sourceNode, source), `'"`)
			if !seen[importPath] {
				seen[importPath] = true
				*imports = append(*imports, importPath)
			}
		}
	}

	// Also handle require() calls for CommonJS
	if node.Type() == "call_expression" {
		functionNode := node.ChildByFieldName("function")
		if functionNode != nil && nodeText(functionNode, source) == "require" {
			argsNode := node.ChildByFieldName("arguments")
			if argsNode != nil && argsNode.ChildCount() > 0 {
				for i := 0; i < int(argsNode.ChildCount()); i++ {
					child := argsNode.Child(i)
					if child.Type() == "string" {
						importPath := strings.Trim(nodeText(child, source), `'"`)
						if !seen[importPath] {
							seen[importPath] = true
							*imports = append(*imports, importPath)
						}
					}
				}
			}
		}
	}

	if cursor.GoToFirstChild() {
		for {
			p.walkImports(cursor, source, imports, seen)
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}
}

// extractFunctionSignature extracts parameters and return type from function/method
func (p *Parser) extractFunctionSignature(node *sitter.Node, source []byte, entity *ast.CodeEntity) {
	// Extract parameters
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		for i := 0; i < int(paramsNode.ChildCount()); i++ {
			child := paramsNode.Child(i)
			if child.Type() == "required_parameter" || child.Type() == "optional_parameter" {
				typeNode := child.ChildByFieldName("type")
				if typeNode != nil {
					paramType := nodeText(typeNode, source)
					entity.Parameters = appendRef(entity.Parameters, p.typeRefID(paramType, entity.Path))
				}
			}
		}
	}

	// Extract return type
	returnTypeNode := node.ChildByFieldName("return_type")
	if returnTypeNode != nil {
		returnType := nodeText(returnTypeNode, source)
		// Remove leading colon if present
		returnType = strings.TrimPrefix(strings.TrimSpace(returnType), ":")
		entity.Returns = appendRef(entity.Returns, p.typeRefID(returnType, entity.Path))
	}
}

// docCommentFor merges the JSDoc preceding node (if any) with supplemental
// metadata (decorator list, async marker). Either side may be empty.
func (p *Parser) docCommentFor(node *sitter.Node, content []byte, metadata string) string {
	return ast.CombineDocComment(ast.PrecedingJSDoc(node, content), metadata)
}

// extractDecorators extracts decorators attached to a class/method node.
// Decorators may appear in two structural shapes:
//
//   - Inline (`@Foo class Bar`): decorator is a child of the declaration
//     node itself.
//   - Exported (`@Foo export class Bar`): tree-sitter wraps both the
//     decorator and the declaration as siblings inside an export_statement,
//     so decorators must be collected from the export_statement parent.
//
// Both shapes are handled so the resulting metadata is identical regardless
// of export status.
func (p *Parser) extractDecorators(node *sitter.Node, source []byte) []string {
	var decorators []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "decorator" {
			decorators = append(decorators, nodeText(child, source))
		}
	}

	if parent := node.Parent(); parent != nil && parent.Type() == "export_statement" {
		for i := 0; i < int(parent.ChildCount()); i++ {
			child := parent.Child(i)
			if child.Type() == "decorator" {
				decorators = append(decorators, nodeText(child, source))
			}
		}
	}

	return decorators
}

// determineVisibility determines if an identifier is exported
func (p *Parser) determineVisibility(node *sitter.Node, _ []byte) ast.Visibility {
	// Check if parent or grandparent is export_statement
	parent := node.Parent()
	if parent != nil {
		if parent.Type() == "export_statement" {
			return ast.VisibilityPublic
		}
		grandparent := parent.Parent()
		if grandparent != nil && grandparent.Type() == "export_statement" {
			return ast.VisibilityPublic
		}
	}

	return ast.VisibilityPrivate
}

// determineMethodVisibility determines method visibility from modifiers
func (p *Parser) determineMethodVisibility(node *sitter.Node, source []byte) ast.Visibility {
	// Check for # prefix in name first (JavaScript private field)
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		name := nodeText(nameNode, source)
		if strings.HasPrefix(name, "#") {
			return ast.VisibilityPrivate
		}
	}

	// Check for TypeScript access modifiers (private, protected, public)
	// These can be in various positions, so check all children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		// Check if this is an accessibility modifier node
		if childType == "accessibility_modifier" {
			modifierText := nodeText(child, source)
			if modifierText == "private" || modifierText == "protected" {
				return ast.VisibilityPrivate
			}
		}
	}

	return ast.VisibilityPublic
}

// hasModifier checks if a node has a specific modifier (async, static, etc.)
func (p *Parser) hasModifier(node *sitter.Node, modifier string) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == modifier {
			return true
		}
	}
	return false
}

// isTSBuiltinType returns true if the type is a TypeScript/JavaScript built-in type
func isTSBuiltinType(name string) bool {
	switch name {
	case "string", "number", "boolean", "object", "any", "unknown",
		"void", "null", "undefined", "never", "symbol", "bigint",
		"String", "Number", "Boolean", "Object", "Array", "Function",
		"Promise", "Map", "Set", "Date", "RegExp", "Error":
		return true
	}
	return false
}

// nodeText returns the text content of a node
func nodeText(node *sitter.Node, source []byte) string {
	return node.Content(source)
}

// countLines counts the total number of lines in content
func countLines(content []byte) int {
	count := 1
	for _, b := range content {
		if b == '\n' {
			count++
		}
	}
	return count
}

// IsTargetFile returns true if the file is a TypeScript/JavaScript file
func IsTargetFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs":
		return true
	}
	return false
}
