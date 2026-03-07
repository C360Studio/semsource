// Package ts provides TypeScript and JavaScript AST parsing and code entity extraction using tree-sitter.
package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/source/ast"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func init() {
	ast.DefaultRegistry.Register("typescript",
		[]string{".ts", ".tsx", ".mts", ".cts"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
	ast.DefaultRegistry.Register("javascript",
		[]string{".js", ".jsx", ".mjs", ".cjs"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from TypeScript/JavaScript source files using tree-sitter
type Parser struct {
	org      string
	project  string
	repoRoot string
}

// NewParser creates a new TypeScript/JavaScript parser
func NewParser(org, project, repoRoot string) *Parser {
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
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
	fileEntity := ast.NewCodeEntity(p.org, p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Hash = hash
	fileEntity.Language = lang
	fileEntity.StartLine = 1
	fileEntity.EndLine = countLines(content)

	parseResult.FileEntity = fileEntity
	parseResult.Entities = append(parseResult.Entities, fileEntity)

	// Extract entities from the AST
	rootNode := tree.RootNode()
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

	// Extract entity based on node type
	switch nodeType {
	case "class_declaration":
		entity := p.extractClass(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
			// Extract methods from the class body
			methods := p.extractClassMethods(node, source, filePath, lang, entity.ID)
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

// extractClass extracts a class entity
func (p *Parser) extractClass(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeClass, name, filePath)
	entity.Language = lang
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
					// Extract the type after 'extends' keyword
					for k := 0; k < int(heritage.ChildCount()); k++ {
						typeNode := heritage.Child(k)
						if typeNode.Type() == "identifier" || typeNode.Type() == "type_identifier" {
							superClass := nodeText(typeNode, source)
							entity.Extends = append(entity.Extends, p.typeNameToEntityID(superClass, filePath))
						}
					}
				} else if heritageType == "implements_clause" {
					// Extract types after 'implements' keyword
					for k := 0; k < int(heritage.ChildCount()); k++ {
						typeNode := heritage.Child(k)
						if typeNode.Type() == "identifier" || typeNode.Type() == "type_identifier" {
							iface := nodeText(typeNode, source)
							entity.Implements = append(entity.Implements, p.typeNameToEntityID(iface, filePath))
						}
					}
				}
			}
		}
	}

	// Extract decorators if present
	decorators := p.extractDecorators(node, source)
	if len(decorators) > 0 {
		if entity.DocComment == "" {
			entity.DocComment = "Decorators: " + strings.Join(decorators, ", ")
		} else {
			entity.DocComment += "\nDecorators: " + strings.Join(decorators, ", ")
		}
	}

	return entity
}

// extractClassMethods extracts methods from a class
func (p *Parser) extractClassMethods(classNode *sitter.Node, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var methods []*ast.CodeEntity

	bodyNode := classNode.ChildByFieldName("body")
	if bodyNode == nil {
		return methods
	}

	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() == "method_definition" {
			method := p.extractMethod(child, source, filePath, lang, parentID)
			if method != nil {
				methods = append(methods, method)
			}
		}
	}

	return methods
}

// extractMethod extracts a method entity
func (p *Parser) extractMethod(node *sitter.Node, source []byte, filePath, lang, parentID string) *ast.CodeEntity {
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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeMethod, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineMethodVisibility(node, source)

	// Check for async
	if p.hasModifier(node, "async") {
		if entity.DocComment == "" {
			entity.DocComment = "Async method"
		}
	}

	// Extract parameters and return type
	p.extractFunctionSignature(node, source, entity)

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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeInterface, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

	// Extract extends by walking all children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "extends_clause" || child.Type() == "extends_type_clause" {
			// Extract all types in the extends clause
			for j := 0; j < int(child.ChildCount()); j++ {
				typeNode := child.Child(j)
				nodeType := typeNode.Type()
				if nodeType == "identifier" || nodeType == "type_identifier" {
					extType := nodeText(typeNode, source)
					entity.Extends = append(entity.Extends, p.typeNameToEntityID(extType, filePath))
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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeType, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeEnum, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeFunction, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(node, source)

	// Check for async
	if p.hasModifier(node, "async") {
		if entity.DocComment == "" {
			entity.DocComment = "Async function"
		}
	}

	// Extract parameters and return type
	p.extractFunctionSignature(node, source, entity)

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
			entity := p.extractVariableDeclarator(child, source, filePath, lang, parentID, kind, node)
			if entity != nil {
				entities = append(entities, entity)
			}
		}
	}

	return entities
}

// extractVariableDeclarator extracts a single variable declarator
func (p *Parser) extractVariableDeclarator(node *sitter.Node, source []byte, filePath, lang, parentID, kind string, declarationNode *sitter.Node) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

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
	if isArrowFunc {
		// Create function entity for arrow functions
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeFunction, name, filePath)

		// Check for async arrow function
		if valueNode != nil {
			for i := 0; i < int(valueNode.ChildCount()); i++ {
				child := valueNode.Child(i)
				if child.Type() == "async" {
					if entity.DocComment == "" {
						entity.DocComment = "Async arrow function"
					}
					break
				}
			}
		}

		// Extract function signature from arrow function
		if valueNode != nil {
			p.extractFunctionSignature(valueNode, source, entity)
		}
	} else if kind == "const" {
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeConst, name, filePath)
	} else {
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeVar, name, filePath)
	}

	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = p.determineVisibility(declarationNode, source)

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
					entity.Parameters = append(entity.Parameters, p.typeNameToEntityID(paramType, entity.Path))
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
		entity.Returns = append(entity.Returns, p.typeNameToEntityID(returnType, entity.Path))
	}
}

// extractDecorators extracts decorators from a node
func (p *Parser) extractDecorators(node *sitter.Node, source []byte) []string {
	var decorators []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "decorator" {
			decorators = append(decorators, nodeText(child, source))
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

// typeNameToEntityID converts a type name to an entity ID reference
func (p *Parser) typeNameToEntityID(typeName, filePath string) string {
	if typeName == "" {
		return ""
	}

	// Clean up type name (remove generics, etc.)
	typeName = strings.TrimSpace(typeName)

	// Strip generic type parameters
	if idx := strings.Index(typeName, "<"); idx > 0 {
		typeName = typeName[:idx]
	}

	// Check for built-in types
	if isTSBuiltinType(typeName) {
		return fmt.Sprintf("builtin:%s", typeName)
	}

	// Local type: create entity ID within current project
	instance := ast.BuildInstanceID(filePath, typeName, ast.TypeType)
	return fmt.Sprintf("%s.semspec.code.type.%s.%s", p.org, p.project, instance)
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
