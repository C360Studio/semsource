// Package java provides Java AST parsing and code entity extraction using tree-sitter.
package java

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/c360studio/semsource/source/ast"
)

func init() {
	ast.DefaultRegistry.Register("java", []string{".java"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from Java source files using tree-sitter.
type Parser struct {
	org      string
	project  string
	repoRoot string
	parser   *sitter.Parser
}

// NewParser creates a new Java AST parser.
func NewParser(org, project, repoRoot string) *Parser {
	p := sitter.NewParser()
	p.SetLanguage(java.GetLanguage())
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
		parser:   p,
	}
}

// ParseFile parses a single Java file and extracts code entities.
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

	// Extract package name
	packageName := p.extractPackageName(rootNode, content)

	result := &ast.ParseResult{
		Path:     relPath,
		Hash:     hash,
		Package:  packageName,
		Imports:  make([]string, 0),
		Entities: make([]*ast.CodeEntity, 0),
	}

	// Create file entity
	fileEntity := ast.NewCodeEntity(p.org, p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Package = packageName
	fileEntity.Hash = hash
	fileEntity.Language = "java"
	fileEntity.StartLine = 1
	fileEntity.EndLine = int(rootNode.EndPoint().Row) + 1

	result.FileEntity = fileEntity
	result.Entities = append(result.Entities, fileEntity)

	// Extract entities and imports from the AST
	childIDs := make([]string, 0)
	for i := 0; i < int(rootNode.NamedChildCount()); i++ {
		child := rootNode.NamedChild(i)

		// Extract imports
		if imports := p.extractImports(child, content); len(imports) > 0 {
			result.Imports = append(result.Imports, imports...)
			fileEntity.Imports = append(fileEntity.Imports, imports...)
		}

		// Extract top-level entities
		entities := p.extractTopLevelNode(child, content, fileEntity.ID, relPath)
		for i, entity := range entities {
			// Only set ContainedBy for the first entity (top-level)
			// Nested entities already have their ContainedBy set correctly
			if i == 0 {
				entity.ContainedBy = fileEntity.ID
				childIDs = append(childIDs, entity.ID)
			}
			result.Entities = append(result.Entities, entity)
		}
	}
	fileEntity.Contains = childIDs

	return result, nil
}

// ParseDirectory parses all Java files in a directory.
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

		// Skip directories and non-Java files
		if info.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}

		// Skip test files and build directories
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
		// Skip common Java build and dependency directories
		switch part {
		case "target", "build", "bin", "out", "classes",
			"node_modules", "vendor", ".gradle", ".mvn",
			"test-output", ".idea", ".settings":
			return true
		}
	}
	return false
}

// extractPackageName extracts the package name from the Java file.
func (p *Parser) extractPackageName(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "package_declaration" {
			// Find the scoped_identifier or identifier node
			for j := 0; j < int(child.NamedChildCount()); j++ {
				pkgNode := child.NamedChild(j)
				if pkgNode.Type() == "scoped_identifier" || pkgNode.Type() == "identifier" {
					return string(content[pkgNode.StartByte():pkgNode.EndByte()])
				}
			}
		}
	}
	return ""
}

// extractImports extracts import statements from the AST.
func (p *Parser) extractImports(node *sitter.Node, content []byte) []string {
	if node.Type() != "import_declaration" {
		return nil
	}

	var imports []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "scoped_identifier" || child.Type() == "identifier" {
			importPath := string(content[child.StartByte():child.EndByte()])
			// Remove asterisk imports
			importPath = strings.TrimSuffix(importPath, ".*")
			imports = append(imports, importPath)
		}
	}
	return imports
}

// extractTopLevelNode extracts entities from a top-level AST node.
// Returns all entities including nested ones.
func (p *Parser) extractTopLevelNode(node *sitter.Node, content []byte, _ /* parentID */, filePath string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	switch node.Type() {
	case "class_declaration":
		classEntities := p.extractClass(node, content, filePath)
		entities = append(entities, classEntities...)

	case "interface_declaration":
		interfaceEntities := p.extractInterface(node, content, filePath)
		entities = append(entities, interfaceEntities...)

	case "enum_declaration":
		entity := p.extractEnum(node, content, filePath)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "record_declaration":
		entity := p.extractRecord(node, content, filePath)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "field_declaration":
		entities = append(entities, p.extractFieldDeclaration(node, content, filePath)...)

	case "method_declaration":
		entity := p.extractMethod(node, content, filePath, "")
		if entity != nil {
			entities = append(entities, entity)
		}
	}

	return entities
}

// extractClass extracts a class entity and all its members.
// Returns all entities including the class itself and nested entities.
func (p *Parser) extractClass(node *sitter.Node, content []byte, filePath string) []*ast.CodeEntity {
	// Get class name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeClass, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)

	// Extract annotations (store in DocComment metadata)
	annotations := p.extractAnnotations(node, content)
	if len(annotations) > 0 {
		entity.DocComment = strings.Join(annotations, "\n")
	}

	// Extract superclass (extends)
	if superclass := node.ChildByFieldName("superclass"); superclass != nil {
		superName := p.extractTypeReference(superclass, content)
		if superName != "" {
			entity.Extends = append(entity.Extends, p.typeNameToEntityID(superName, filePath))
		}
	}

	// Extract interfaces (implements)
	if interfaces := node.ChildByFieldName("interfaces"); interfaces != nil {
		for i := 0; i < int(interfaces.NamedChildCount()); i++ {
			ifaceNode := interfaces.NamedChild(i)
			ifaceName := p.extractTypeReference(ifaceNode, content)
			if ifaceName != "" {
				entity.Implements = append(entity.Implements, p.typeNameToEntityID(ifaceName, filePath))
			}
		}
	}

	// Extract body members (methods, fields, inner classes)
	// Returns all child entities
	allEntities := []*ast.CodeEntity{entity}
	if body := node.ChildByFieldName("body"); body != nil {
		childEntities := p.extractClassBody(body, content, entity, filePath)
		allEntities = append(allEntities, childEntities...)
	}

	return allEntities
}

// extractInterface extracts an interface entity and all its members.
// Returns all entities including the interface itself and nested entities.
func (p *Parser) extractInterface(node *sitter.Node, content []byte, filePath string) []*ast.CodeEntity {
	// Get interface name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeInterface, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)

	// Extract annotations
	annotations := p.extractAnnotations(node, content)
	if len(annotations) > 0 {
		entity.DocComment = strings.Join(annotations, "\n")
	}

	// Extract extended interfaces
	if extends := node.ChildByFieldName("extends"); extends != nil {
		for i := 0; i < int(extends.NamedChildCount()); i++ {
			extNode := extends.NamedChild(i)
			extName := p.extractTypeReference(extNode, content)
			if extName != "" {
				entity.Extends = append(entity.Extends, p.typeNameToEntityID(extName, filePath))
			}
		}
	}

	// Extract body (method signatures)
	allEntities := []*ast.CodeEntity{entity}
	if body := node.ChildByFieldName("body"); body != nil {
		childIDs := make([]string, 0)
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			if child.Type() == "method_declaration" {
				method := p.extractMethod(child, content, filePath, entity.ID)
				if method != nil {
					method.ContainedBy = entity.ID
					childIDs = append(childIDs, method.ID)
					allEntities = append(allEntities, method)
				}
			}
		}
		entity.Contains = childIDs
	}

	return allEntities
}

// extractEnum extracts an enum entity.
func (p *Parser) extractEnum(node *sitter.Node, content []byte, filePath string) *ast.CodeEntity {
	// Get enum name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeEnum, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)

	// Extract annotations
	annotations := p.extractAnnotations(node, content)
	if len(annotations) > 0 {
		entity.DocComment = strings.Join(annotations, "\n")
	}

	// Extract implemented interfaces
	if interfaces := node.ChildByFieldName("interfaces"); interfaces != nil {
		for i := 0; i < int(interfaces.NamedChildCount()); i++ {
			ifaceNode := interfaces.NamedChild(i)
			ifaceName := p.extractTypeReference(ifaceNode, content)
			if ifaceName != "" {
				entity.Implements = append(entity.Implements, p.typeNameToEntityID(ifaceName, filePath))
			}
		}
	}

	return entity
}

// extractRecord extracts a record entity (Java 14+).
func (p *Parser) extractRecord(node *sitter.Node, content []byte, filePath string) *ast.CodeEntity {
	// Get record name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeStruct, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)

	// Extract annotations
	annotations := p.extractAnnotations(node, content)
	if len(annotations) > 0 {
		entity.DocComment = strings.Join(annotations, "\n")
	}

	// Extract implemented interfaces
	if interfaces := node.ChildByFieldName("interfaces"); interfaces != nil {
		for i := 0; i < int(interfaces.NamedChildCount()); i++ {
			ifaceNode := interfaces.NamedChild(i)
			ifaceName := p.extractTypeReference(ifaceNode, content)
			if ifaceName != "" {
				entity.Implements = append(entity.Implements, p.typeNameToEntityID(ifaceName, filePath))
			}
		}
	}

	return entity
}

// extractMethod extracts a method entity.
func (p *Parser) extractMethod(node *sitter.Node, content []byte, filePath string, receiverID string) *ast.CodeEntity {
	// Get method name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeMethod, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)

	if receiverID != "" {
		entity.Receiver = receiverID
	}

	// Extract annotations and modifiers
	annotations := p.extractAnnotations(node, content)
	modifiers := p.extractModifiers(node, content)
	docParts := make([]string, 0)
	if len(annotations) > 0 {
		docParts = append(docParts, strings.Join(annotations, "\n"))
	}
	if len(modifiers) > 0 {
		docParts = append(docParts, strings.Join(modifiers, " "))
	}
	if len(docParts) > 0 {
		entity.DocComment = strings.Join(docParts, "\n")
	}

	// Extract parameters
	if params := node.ChildByFieldName("parameters"); params != nil {
		entity.Parameters = p.extractParameters(params, content, filePath)
	}

	// Extract return type
	if returnType := node.ChildByFieldName("type"); returnType != nil {
		typeName := p.extractTypeReference(returnType, content)
		if typeName != "" && typeName != "void" {
			entity.Returns = append(entity.Returns, p.typeNameToEntityID(typeName, filePath))
		}
	}

	return entity
}

// extractFieldDeclaration extracts field entities from a field declaration.
func (p *Parser) extractFieldDeclaration(node *sitter.Node, content []byte, filePath string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	// Get field type
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}
	typeName := p.extractTypeReference(typeNode, content)

	// Extract visibility and modifiers
	visibility := p.extractVisibility(node, content)
	modifiers := p.extractModifiers(node, content)

	// A field declaration can have multiple declarators
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := string(content[nameNode.StartByte():nameNode.EndByte()])

			entity := ast.NewCodeEntity(p.org, p.project, ast.TypeVar, name, filePath)
			entity.Language = "java"
			entity.StartLine = int(node.StartPoint().Row) + 1
			entity.EndLine = int(node.EndPoint().Row) + 1
			entity.Visibility = visibility

			if typeName != "" {
				entity.References = append(entity.References, p.typeNameToEntityID(typeName, filePath))
			}

			if len(modifiers) > 0 {
				entity.DocComment = strings.Join(modifiers, " ")
			}

			entities = append(entities, entity)
		}
	}

	return entities
}

// extractClassBody extracts members from a class body.
// Returns all child entities and updates classEntity.Contains.
func (p *Parser) extractClassBody(body *sitter.Node, content []byte, classEntity *ast.CodeEntity, filePath string) []*ast.CodeEntity {
	childIDs := make([]string, 0)
	allChildEntities := make([]*ast.CodeEntity, 0)

	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)

		switch child.Type() {
		case "method_declaration":
			method := p.extractMethod(child, content, filePath, classEntity.ID)
			if method != nil {
				method.ContainedBy = classEntity.ID
				childIDs = append(childIDs, method.ID)
				allChildEntities = append(allChildEntities, method)
			}

		case "constructor_declaration":
			ctor := p.extractConstructor(child, content, filePath, classEntity.ID)
			if ctor != nil {
				ctor.ContainedBy = classEntity.ID
				childIDs = append(childIDs, ctor.ID)
				allChildEntities = append(allChildEntities, ctor)
			}

		case "field_declaration":
			fields := p.extractFieldDeclaration(child, content, filePath)
			for _, field := range fields {
				field.ContainedBy = classEntity.ID
				childIDs = append(childIDs, field.ID)
				allChildEntities = append(allChildEntities, field)
			}

		case "class_declaration":
			innerClassEntities := p.extractClass(child, content, filePath)
			if len(innerClassEntities) > 0 {
				// First entity is the class itself
				innerClassEntities[0].ContainedBy = classEntity.ID
				childIDs = append(childIDs, innerClassEntities[0].ID)
				allChildEntities = append(allChildEntities, innerClassEntities...)
			}

		case "interface_declaration":
			innerInterfaceEntities := p.extractInterface(child, content, filePath)
			if len(innerInterfaceEntities) > 0 {
				innerInterfaceEntities[0].ContainedBy = classEntity.ID
				childIDs = append(childIDs, innerInterfaceEntities[0].ID)
				allChildEntities = append(allChildEntities, innerInterfaceEntities...)
			}

		case "enum_declaration":
			innerEnum := p.extractEnum(child, content, filePath)
			if innerEnum != nil {
				innerEnum.ContainedBy = classEntity.ID
				childIDs = append(childIDs, innerEnum.ID)
				allChildEntities = append(allChildEntities, innerEnum)
			}
		}
	}

	classEntity.Contains = childIDs
	return allChildEntities
}

// extractConstructor extracts a constructor entity.
func (p *Parser) extractConstructor(node *sitter.Node, content []byte, filePath string, receiverID string) *ast.CodeEntity {
	// Get constructor name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeMethod, name, filePath)
	entity.Language = "java"
	entity.StartLine = int(node.StartPoint().Row) + 1
	entity.EndLine = int(node.EndPoint().Row) + 1
	entity.Visibility = p.extractVisibility(node, content)
	entity.Receiver = receiverID

	// Extract annotations and modifiers
	annotations := p.extractAnnotations(node, content)
	modifiers := p.extractModifiers(node, content)
	docParts := make([]string, 0)
	if len(annotations) > 0 {
		docParts = append(docParts, strings.Join(annotations, "\n"))
	}
	if len(modifiers) > 0 {
		docParts = append(docParts, strings.Join(modifiers, " "))
	}
	if len(docParts) > 0 {
		entity.DocComment = strings.Join(docParts, "\n")
	}

	// Extract parameters
	if params := node.ChildByFieldName("parameters"); params != nil {
		entity.Parameters = p.extractParameters(params, content, filePath)
	}

	return entity
}

// extractParameters extracts parameter type entity IDs.
func (p *Parser) extractParameters(params *sitter.Node, content []byte, filePath string) []string {
	var paramTypes []string

	for i := 0; i < int(params.NamedChildCount()); i++ {
		child := params.NamedChild(i)
		if child.Type() == "formal_parameter" || child.Type() == "spread_parameter" {
			if typeNode := child.ChildByFieldName("type"); typeNode != nil {
				typeName := p.extractTypeReference(typeNode, content)
				if typeName != "" {
					paramTypes = append(paramTypes, p.typeNameToEntityID(typeName, filePath))
				}
			}
		}
	}

	return paramTypes
}

// extractVisibility extracts visibility from modifiers.
func (p *Parser) extractVisibility(node *sitter.Node, content []byte) ast.Visibility {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			modText := string(content[child.StartByte():child.EndByte()])
			if strings.Contains(modText, "public") {
				return ast.VisibilityPublic
			} else if strings.Contains(modText, "private") {
				return ast.VisibilityPrivate
			} else if strings.Contains(modText, "protected") {
				return ast.VisibilityPrivate // Protected is treated as private for simplicity
			}
		}
	}
	// Default to package-private (treated as private)
	return ast.VisibilityPrivate
}

// extractModifiers extracts modifier keywords (static, final, abstract, etc.).
func (p *Parser) extractModifiers(node *sitter.Node, content []byte) []string {
	var modifiers []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				modText := string(content[mod.StartByte():mod.EndByte()])
				modText = strings.TrimSpace(modText)
				// Filter out access modifiers (already captured in Visibility) and empty strings
				if modText != "" && modText != "public" && modText != "private" && modText != "protected" {
					modifiers = append(modifiers, modText)
				}
			}
		}
	}

	return modifiers
}

// extractAnnotations extracts annotation strings.
func (p *Parser) extractAnnotations(node *sitter.Node, content []byte) []string {
	var annotations []string

	// Check all children, not just named children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()
		if childType == "marker_annotation" || childType == "annotation" {
			annText := strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
			annotations = append(annotations, annText)
		}
		// Also check inside modifiers node
		if childType == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				modChild := child.Child(j)
				if modChild.Type() == "marker_annotation" || modChild.Type() == "annotation" {
					annText := strings.TrimSpace(string(content[modChild.StartByte():modChild.EndByte()]))
					annotations = append(annotations, annText)
				}
			}
		}
	}

	return annotations
}

// extractTypeReference extracts a type name from a type node, stripping generics.
func (p *Parser) extractTypeReference(typeNode *sitter.Node, content []byte) string {
	if typeNode == nil {
		return ""
	}

	switch typeNode.Type() {
	case "type_identifier":
		return string(content[typeNode.StartByte():typeNode.EndByte()])

	case "scoped_type_identifier":
		return string(content[typeNode.StartByte():typeNode.EndByte()])

	case "generic_type":
		// Extract base type, ignoring type parameters
		// Try both "type" and the first named child
		if baseType := typeNode.ChildByFieldName("type"); baseType != nil {
			return p.extractTypeReference(baseType, content)
		}
		// Fallback: get first named child
		if typeNode.NamedChildCount() > 0 {
			return p.extractTypeReference(typeNode.NamedChild(0), content)
		}

	case "array_type":
		// Extract element type
		if elemType := typeNode.ChildByFieldName("element"); elemType != nil {
			return p.extractTypeReference(elemType, content)
		}

	case "void_type":
		return "void"

	default:
		// For primitive types and others, use the raw text
		return string(content[typeNode.StartByte():typeNode.EndByte()])
	}

	return ""
}

// typeNameToEntityID converts a Java type name to an entity ID reference.
func (p *Parser) typeNameToEntityID(typeName, filePath string) string {
	if typeName == "" {
		return ""
	}

	// Clean up the type name
	typeName = strings.TrimSpace(typeName)

	// Check for built-in types
	if isBuiltinType(typeName) {
		return fmt.Sprintf("builtin:%s", typeName)
	}

	// Check for fully qualified type (e.g., "java.util.List")
	if strings.Contains(typeName, ".") {
		// External type reference
		return fmt.Sprintf("external:%s", typeName)
	}

	// Local type - create entity ID within current project
	instance := ast.BuildInstanceID(filePath, typeName, ast.TypeType)
	return fmt.Sprintf("%s.semspec.code.type.%s.%s", p.org, p.project, instance)
}

// isBuiltinType returns true if the type is a Java built-in type.
func isBuiltinType(name string) bool {
	switch name {
	case "boolean", "byte", "char", "short", "int", "long", "float", "double",
		"void", "Boolean", "Byte", "Character", "Short", "Integer", "Long", "Float", "Double",
		"String", "Object", "Class", "Enum", "Void",
		"Number", "CharSequence", "Comparable", "Cloneable", "Iterable",
		"Throwable", "Exception", "RuntimeException", "Error":
		return true
	}
	return false
}
