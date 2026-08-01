// Package c provides C AST parsing and code entity extraction using tree-sitter.
package c

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"

	"github.com/c360studio/semsource/source/ast"
)

func init() {
	ast.DefaultRegistry.Register("c", []string{".c", ".h"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from C source files using tree-sitter.
//
// C has no module system, so nothing here resolves a name to a definition in
// another file: an identifier's meaning depends on which headers the compiler
// was told to include, and this parser never runs a preprocessor. Entity
// identity is carried by the defining file's path instead (see
// ast.BuildScopedInstanceID), which is what keeps two files that each define a
// static function of the same name from collapsing onto one entity.
type Parser struct {
	org      string
	project  string
	repoRoot string
	parser   *sitter.Parser
}

// NewParser creates a new C AST parser.
func NewParser(org, project, repoRoot string) *Parser {
	p := sitter.NewParser()
	p.SetLanguage(c.GetLanguage())
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
		parser:   p,
	}
}

// ParseFile parses a single C file and extracts code entities.
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

	fileEntity := ast.NewCodeEntity(p.org, "c", p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Hash = hash
	fileEntity.StartLine = 1
	fileEntity.EndLine = int(root.EndPoint().Row) + 1
	result.FileEntity = fileEntity
	result.Entities = append(result.Entities, fileEntity)

	childIDs := make([]string, 0)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(i)

		if node.Type() == "preproc_include" {
			if inc := includePath(node, content); inc != "" {
				result.Imports = append(result.Imports, inc)
				fileEntity.Imports = append(fileEntity.Imports, inc)
			}
			continue
		}

		for _, entity := range p.extractTopLevel(node, content, relPath) {
			entity.ContainedBy = fileEntity.ID
			childIDs = append(childIDs, entity.ID)
			result.Entities = append(result.Entities, entity)
		}
	}
	fileEntity.Contains = childIDs

	return result, nil
}

// extractTopLevel converts one top-level node into zero or more entities.
//
// A `typedef struct Point { … } Point;` yields two: the tag `struct Point` and
// the alias `Point`. Both names exist in C and either can be what a reader
// searches for, and they cannot collide because the entity type is its own ID
// segment.
func (p *Parser) extractTopLevel(node *sitter.Node, content []byte, relPath string) []*ast.CodeEntity {
	switch node.Type() {
	case "function_definition":
		if e := p.functionEntity(node, content, relPath); e != nil {
			return []*ast.CodeEntity{e}
		}

	case "declaration":
		// A declaration is either a prototype or a file-scope variable.
		// Prototypes matter: a header-only C library — MAVLink is the case in
		// point — declares its whole API this way and defines almost nothing,
		// so skipping them would index such a repository as nearly empty.
		return p.declarationEntities(node, content, relPath)

	case "struct_specifier", "union_specifier", "enum_specifier":
		if e := p.taggedTypeEntity(node, content, relPath); e != nil {
			return []*ast.CodeEntity{e}
		}

	case "type_definition":
		return p.typedefEntities(node, content, relPath)

	case "preproc_def", "preproc_function_def":
		if e := p.macroEntity(node, content, relPath); e != nil {
			return []*ast.CodeEntity{e}
		}

	case "preproc_ifdef", "preproc_if", "preproc_else", "preproc_elif",
		"preproc_ifdef_in_field_declaration_list", "preproc_if_in_field_declaration_list":
		// No preprocessor runs, so which branch compiles is unknowable. Both are
		// read: a symbol that exists only under one build flag is still a symbol
		// someone searches for, and skipping the block would have hidden every
		// declaration inside it. Measured on MAVLink, 893 top-level conditional
		// blocks would otherwise contribute nothing at all.
		//
		// A symbol declared in two mutually exclusive branches lands on one
		// entity ID, which is correct: it is one logical symbol.
		var out []*ast.CodeEntity
		for i := 0; i < int(node.NamedChildCount()); i++ {
			out = append(out, p.extractTopLevel(node.NamedChild(i), content, relPath)...)
		}
		return out
	}
	return nil
}

// functionEntity builds an entity for a function definition.
func (p *Parser) functionEntity(node *sitter.Node, content []byte, relPath string) *ast.CodeEntity {
	decl := functionDeclarator(node.ChildByFieldName("declarator"))
	if decl == nil {
		return nil
	}
	name := declaratorName(decl.ChildByFieldName("declarator"), content)
	if name == "" {
		return nil
	}

	entity := p.newEntity(ast.TypeFunction, name, relPath, node)
	entity.Signature = functionSignature(node, decl, content)
	entity.DocComment = docComment(node, content)
	return entity
}

// declarationEntities handles both function prototypes and file-scope
// variables, which share the `declaration` node type.
func (p *Parser) declarationEntities(node *sitter.Node, content []byte, relPath string) []*ast.CodeEntity {
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil {
		return nil
	}

	if fn := functionDeclarator(declarator); fn != nil {
		name := declaratorName(fn.ChildByFieldName("declarator"), content)
		if name == "" {
			return nil
		}
		entity := p.newEntity(ast.TypeFunction, name, relPath, node)
		entity.Signature = functionSignature(node, fn, content)
		entity.DocComment = docComment(node, content)
		return []*ast.CodeEntity{entity}
	}

	name := declaratorName(declarator, content)
	if name == "" {
		return nil
	}
	entity := p.newEntity(ast.TypeVar, name, relPath, node)
	entity.DocComment = docComment(node, content)
	return []*ast.CodeEntity{entity}
}

// taggedTypeEntity builds an entity for a named struct, union, or enum. An
// anonymous specifier has no name to be found by and is skipped.
func (p *Parser) taggedTypeEntity(node *sitter.Node, content []byte, relPath string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	entity := p.newEntity(specifierType(node.Type()), nameNode.Content(content), relPath, node)
	entity.DocComment = docComment(node, content)
	return entity
}

// typedefEntities builds the alias entity, plus the tag it wraps when that tag
// is named.
func (p *Parser) typedefEntities(node *sitter.Node, content []byte, relPath string) []*ast.CodeEntity {
	var out []*ast.CodeEntity

	if aliasNode := node.ChildByFieldName("declarator"); aliasNode != nil {
		if name := declaratorName(aliasNode, content); name != "" {
			entity := p.newEntity(ast.TypeType, name, relPath, node)
			entity.DocComment = docComment(node, content)
			out = append(out, entity)
		}
	}

	if inner := node.ChildByFieldName("type"); inner != nil {
		switch inner.Type() {
		case "struct_specifier", "union_specifier", "enum_specifier":
			if e := p.taggedTypeEntity(inner, content, relPath); e != nil {
				out = append(out, e)
			}
		}
	}
	return out
}

// macroEntity builds an entity for a #define. Macros are the closest thing C
// has to a named constant, and in a firmware codebase they carry a large share
// of the configuration surface.
func (p *Parser) macroEntity(node *sitter.Node, content []byte, relPath string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	entity := p.newEntity(ast.TypeConst, nameNode.Content(content), relPath, node)
	if params := node.ChildByFieldName("parameters"); params != nil {
		entity.Signature = entity.Name + params.Content(content)
	}
	entity.DocComment = docComment(node, content)
	return entity
}

// newEntity constructs an entity with the fields every kind shares.
func (p *Parser) newEntity(t ast.CodeEntityType, name, relPath string, node *sitter.Node) *ast.CodeEntity {
	entity := ast.NewCodeEntity(p.org, "c", p.project, t, name, relPath)
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	return entity
}

// specifierType maps a tree-sitter specifier node type to an entity type. C's
// union has no dedicated entity type; it is recorded as a struct, being the
// same shape of thing — a named record — for retrieval purposes.
func specifierType(nodeType string) ast.CodeEntityType {
	if nodeType == "enum_specifier" {
		return ast.TypeEnum
	}
	return ast.TypeStruct
}

// functionDeclarator unwraps pointer declarators to find the function
// declarator beneath. `struct Node *next(void)` nests one inside a
// pointer_declarator, and a pointer-to-pointer return nests two.
func functionDeclarator(node *sitter.Node) *sitter.Node {
	for node != nil {
		switch node.Type() {
		case "function_declarator":
			return node
		case "pointer_declarator", "parenthesized_declarator":
			node = node.ChildByFieldName("declarator")
		default:
			return nil
		}
	}
	return nil
}

// declaratorName unwraps the declarator chain to the identifier being declared.
// `static int counter = 0` reaches it through an init_declarator, `int arr[10]`
// through an array_declarator, `char *s` through a pointer_declarator.
func declaratorName(node *sitter.Node, content []byte) string {
	for node != nil {
		switch node.Type() {
		case "identifier", "type_identifier", "field_identifier":
			return node.Content(content)
		case "init_declarator", "array_declarator", "pointer_declarator",
			"parenthesized_declarator", "function_declarator":
			node = node.ChildByFieldName("declarator")
		default:
			return ""
		}
	}
	return ""
}

// functionSignature renders a single-line signature: return type, name, and
// parameter list as written.
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
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return ""
	}
	return strings.Trim(pathNode.Content(content), `"<>`)
}

// docComment returns the documentation immediately preceding node.
//
// The shared ast.PrecedingDocComment helper recognises only `/** … */`, which is
// the Javadoc/JSDoc convention it was written for. C's documentation convention
// is Doxygen, which is mostly `///` and `//!` line runs and `/*! … */` blocks —
// so relying on the shared helper alone would leave the majority of a firmware
// codebase's comments unindexed. Doc comments are embedded and ranked, so that
// is a retrieval-quality loss, not a cosmetic one.
func docComment(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	prev := node.PrevNamedSibling()
	if prev == nil || prev.Type() != "comment" {
		return ""
	}
	// The comment must sit directly above the declaration. One separated by a
	// blank line is describing something else — often the file as a whole — and
	// attaching it would document this symbol with another's prose, which reads
	// as authoritative while being wrong.
	if int(node.StartPoint().Row)-int(prev.EndPoint().Row) > 1 {
		return ""
	}

	text := strings.TrimSpace(prev.Content(content))
	switch {
	case strings.HasPrefix(text, "/**"), strings.HasPrefix(text, "/*!"):
		// CleanDocCommentBlock strips the Javadoc delimiters; normalise the
		// Doxygen "/*!" spelling onto it rather than duplicating the cleaner.
		return ast.CleanDocCommentBlock("/**" + strings.TrimPrefix(strings.TrimPrefix(text, "/*!"), "/**"))
	case strings.HasPrefix(text, "///"), strings.HasPrefix(text, "//!"):
		return lineCommentRun(prev, content)
	}
	return ""
}

// lineCommentRun collects a run of adjacent Doxygen line comments ending at
// last, returning them oldest-first. Adjacency is required: a comment separated
// by a blank line documents something else, and folding it in would attribute
// one symbol's description to another.
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

// collapseSpace flattens a multi-line construct onto one line so a signature
// stays readable when a parameter list is wrapped in the source.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
