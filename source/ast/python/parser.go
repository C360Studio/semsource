// Package python provides Python AST parsing and code entity extraction using tree-sitter.
package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
)

func init() {
	ast.DefaultRegistry.Register("python", []string{".py"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from Python source files using tree-sitter.
type Parser struct {
	org      string
	project  string
	repoRoot string
	parser   *sitter.Parser
}

// NewParser creates a new Python AST parser.
func NewParser(org, project, repoRoot string) *Parser {
	p := sitter.NewParser()
	p.SetLanguage(python.GetLanguage())
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
		parser:   p,
	}
}

// ParseFile parses a single Python file and extracts code entities.
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*ast.ParseResult, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Compute hash for change detection
	hash := ast.ComputeHash(content)

	// Get relative path from repo root
	relPath, err := filepath.Rel(p.repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// Parse with tree-sitter
	tree, err := p.parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse file: %w", err)
	}
	defer tree.Close()

	rootNode := tree.RootNode()

	// Determine module/package name from file path
	moduleName := p.extractModuleName(relPath)

	result := &ast.ParseResult{
		Path:     relPath,
		Hash:     hash,
		Package:  moduleName,
		Imports:  make([]string, 0),
		Entities: make([]*ast.CodeEntity, 0),
	}

	// Create file entity
	fileEntity := ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Package = moduleName
	fileEntity.Hash = hash
	fileEntity.StartLine = 1
	fileEntity.EndLine = int(rootNode.EndPoint().Row) + 1

	// Extract file-level docstring
	if docstring := p.extractModuleDocstring(rootNode, content); docstring != "" {
		fileEntity.DocComment = docstring
	}

	result.FileEntity = fileEntity
	result.Entities = append(result.Entities, fileEntity)

	// Extract entities from the AST
	childIDs := make([]string, 0)
	for i := 0; i < int(rootNode.NamedChildCount()); i++ {
		child := rootNode.NamedChild(i)
		entities := p.extractNode(child, content, fileEntity.ID, relPath)
		for _, entity := range entities {
			entity.ContainedBy = fileEntity.ID
			result.Entities = append(result.Entities, entity)
			childIDs = append(childIDs, entity.ID)
		}

		// Extract imports
		if imports := p.extractImports(child, content); len(imports) > 0 {
			result.Imports = append(result.Imports, imports...)
			fileEntity.Imports = append(fileEntity.Imports, imports...)
		}
	}
	fileEntity.Contains = childIDs

	return result, nil
}

// ParseDirectory parses all Python files in a directory.
func (p *Parser) ParseDirectory(ctx context.Context, dirPath string) ([]*ast.ParseResult, error) {
	var results []*ast.ParseResult

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories and non-Python files
		if info.IsDir() || !strings.HasSuffix(path, ".py") {
			return nil
		}

		// Skip test files and virtual env directories
		relPath, _ := filepath.Rel(p.repoRoot, path)
		if p.shouldSkipPath(relPath) {
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

// shouldSkipPath returns true if the path should be skipped during directory parsing.
func (p *Parser) shouldSkipPath(relPath string) bool {
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range parts {
		// Skip hidden directories
		if strings.HasPrefix(part, ".") {
			return true
		}
		// Skip common Python virtual environments and build directories
		switch part {
		case "venv", ".venv", "env", ".env", "__pycache__", ".pytest_cache",
			"node_modules", "vendor", "dist", "build", ".tox", ".eggs",
			"site-packages", ".mypy_cache":
			return true
		}
	}
	return false
}

// extractModuleName extracts the module name from the file path.
func (p *Parser) extractModuleName(relPath string) string {
	// Remove .py extension
	modPath := strings.TrimSuffix(relPath, ".py")
	// Convert path separators to dots
	modPath = strings.ReplaceAll(modPath, string(filepath.Separator), ".")
	// Handle __init__.py specially
	modPath = strings.TrimSuffix(modPath, ".__init__")
	return modPath
}

// extractModuleDocstring extracts the module-level docstring if present.
func (p *Parser) extractModuleDocstring(node *sitter.Node, content []byte) string {
	if node.NamedChildCount() == 0 {
		return ""
	}

	firstChild := node.NamedChild(0)
	if firstChild.Type() == "expression_statement" && firstChild.NamedChildCount() > 0 {
		expr := firstChild.NamedChild(0)
		if expr.Type() == "string" {
			return p.extractStringContent(expr, content)
		}
	}
	return ""
}

// extractNode extracts entities from a top-level AST node.
func (p *Parser) extractNode(node *sitter.Node, content []byte, _ /* parentID */, filePath string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	switch node.Type() {
	case "class_definition":
		classEntities := p.extractClass(node, content, filePath)
		entities = append(entities, classEntities...)

	case "function_definition":
		entity := p.extractFunction(node, content, filePath, false)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "decorated_definition":
		// Handle decorated classes and functions
		definition := p.findDefinitionInDecorated(node)
		if definition != nil {
			decorators := p.extractDecorators(node, content)
			switch definition.Type() {
			case "class_definition":
				classEntities := p.extractClass(definition, content, filePath)
				if len(classEntities) > 0 {
					// Add decorator info to doc comment of the class entity (first in list)
					if len(decorators) > 0 {
						classEntities[0].DocComment = p.prependDecorators(classEntities[0].DocComment, decorators)
					}
					// Check for dataclass decorator
					for _, dec := range decorators {
						if dec == "@dataclass" || strings.HasPrefix(dec, "@dataclass(") {
							classEntities[0].Type = ast.TypeStruct
						}
					}
					entities = append(entities, classEntities...)
				}
			case "function_definition":
				entity := p.extractFunction(definition, content, filePath, false)
				if entity != nil {
					if len(decorators) > 0 {
						entity.DocComment = p.prependDecorators(entity.DocComment, decorators)
					}
					entities = append(entities, entity)
				}
			}
		}

	case "expression_statement":
		// Module-level assignments (constants, variables)
		entities = append(entities, p.extractAssignment(node, content, filePath)...)
	}

	return entities
}

// extractClass extracts a class entity and all its method entities.
// Returns all entities including the class itself and nested entities.
func (p *Parser) extractClass(node *sitter.Node, content []byte, filePath string) []*ast.CodeEntity {
	// Get class name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, "python", p.project, ast.TypeClass, name, filePath)
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.determineVisibility(name)

	// Extract base classes (extends)
	if argList := node.ChildByFieldName("superclasses"); argList != nil {
		for i := 0; i < int(argList.NamedChildCount()); i++ {
			arg := argList.NamedChild(i)
			baseName := string(content[arg.StartByte():arg.EndByte()])
			// Filter out metaclass and other keyword arguments
			if !strings.Contains(baseName, "=") {
				entity.Extends = append(entity.Extends, p.typeNameToEntityID(baseName, filePath))
			}
		}
	}

	// Extract docstring
	if body := node.ChildByFieldName("body"); body != nil {
		entity.DocComment = p.extractBodyDocstring(body, content)
	}

	// Collect all entities (class + methods)
	allEntities := []*ast.CodeEntity{entity}

	// Extract methods and class variables
	if body := node.ChildByFieldName("body"); body != nil {
		methodIDs := make([]string, 0)
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			switch child.Type() {
			case "function_definition":
				method := p.extractFunction(child, content, filePath, true)
				if method != nil {
					method.ContainedBy = entity.ID
					method.Receiver = entity.ID
					methodIDs = append(methodIDs, method.ID)
					allEntities = append(allEntities, method)
				}
			case "decorated_definition":
				if def := p.findDefinitionInDecorated(child); def != nil {
					if def.Type() == "function_definition" {
						decorators := p.extractDecorators(child, content)
						method := p.extractFunction(def, content, filePath, true)
						if method != nil {
							method.ContainedBy = entity.ID
							method.Receiver = entity.ID
							if len(decorators) > 0 {
								method.DocComment = p.prependDecorators(method.DocComment, decorators)
							}
							methodIDs = append(methodIDs, method.ID)
							allEntities = append(allEntities, method)
						}
					}
				}
			}
		}
		entity.Contains = methodIDs
	}

	return allEntities
}

// extractFunction extracts a function or method entity.
func (p *Parser) extractFunction(node *sitter.Node, content []byte, filePath string, isMethod bool) *ast.CodeEntity {
	// Get function name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entityType := ast.TypeFunction
	if isMethod {
		entityType = ast.TypeMethod
	}

	entity := ast.NewCodeEntity(p.org, "python", p.project, entityType, name, filePath)
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.determineVisibility(name)

	// Extract parameters
	if params := node.ChildByFieldName("parameters"); params != nil {
		entity.Parameters = p.extractParameters(params, content, filePath)
	}

	// Extract return type annotation
	if returnType := node.ChildByFieldName("return_type"); returnType != nil {
		typeStr := string(content[returnType.StartByte():returnType.EndByte()])
		entity.Returns = append(entity.Returns, p.typeNameToEntityID(typeStr, filePath))
	}

	// Extract docstring
	if body := node.ChildByFieldName("body"); body != nil {
		entity.DocComment = p.extractBodyDocstring(body, content)
	}

	// Check if async
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "async" {
			if entity.DocComment != "" {
				entity.DocComment = "async\n" + entity.DocComment
			} else {
				entity.DocComment = "async"
			}
			break
		}
	}

	return entity
}

// extractParameters extracts parameter type annotations.
func (p *Parser) extractParameters(node *sitter.Node, content []byte, filePath string) []string {
	var params []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "typed_parameter", "typed_default_parameter":
			// Extract the type annotation
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				typeStr := string(content[typeNode.StartByte():typeNode.EndByte()])
				params = append(params, p.typeNameToEntityID(typeStr, filePath))
			}
		case "identifier":
			// Untyped parameter - use "any"
			params = append(params, "builtin:any")
		}
	}
	return params
}

// extractAssignment extracts module-level variable/constant assignments.
func (p *Parser) extractAssignment(node *sitter.Node, content []byte, filePath string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	// Look for assignment inside expression_statement
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "assignment" {
			left := child.ChildByFieldName("left")
			if left == nil {
				continue
			}

			var name string
			switch left.Type() {
			case "identifier":
				name = string(content[left.StartByte():left.EndByte()])
			case "pattern_list":
				// Multiple assignment, skip for now
				continue
			}

			if name == "" {
				continue
			}

			// Determine if it's a constant (all caps) or variable
			entityType := ast.TypeVar
			if isAllCaps(name) {
				entityType = ast.TypeConst
			}

			entity := ast.NewCodeEntity(p.org, "python", p.project, entityType, name, filePath)
			entity.StartLine = int(node.StartPoint().Row) + 1
			entity.EndLine = int(node.EndPoint().Row) + 1
			entity.Visibility = p.determineVisibility(name)

			// Check for type annotation
			if typeNode := child.ChildByFieldName("type"); typeNode != nil {
				typeStr := string(content[typeNode.StartByte():typeNode.EndByte()])
				entity.References = append(entity.References, p.typeNameToEntityID(typeStr, filePath))
			}

			entities = append(entities, entity)
		}
	}

	return entities
}

// extractImports extracts import statements.
func (p *Parser) extractImports(node *sitter.Node, content []byte) []string {
	var imports []string

	switch node.Type() {
	case "import_statement":
		// import foo, bar
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "dotted_name" || child.Type() == "aliased_import" {
				importName := string(content[child.StartByte():child.EndByte()])
				// Strip alias if present
				if idx := strings.Index(importName, " as "); idx != -1 {
					importName = importName[:idx]
				}
				imports = append(imports, importName)
			}
		}

	case "import_from_statement":
		// from foo import bar
		moduleNode := node.ChildByFieldName("module_name")
		if moduleNode != nil {
			moduleName := string(content[moduleNode.StartByte():moduleNode.EndByte()])
			imports = append(imports, moduleName)
		}
	}

	return imports
}

// extractDecorators extracts decorator names from a decorated definition.
func (p *Parser) extractDecorators(node *sitter.Node, content []byte) []string {
	var decorators []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "decorator" {
			decText := strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
			decorators = append(decorators, decText)
		}
	}
	return decorators
}

// findDefinitionInDecorated finds the actual definition node inside a decorated_definition.
func (p *Parser) findDefinitionInDecorated(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "class_definition", "function_definition":
			return child
		}
	}
	return nil
}

// extractBodyDocstring extracts the docstring from a function/class body.
func (p *Parser) extractBodyDocstring(body *sitter.Node, content []byte) string {
	if body.NamedChildCount() == 0 {
		return ""
	}

	firstChild := body.NamedChild(0)
	if firstChild.Type() == "expression_statement" && firstChild.NamedChildCount() > 0 {
		expr := firstChild.NamedChild(0)
		if expr.Type() == "string" {
			return p.extractStringContent(expr, content)
		}
	}
	return ""
}

// extractStringContent extracts the actual string content, removing quotes.
func (p *Parser) extractStringContent(node *sitter.Node, content []byte) string {
	raw := string(content[node.StartByte():node.EndByte()])
	// Remove triple quotes
	raw = strings.TrimPrefix(raw, `"""`)
	raw = strings.TrimSuffix(raw, `"""`)
	raw = strings.TrimPrefix(raw, `'''`)
	raw = strings.TrimSuffix(raw, `'''`)
	// Remove single quotes
	raw = strings.TrimPrefix(raw, `"`)
	raw = strings.TrimSuffix(raw, `"`)
	raw = strings.TrimPrefix(raw, `'`)
	raw = strings.TrimSuffix(raw, `'`)
	return strings.TrimSpace(raw)
}

// prependDecorators adds decorator info to the beginning of a docstring.
func (p *Parser) prependDecorators(docstring string, decorators []string) string {
	decStr := strings.Join(decorators, "\n")
	if docstring == "" {
		return decStr
	}
	return decStr + "\n" + docstring
}

// determineVisibility determines visibility based on Python naming conventions.
func (p *Parser) determineVisibility(name string) ast.Visibility {
	if strings.HasPrefix(name, "_") {
		return ast.VisibilityPrivate
	}
	return ast.VisibilityPublic
}

// typeNameToEntityID converts a Python type name to an entity ID reference.
func (p *Parser) typeNameToEntityID(typeName, filePath string) string {
	if typeName == "" {
		return ""
	}

	// Clean up the type name
	typeName = strings.TrimSpace(typeName)

	// Handle generic types like List[int], Dict[str, Any]
	if idx := strings.Index(typeName, "["); idx != -1 {
		baseName := typeName[:idx]
		if isBuiltinType(baseName) {
			return fmt.Sprintf("builtin:%s", baseName)
		}
	}

	// Handle Optional, Union with subscripts
	if strings.HasPrefix(typeName, "Optional[") || strings.HasPrefix(typeName, "Union[") {
		baseName := typeName[:strings.Index(typeName, "[")]
		return fmt.Sprintf("builtin:%s", baseName)
	}

	// Check for built-in types
	if isBuiltinType(typeName) {
		return fmt.Sprintf("builtin:%s", typeName)
	}

	// Check for module-qualified type (e.g., "module.ClassName")
	if idx := strings.LastIndex(typeName, "."); idx > 0 {
		// External type reference
		return fmt.Sprintf("external:%s", typeName)
	}

	// Local type - create entity ID within current project
	instance := ast.BuildInstanceID(filePath, typeName, ast.TypeType)
	return entityid.Build(p.org, entityid.PlatformSemsource, "python", p.project, "type", instance)
}

// isBuiltinType returns true if the type is a Python built-in type.
func isBuiltinType(name string) bool {
	switch name {
	case "int", "float", "str", "bool", "bytes", "bytearray",
		"list", "List", "dict", "Dict", "set", "Set", "frozenset", "FrozenSet",
		"tuple", "Tuple", "type", "Type", "object", "None", "NoneType",
		"Any", "Optional", "Union", "Callable", "Awaitable", "Coroutine",
		"Iterator", "Iterable", "Generator", "AsyncIterator", "AsyncIterable", "AsyncGenerator",
		"Sequence", "Mapping", "MutableMapping", "MutableSequence", "MutableSet",
		"TypeVar", "Generic", "Protocol", "Final", "Literal", "ClassVar",
		"IO", "TextIO", "BinaryIO", "Pattern", "Match",
		"ContextManager", "AsyncContextManager":
		return true
	}
	return false
}

// isAllCaps returns true if the string is all uppercase (constant naming convention).
func isAllCaps(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}
