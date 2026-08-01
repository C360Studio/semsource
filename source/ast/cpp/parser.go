// Package cpp provides C++ AST parsing and code entity extraction using tree-sitter.
package cpp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"

	"github.com/c360studio/semsource/source/ast"
)

func init() {
	// ".h" is claimed here as well as by the C parser. Which one reads a header
	// is decided per watch path from the declared language set, not by this
	// registration — see processor/ast-source/routing.go.
	ast.DefaultRegistry.Register("cpp", []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx", ".h"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from C++ source files using tree-sitter.
//
// No preprocessor runs, so a symbol that exists only after macro expansion or
// inside an inactive #if branch is not extracted. Identity is carried by the
// defining file's path (ast.BuildScopedInstanceID) plus the enclosing namespace
// and class chain, which is what keeps two same-named methods on different
// classes — or the same class compiled into two files — from colliding.
type Parser struct {
	org      string
	project  string
	repoRoot string
	parser   *sitter.Parser
}

// NewParser creates a new C++ AST parser.
func NewParser(org, project, repoRoot string) *Parser {
	p := sitter.NewParser()
	p.SetLanguage(cpp.GetLanguage())
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
		parser:   p,
	}
}

// ParseFile parses a single C++ file and extracts code entities.
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*ast.ParseResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	hash := ast.ComputeHash(content)

	relPath, err := filepath.Rel(p.repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	tree, err := p.parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse file: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	result := &ast.ParseResult{
		Path:     relPath,
		Hash:     hash,
		Imports:  make([]string, 0),
		Entities: make([]*ast.CodeEntity, 0),
	}

	fileEntity := ast.NewCodeEntity(p.org, "cpp", p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Hash = hash
	fileEntity.StartLine = 1
	fileEntity.EndLine = int(root.EndPoint().Row) + 1
	result.FileEntity = fileEntity
	result.Entities = append(result.Entities, fileEntity)

	childIDs := make([]string, 0)
	p.walk(root, content, relPath, nil, func(entity *ast.CodeEntity, topLevel bool) {
		if topLevel {
			entity.ContainedBy = fileEntity.ID
			childIDs = append(childIDs, entity.ID)
		}
		result.Entities = append(result.Entities, entity)
	})

	for i := 0; i < int(root.NamedChildCount()); i++ {
		if inc := includePath(root.NamedChild(i), content); inc != "" {
			result.Imports = append(result.Imports, inc)
			fileEntity.Imports = append(fileEntity.Imports, inc)
		}
	}

	fileEntity.Contains = childIDs
	return result, nil
}

// emit receives each extracted entity; topLevel marks those directly under the
// file rather than nested in a class or namespace.
type emit func(entity *ast.CodeEntity, topLevel bool)

// walk descends a declaration list, carrying the enclosing namespace/class chain
// as scope. Scope is what disambiguates two methods of the same name on
// different classes in one file.
func (p *Parser) walk(node *sitter.Node, content []byte, relPath string, scope []string, out emit) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		p.visit(node.NamedChild(i), content, relPath, scope, out)
	}
}

func (p *Parser) visit(node *sitter.Node, content []byte, relPath string, scope []string, out emit) {
	topLevel := len(scope) == 0

	switch node.Type() {
	case "namespace_definition":
		// A namespace is recorded as a type rather than a package: the package
		// entity type deliberately drops the name from its instance ID, so two
		// namespaces in one file would collide on a single identity.
		name := nodeText(node.ChildByFieldName("name"), content)
		if name != "" {
			entity := p.newEntity(ast.TypeType, name, relPath, scope, node)
			entity.DocComment = docComment(node, content)
			out(entity, topLevel)
		}
		if body := node.ChildByFieldName("body"); body != nil {
			p.walk(body, content, relPath, append(scope, name), out)
		}

	case "template_declaration":
		// The templated class or function is an unnamed child alongside the
		// parameter list. Instantiations are not visible without a compiler, so
		// the declaration is what gets indexed.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() != "template_parameter_list" {
				p.visit(child, content, relPath, scope, out)
			}
		}

	case "class_specifier", "struct_specifier", "union_specifier":
		p.classEntity(node, content, relPath, scope, out)

	case "enum_specifier":
		if name := nodeText(node.ChildByFieldName("name"), content); name != "" {
			entity := p.newEntity(ast.TypeEnum, name, relPath, scope, node)
			entity.DocComment = docComment(node, content)
			out(entity, topLevel)
		}

	case "alias_declaration":
		if name := nodeText(node.ChildByFieldName("name"), content); name != "" {
			entity := p.newEntity(ast.TypeType, name, relPath, scope, node)
			entity.DocComment = docComment(node, content)
			out(entity, topLevel)
		}

	case "type_definition":
		if name := declaratorName(node.ChildByFieldName("declarator"), content); name != "" {
			entity := p.newEntity(ast.TypeType, name, relPath, scope, node)
			entity.DocComment = docComment(node, content)
			out(entity, topLevel)
		}

	case "function_definition", "declaration", "field_declaration":
		p.callableOrField(node, content, relPath, scope, out)

	case "preproc_def", "preproc_function_def":
		if name := nodeText(node.ChildByFieldName("name"), content); name != "" {
			entity := p.newEntity(ast.TypeConst, name, relPath, scope, node)
			if params := node.ChildByFieldName("parameters"); params != nil {
				entity.Signature = name + params.Content(content)
			}
			entity.DocComment = docComment(node, content)
			out(entity, topLevel)
		}

	case "linkage_specification":
		// extern "C" { … } — a linkage wrapper, not a scope.
		if body := node.ChildByFieldName("body"); body != nil {
			p.walk(body, content, relPath, scope, out)
		}
	}
}

// classEntity emits a class, struct, or union and then descends into its body so
// members carry the class as scope.
func (p *Parser) classEntity(node *sitter.Node, content []byte, relPath string, scope []string, out emit) {
	name := nodeText(node.ChildByFieldName("name"), content)
	if name == "" {
		return // anonymous: nothing to find it by
	}

	entityType := ast.TypeClass
	if node.Type() != "class_specifier" {
		entityType = ast.TypeStruct
	}
	entity := p.newEntity(entityType, name, relPath, scope, node)
	entity.DocComment = docComment(node, content)
	entity.Extends = baseClasses(node, content)
	out(entity, len(scope) == 0)

	if body := node.ChildByFieldName("body"); body != nil {
		p.walk(body, content, relPath, append(scope, name), out)
	}
}

// callableOrField handles the three nodes that may hold a function or a
// variable, inside a class or at file scope.
func (p *Parser) callableOrField(node *sitter.Node, content []byte, relPath string, scope []string, out emit) {
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil {
		return
	}
	topLevel := len(scope) == 0

	if fn := functionDeclarator(declarator); fn != nil {
		inner := fn.ChildByFieldName("declarator")
		name, extraScope := callableName(inner, content)
		if name == "" {
			return
		}

		// A method is a method wherever it is written: inside the class body, or
		// out of line as `int Radio::send(...)`. The qualified form contributes
		// its own scope so both spellings agree on the enclosing class.
		effectiveScope := append(append([]string(nil), scope...), extraScope...)
		entityType := ast.TypeFunction
		if len(effectiveScope) > 0 {
			entityType = ast.TypeMethod
		}

		entity := p.newEntity(entityType, name, relPath, effectiveScope, node)
		entity.Signature = functionSignature(node, fn, content)
		entity.DocComment = docComment(node, content)
		out(entity, topLevel)
		return
	}

	name := declaratorName(declarator, content)
	if name == "" {
		return
	}
	entity := p.newEntity(ast.TypeVar, name, relPath, scope, node)
	entity.DocComment = docComment(node, content)
	out(entity, topLevel)
}

// callableName returns a callable's name and any scope its declarator carries.
// A destructor keeps its "~" so it cannot be confused with the constructor, and
// a qualified name yields the class it was defined against.
func callableName(node *sitter.Node, content []byte) (name string, scope []string) {
	if node == nil {
		return "", nil
	}
	switch node.Type() {
	case "destructor_name":
		return "~" + nodeText(node.NamedChild(0), content), nil
	case "qualified_identifier":
		var qualifiers []string
		cur := node
		for cur != nil && cur.Type() == "qualified_identifier" {
			qualifiers = append(qualifiers, nodeText(cur.ChildByFieldName("scope"), content))
			cur = cur.ChildByFieldName("name")
		}
		inner, innerScope := callableName(cur, content)
		return inner, append(qualifiers, innerScope...)
	case "identifier", "field_identifier", "operator_name":
		return node.Content(content), nil
	}
	return declaratorName(node, content), nil
}

// baseClasses lists the types a class derives from, for the extends edge.
func baseClasses(node *sitter.Node, content []byte) []string {
	var out []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "base_class_clause" {
			continue
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			base := child.NamedChild(j)
			if base.Type() == "type_identifier" || base.Type() == "qualified_identifier" {
				out = append(out, base.Content(content))
			}
		}
	}
	return out
}

// newEntity constructs a scoped entity with the fields every kind shares.
func (p *Parser) newEntity(t ast.CodeEntityType, name, relPath string, scope []string, node *sitter.Node) *ast.CodeEntity {
	entity := ast.NewScopedCodeEntity(p.org, "cpp", p.project, t, scope, name, relPath)
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	return entity
}

// functionDeclarator unwraps pointer and reference declarators to the function
// declarator beneath.
func functionDeclarator(node *sitter.Node) *sitter.Node {
	for node != nil {
		switch node.Type() {
		case "function_declarator":
			return node
		case "pointer_declarator", "reference_declarator", "parenthesized_declarator":
			node = node.ChildByFieldName("declarator")
		default:
			return nil
		}
	}
	return nil
}

// declaratorName unwraps a declarator chain to the identifier being declared.
func declaratorName(node *sitter.Node, content []byte) string {
	for node != nil {
		switch node.Type() {
		case "identifier", "type_identifier", "field_identifier", "namespace_identifier":
			return node.Content(content)
		case "init_declarator", "array_declarator", "pointer_declarator",
			"reference_declarator", "parenthesized_declarator", "function_declarator":
			node = node.ChildByFieldName("declarator")
		default:
			return ""
		}
	}
	return ""
}

// functionSignature renders a single-line signature.
func functionSignature(node, decl *sitter.Node, content []byte) string {
	var b strings.Builder
	if ret := node.ChildByFieldName("type"); ret != nil {
		b.WriteString(collapseSpace(ret.Content(content)))
		b.WriteString(" ")
	}
	b.WriteString(collapseSpace(decl.Content(content)))
	return strings.TrimSpace(b.String())
}

// includePath returns the header named by a #include, without its delimiters.
func includePath(node *sitter.Node, content []byte) string {
	if node.Type() != "preproc_include" {
		return ""
	}
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return ""
	}
	return strings.Trim(pathNode.Content(content), `"<>`)
}

// docComment returns the Doxygen documentation immediately above node. C++ uses
// the same conventions as C — `///` and `//!` runs, `/** */` and `/*! */` blocks
// — so the shared Javadoc-only helper would miss most of it.
func docComment(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	prev := node.PrevNamedSibling()
	if prev == nil || prev.Type() != "comment" {
		return ""
	}
	if int(node.StartPoint().Row)-int(prev.EndPoint().Row) > 1 {
		return "" // separated by a blank line: it documents something else
	}

	text := strings.TrimSpace(prev.Content(content))
	switch {
	case strings.HasPrefix(text, "/**"), strings.HasPrefix(text, "/*!"):
		return ast.CleanDocCommentBlock("/**" + strings.TrimPrefix(strings.TrimPrefix(text, "/*!"), "/**"))
	case strings.HasPrefix(text, "///"), strings.HasPrefix(text, "//!"):
		return lineCommentRun(prev, content)
	}
	return ""
}

// lineCommentRun collects a run of adjacent Doxygen line comments, oldest first.
func lineCommentRun(last *sitter.Node, content []byte) string {
	var lines []string
	node := last
	prevRow := int(last.EndPoint().Row)

	for node != nil && node.Type() == "comment" {
		text := strings.TrimSpace(node.Content(content))
		if !strings.HasPrefix(text, "///") && !strings.HasPrefix(text, "//!") {
			break
		}
		if row := int(node.EndPoint().Row); prevRow-row > 1 {
			break
		}
		prevRow = int(node.StartPoint().Row)

		text = strings.TrimPrefix(strings.TrimPrefix(text, "///"), "//!")
		lines = append([]string{strings.TrimSpace(text)}, lines...)
		node = node.PrevNamedSibling()
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// nodeText returns a node's source text, or "" when the node is absent.
func nodeText(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	return node.Content(content)
}

// collapseSpace flattens a wrapped construct onto one line.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
