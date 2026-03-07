// Package svelte provides Svelte 5 AST parsing and code entity extraction using tree-sitter.
// It supports full runes awareness ($state, $props, $derived, $effect) and template component tracking.
package svelte

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/source/ast"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/svelte"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func init() {
	ast.DefaultRegistry.Register("svelte", []string{".svelte"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from Svelte source files using tree-sitter
type Parser struct {
	org      string
	project  string
	repoRoot string
}

// NewParser creates a new Svelte parser
func NewParser(org, project, repoRoot string) *Parser {
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
	}
}

// ParseFile parses a single Svelte file and extracts code entities
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*ast.ParseResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

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

	// Parse with tree-sitter Svelte grammar
	parser := sitter.NewParser()
	parser.SetLanguage(svelte.GetLanguage())

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse svelte: %w", err)
	}
	defer tree.Close()

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

	// Parse the Svelte tree to extract entities
	rootNode := tree.RootNode()
	p.populateEntities(ctx, parseResult, rootNode, content, filePath, relPath, hash)

	return parseResult, nil
}

// populateEntities builds all code entities from the parsed Svelte tree and populates parseResult.
func (p *Parser) populateEntities(ctx context.Context, parseResult *ast.ParseResult, rootNode *sitter.Node, content []byte, filePath, relPath, hash string) {
	// Extract script content and detect language FIRST (needed for all entities)
	scriptContent, scriptLang := p.extractScriptContent(rootNode, content)
	if scriptLang == "" {
		scriptLang = "typescript" // default for Svelte 5 projects
	}

	// Create file entity with actual script language + svelte framework
	fileEntity := ast.NewCodeEntity(p.org, p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Hash = hash
	fileEntity.Language = scriptLang
	fileEntity.Framework = "svelte"
	fileEntity.StartLine = 1
	fileEntity.EndLine = countLines(content)

	parseResult.FileEntity = fileEntity
	parseResult.Entities = append(parseResult.Entities, fileEntity)

	// Extract component entity from the file
	componentName := extractComponentName(filePath)
	componentEntity := p.createComponentEntity(componentName, relPath, scriptLang)
	componentEntity.ContainedBy = fileEntity.ID
	componentEntity.StartLine = 1
	componentEntity.EndLine = countLines(content)

	// Parse script content if present
	if scriptContent != nil {
		// Parse script block with TypeScript parser
		scriptEntities, imports := p.parseScriptBlock(ctx, scriptContent, scriptLang, relPath, componentEntity.ID)

		// Extract runes from script
		runeInfo := extractRunes(scriptContent)
		componentEntity.DocComment = runeInfo.ToDocComment()

		// Add imports to file entity
		fileEntity.Imports = imports
		parseResult.Imports = imports

		// Add script entities with svelte framework marker
		for _, entity := range scriptEntities {
			entity.Framework = "svelte"
			parseResult.Entities = append(parseResult.Entities, entity)
			componentEntity.Contains = append(componentEntity.Contains, entity.ID)
		}
	}

	// Extract template component usage
	templateUsage := p.extractTemplateUsage(rootNode, content)
	for _, componentRef := range templateUsage {
		refID := p.componentNameToEntityID(componentRef, relPath)
		componentEntity.References = append(componentEntity.References, refID)
	}

	parseResult.Entities = append(parseResult.Entities, componentEntity)
	fileEntity.Contains = append(fileEntity.Contains, componentEntity.ID)
}

// ParseDirectory parses all Svelte files in a directory
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
			if base == "node_modules" || base == "dist" || base == ".svelte-kit" ||
				base == "build" || base == "coverage" || base == ".turbo" ||
				base == ".next" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".svelte") {
			return nil
		}

		result, err := p.ParseFile(ctx, path)
		if err != nil {
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

// createComponentEntity creates a component entity for a Svelte file
func (p *Parser) createComponentEntity(name, path, lang string) *ast.CodeEntity {
	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeComponent, name, path)
	entity.Language = lang                   // typescript or javascript
	entity.Framework = "svelte"              // Svelte is the framework
	entity.Visibility = ast.VisibilityPublic // Svelte components are typically public
	return entity
}

// extractScriptContent extracts the <script> tag content from a Svelte file
func (p *Parser) extractScriptContent(root *sitter.Node, source []byte) ([]byte, string) {
	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	return p.findScriptContent(cursor, source)
}

// findScriptContent recursively searches for script_element nodes
func (p *Parser) findScriptContent(cursor *sitter.TreeCursor, source []byte) ([]byte, string) {
	node := cursor.CurrentNode()

	if node.Type() == "script_element" {
		// Find the raw_text child which contains the script content
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "raw_text" {
				content := child.Content(source)
				// Determine language from start_tag attributes
				lang := p.extractScriptLang(node, source)
				return []byte(content), lang
			}
		}
	}

	if cursor.GoToFirstChild() {
		for {
			content, lang := p.findScriptContent(cursor, source)
			if content != nil {
				return content, lang
			}
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}

	return nil, ""
}

// extractScriptLang extracts the lang attribute from a script tag
func (p *Parser) extractScriptLang(scriptNode *sitter.Node, source []byte) string {
	for i := 0; i < int(scriptNode.ChildCount()); i++ {
		child := scriptNode.Child(i)
		if child.Type() == "start_tag" {
			// Look for lang attribute
			for j := 0; j < int(child.ChildCount()); j++ {
				attr := child.Child(j)
				if attr.Type() == "attribute" {
					attrText := attr.Content(source)
					if strings.Contains(attrText, "lang=") {
						if strings.Contains(attrText, "ts") || strings.Contains(attrText, "typescript") {
							return "typescript"
						}
					}
				}
			}
		}
	}
	return "javascript"
}

// parseScriptBlock parses the script content as TypeScript/JavaScript
func (p *Parser) parseScriptBlock(ctx context.Context, scriptContent []byte, lang, filePath, parentID string) ([]*ast.CodeEntity, []string) {
	var entities []*ast.CodeEntity
	var imports []string

	// Parse with TypeScript grammar
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	tree, err := parser.ParseCtx(ctx, nil, scriptContent)
	if err != nil {
		return entities, imports
	}
	defer tree.Close()

	// Check context after parsing
	select {
	case <-ctx.Done():
		return entities, imports
	default:
	}

	rootNode := tree.RootNode()

	// Extract imports
	imports = p.extractImports(rootNode, scriptContent)

	// Extract functions, classes, etc.
	entities = p.extractScriptEntities(rootNode, scriptContent, filePath, lang, parentID)

	return entities, imports
}

// extractImports extracts import statements from TypeScript/JavaScript
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

// extractScriptEntities extracts entities from script content
func (p *Parser) extractScriptEntities(node *sitter.Node, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	cursor := sitter.NewTreeCursor(node)
	defer cursor.Close()

	entities = append(entities, p.walkScriptNode(cursor, source, filePath, lang, parentID)...)

	return entities
}

// walkScriptNode recursively walks the AST and extracts entities
func (p *Parser) walkScriptNode(cursor *sitter.TreeCursor, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	node := cursor.CurrentNode()
	nodeType := node.Type()

	switch nodeType {
	case "function_declaration":
		entity := p.extractFunction(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "interface_declaration":
		entity := p.extractInterface(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "type_alias_declaration":
		entity := p.extractTypeAlias(node, source, filePath, lang, parentID)
		if entity != nil {
			entities = append(entities, entity)
		}

	case "lexical_declaration", "variable_declaration":
		varEntities := p.extractVariableDeclaration(node, source, filePath, lang, parentID)
		entities = append(entities, varEntities...)
	}

	if cursor.GoToFirstChild() {
		for {
			entities = append(entities, p.walkScriptNode(cursor, source, filePath, lang, parentID)...)
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}

	return entities
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
	entity.Visibility = ast.VisibilityPrivate // Functions in script are component-private

	// Check for async
	if p.hasModifier(node, "async") {
		entity.DocComment = "Async function"
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

	entity := ast.NewCodeEntity(p.org, p.project, ast.TypeInterface, name, filePath)
	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = ast.VisibilityPrivate

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
	entity.Visibility = ast.VisibilityPrivate

	return entity
}

// extractVariableDeclaration extracts const/let/var declarations
func (p *Parser) extractVariableDeclaration(node *sitter.Node, source []byte, filePath, lang, parentID string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	kind := "const"
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "const":
			kind = "const"
		case "let":
			kind = "let"
		case "var":
			kind = "var"
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "variable_declarator" {
			entity := p.extractVariableDeclarator(child, source, filePath, lang, parentID, kind)
			if entity != nil {
				entities = append(entities, entity)
			}
		}
	}

	return entities
}

// extractVariableDeclarator extracts a single variable declarator
func (p *Parser) extractVariableDeclarator(node *sitter.Node, source []byte, filePath, lang, parentID, kind string) *ast.CodeEntity {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	name := nodeText(nameNode, source)
	valueNode := node.ChildByFieldName("value")

	isArrowFunc := false
	if valueNode != nil && valueNode.Type() == "arrow_function" {
		isArrowFunc = true
	}

	lineNum := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	var entity *ast.CodeEntity
	if isArrowFunc {
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeFunction, name, filePath)
		entity.DocComment = "Arrow function"
	} else if kind == "const" {
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeConst, name, filePath)
	} else {
		entity = ast.NewCodeEntity(p.org, p.project, ast.TypeVar, name, filePath)
	}

	entity.Language = lang
	entity.ContainedBy = parentID
	entity.StartLine = lineNum
	entity.EndLine = endLine
	entity.Visibility = ast.VisibilityPrivate

	return entity
}

// extractTemplateUsage extracts component usage from the template section
func (p *Parser) extractTemplateUsage(root *sitter.Node, source []byte) []string {
	var components []string
	seen := make(map[string]bool)

	cursor := sitter.NewTreeCursor(root)
	defer cursor.Close()

	p.walkTemplateForComponents(cursor, source, &components, seen)

	return components
}

// walkTemplateForComponents finds component usage in template
func (p *Parser) walkTemplateForComponents(cursor *sitter.TreeCursor, source []byte, components *[]string, seen map[string]bool) {
	node := cursor.CurrentNode()

	// Look for element nodes that start with uppercase (component convention)
	if node.Type() == "element" || node.Type() == "self_closing_element" {
		// Find the tag name
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "start_tag" || child.Type() == "self_closing_tag" {
				tagName := p.extractTagName(child, source)
				if tagName != "" && isComponentName(tagName) && !seen[tagName] {
					seen[tagName] = true
					*components = append(*components, tagName)
				}
			}
		}
	}

	if cursor.GoToFirstChild() {
		for {
			p.walkTemplateForComponents(cursor, source, components, seen)
			if !cursor.GoToNextSibling() {
				break
			}
		}
		cursor.GoToParent()
	}
}

// extractTagName extracts the tag name from a start_tag or self_closing_tag
func (p *Parser) extractTagName(tagNode *sitter.Node, source []byte) string {
	for i := 0; i < int(tagNode.ChildCount()); i++ {
		child := tagNode.Child(i)
		if child.Type() == "tag_name" {
			return child.Content(source)
		}
	}
	return ""
}

// componentNameToEntityID converts a component name to an entity ID reference
func (p *Parser) componentNameToEntityID(componentName, filePath string) string {
	if componentName == "" {
		return ""
	}

	// Create entity ID within current project
	instance := ast.BuildInstanceID(filePath, componentName, ast.TypeComponent)
	return fmt.Sprintf("%s.semspec.code.component.%s.%s", p.org, p.project, instance)
}

// hasModifier checks if a node has a specific modifier
func (p *Parser) hasModifier(node *sitter.Node, modifier string) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == modifier {
			return true
		}
	}
	return false
}

// isComponentName returns true if the name follows component naming (PascalCase)
func isComponentName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Components start with uppercase letter
	first := name[0]
	return first >= 'A' && first <= 'Z'
}

// extractComponentName extracts a component name from the file path
func extractComponentName(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, ".svelte")
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
