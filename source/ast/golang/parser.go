// Package golang provides Go AST parsing and code entity extraction.
package golang

import (
	"context"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
)

func init() {
	ast.DefaultRegistry.Register("go", []string{".go"},
		func(org, project, repoRoot string) ast.FileParser {
			return NewParser(org, project, repoRoot)
		})
}

// Parser extracts code entities from Go source files
type Parser struct {
	// org is the organization prefix for entity IDs
	org string

	// project is the project name for entity IDs
	project string

	// repoRoot is the root directory of the repository
	repoRoot string

	// importMap holds the current file's imports, keyed by local name (alias or last path segment)
	importMap map[string]string
}

// NewParser creates a new Go AST parser
func NewParser(org, project, repoRoot string) *Parser {
	return &Parser{
		org:      org,
		project:  project,
		repoRoot: repoRoot,
	}
}

// ParseFile parses a single Go file and extracts code entities
func (p *Parser) ParseFile(ctx context.Context, filePath string) (*ast.ParseResult, error) {
	// Check for cancellation before starting potentially expensive file I/O.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

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

	// Parse file
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse file: %w", err)
	}

	result := &ast.ParseResult{
		Path:     relPath,
		Hash:     hash,
		Package:  file.Name.Name,
		Imports:  make([]string, 0),
		Entities: make([]*ast.CodeEntity, 0),
	}

	// Build import map for type resolution
	p.importMap = make(map[string]string)
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		result.Imports = append(result.Imports, importPath)

		// Determine local name: use alias if provided, otherwise last path segment
		var localName string
		if imp.Name != nil && imp.Name.Name != "." && imp.Name.Name != "_" {
			localName = imp.Name.Name
		} else {
			// Extract last segment of import path
			parts := strings.Split(importPath, "/")
			localName = parts[len(parts)-1]
		}
		p.importMap[localName] = importPath
	}

	// Create file entity
	fileEntity := ast.NewCodeEntity(p.org, "golang", p.project, ast.TypeFile, filepath.Base(filePath), relPath)
	fileEntity.Package = file.Name.Name
	fileEntity.Hash = hash
	fileEntity.Imports = result.Imports
	fileEntity.StartLine = 1
	fileEntity.EndLine = fset.Position(file.End()).Line

	// Extract doc comment if present
	if file.Doc != nil {
		fileEntity.DocComment = file.Doc.Text()
	}

	result.FileEntity = fileEntity
	result.Entities = append(result.Entities, fileEntity)

	// Extract declarations
	childIDs := make([]string, 0)
	for _, decl := range file.Decls {
		entities := p.extractDeclaration(fset, decl, fileEntity.ID, relPath)
		for _, entity := range entities {
			entity.ContainedBy = fileEntity.ID
			result.Entities = append(result.Entities, entity)
			childIDs = append(childIDs, entity.ID)
		}
	}
	fileEntity.Contains = childIDs

	return result, nil
}

// ParseDirectory parses all Go files in a directory
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

		// Skip non-Go files and test files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip vendor and hidden directories
		relPath, _ := filepath.Rel(p.repoRoot, path)
		if strings.Contains(relPath, "vendor/") || strings.HasPrefix(filepath.Base(relPath), ".") {
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

// extractDeclaration extracts entities from a declaration
func (p *Parser) extractDeclaration(fset *token.FileSet, decl goast.Decl, _ /* parentID */, filePath string) []*ast.CodeEntity {
	var entities []*ast.CodeEntity

	switch d := decl.(type) {
	case *goast.FuncDecl:
		entity := p.extractFunction(fset, d, filePath)
		if entity != nil {
			entities = append(entities, entity)
		}

	case *goast.GenDecl:
		switch d.Tok {
		case token.TYPE:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*goast.TypeSpec); ok {
					entity := p.extractTypeSpec(fset, ts, d.Doc, filePath)
					if entity != nil {
						entities = append(entities, entity)
					}
				}
			}
		case token.CONST:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*goast.ValueSpec); ok {
					for _, name := range vs.Names {
						entity := p.extractValueSpec(fset, name, vs, d.Doc, ast.TypeConst, filePath)
						if entity != nil {
							entities = append(entities, entity)
						}
					}
				}
			}
		case token.VAR:
			for _, spec := range d.Specs {
				if vs, ok := spec.(*goast.ValueSpec); ok {
					for _, name := range vs.Names {
						entity := p.extractValueSpec(fset, name, vs, d.Doc, ast.TypeVar, filePath)
						if entity != nil {
							entities = append(entities, entity)
						}
					}
				}
			}
		}
	}

	return entities
}

// extractFunction extracts a function or method entity
func (p *Parser) extractFunction(fset *token.FileSet, fn *goast.FuncDecl, filePath string) *ast.CodeEntity {
	name := fn.Name.Name
	entityType := ast.TypeFunction

	// Check if it's a method (has receiver)
	var receiverType string
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		entityType = ast.TypeMethod
		receiverType = p.extractTypeName(fn.Recv.List[0].Type)
	}

	entity := ast.NewCodeEntity(p.org, "golang", p.project, entityType, name, filePath)
	entity.StartLine = fset.Position(fn.Pos()).Line
	entity.EndLine = fset.Position(fn.End()).Line

	if fn.Doc != nil {
		entity.DocComment = fn.Doc.Text()
	}

	// Extract receiver
	if receiverType != "" {
		entity.Receiver = p.typeNameToEntityID(receiverType, filePath)
	}

	// Extract parameter types - resolve to entity IDs
	// Note: Go AST groups parameters with same type, e.g., "a, b int" is one field with 2 names
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			typeName := p.extractTypeName(field.Type)
			if typeName != "" {
				typeID := p.typeNameToEntityID(typeName, filePath)
				// If field has no names (variadic or single anonymous), add once
				// If field has names, add once per name
				count := len(field.Names)
				if count == 0 {
					count = 1
				}
				for i := 0; i < count; i++ {
					entity.Parameters = append(entity.Parameters, typeID)
				}
			}
		}
	}

	// Extract return types - resolve to entity IDs
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			typeName := p.extractTypeName(field.Type)
			if typeName != "" {
				entity.Returns = append(entity.Returns, p.typeNameToEntityID(typeName, filePath))
			}
		}
	}

	// Extract function calls - resolve to entity IDs
	if fn.Body != nil {
		calls := p.extractFunctionCalls(fn.Body, filePath)
		entity.Calls = calls
	}

	return entity
}

// extractTypeSpec extracts a type (struct, interface, alias) entity
func (p *Parser) extractTypeSpec(fset *token.FileSet, ts *goast.TypeSpec, doc *goast.CommentGroup, filePath string) *ast.CodeEntity {
	name := ts.Name.Name
	var entityType ast.CodeEntityType

	switch t := ts.Type.(type) {
	case *goast.StructType:
		entityType = ast.TypeStruct
		entity := ast.NewCodeEntity(p.org, "golang", p.project, entityType, name, filePath)
		entity.StartLine = fset.Position(ts.Pos()).Line
		entity.EndLine = fset.Position(ts.End()).Line

		if doc != nil {
			entity.DocComment = doc.Text()
		}

		// Extract embedded types and field references
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				typeName := p.extractTypeName(field.Type)
				if len(field.Names) == 0 && typeName != "" {
					// Embedded field - resolve to entity ID
					entity.Embeds = append(entity.Embeds, p.typeNameToEntityID(typeName, filePath))
				} else if typeName != "" {
					// Regular field - track type reference as entity ID
					entity.References = append(entity.References, p.typeNameToEntityID(typeName, filePath))
				}
			}
		}

		return entity

	case *goast.InterfaceType:
		entityType = ast.TypeInterface
		entity := ast.NewCodeEntity(p.org, "golang", p.project, entityType, name, filePath)
		entity.StartLine = fset.Position(ts.Pos()).Line
		entity.EndLine = fset.Position(ts.End()).Line

		if doc != nil {
			entity.DocComment = doc.Text()
		}

		// Extract embedded interfaces
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if len(method.Names) == 0 {
					// Embedded interface - resolve to entity ID
					typeName := p.extractTypeName(method.Type)
					if typeName != "" {
						entity.Embeds = append(entity.Embeds, p.typeNameToEntityID(typeName, filePath))
					}
				}
			}
		}

		return entity

	default:
		// Type alias or other type definition
		entityType = ast.TypeType
		entity := ast.NewCodeEntity(p.org, "golang", p.project, entityType, name, filePath)
		entity.StartLine = fset.Position(ts.Pos()).Line
		entity.EndLine = fset.Position(ts.End()).Line

		if doc != nil {
			entity.DocComment = doc.Text()
		}

		// Track the underlying type as a reference - resolve to entity ID
		typeName := p.extractTypeName(ts.Type)
		if typeName != "" {
			entity.References = append(entity.References, p.typeNameToEntityID(typeName, filePath))
		}

		return entity
	}
}

// extractValueSpec extracts a const or var entity
func (p *Parser) extractValueSpec(fset *token.FileSet, name *goast.Ident, vs *goast.ValueSpec, doc *goast.CommentGroup, entityType ast.CodeEntityType, filePath string) *ast.CodeEntity {
	entity := ast.NewCodeEntity(p.org, "golang", p.project, entityType, name.Name, filePath)
	entity.StartLine = fset.Position(name.Pos()).Line
	entity.EndLine = fset.Position(vs.End()).Line

	if doc != nil {
		entity.DocComment = doc.Text()
	} else if vs.Doc != nil {
		entity.DocComment = vs.Doc.Text()
	}

	// Track type reference if present - resolve to entity ID
	if vs.Type != nil {
		typeName := p.extractTypeName(vs.Type)
		if typeName != "" {
			entity.References = append(entity.References, p.typeNameToEntityID(typeName, filePath))
		}
	}

	return entity
}

// extractTypeName extracts the name from a type expression
func (p *Parser) extractTypeName(expr goast.Expr) string {
	switch t := expr.(type) {
	case *goast.Ident:
		return t.Name
	case *goast.SelectorExpr:
		// Package-qualified type: pkg.Type
		if x, ok := t.X.(*goast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *goast.StarExpr:
		// Pointer type: *Type
		return p.extractTypeName(t.X)
	case *goast.ArrayType:
		// Array/slice type: []Type
		return p.extractTypeName(t.Elt)
	case *goast.MapType:
		// Map type: map[K]V
		return "map"
	case *goast.ChanType:
		// Channel type
		return "chan"
	case *goast.FuncType:
		// Function type
		return "func"
	case *goast.InterfaceType:
		// Anonymous interface
		return "interface"
	case *goast.StructType:
		// Anonymous struct
		return "struct"
	}
	return ""
}

// extractFunctionCalls extracts function call references from a block
func (p *Parser) extractFunctionCalls(block *goast.BlockStmt, filePath string) []string {
	var calls []string
	seen := make(map[string]bool)

	goast.Inspect(block, func(n goast.Node) bool {
		if call, ok := n.(*goast.CallExpr); ok {
			var name string
			switch fn := call.Fun.(type) {
			case *goast.Ident:
				name = fn.Name
			case *goast.SelectorExpr:
				if x, ok := fn.X.(*goast.Ident); ok {
					name = x.Name + "." + fn.Sel.Name
				} else {
					name = fn.Sel.Name
				}
			}
			if name != "" && !seen[name] {
				seen[name] = true
				// Resolve to entity ID for function calls
				callID := p.callNameToEntityID(name, filePath)
				calls = append(calls, callID)
			}
		}
		return true
	})

	return calls
}

// callNameToEntityID converts a function call name to an entity ID reference.
// For local functions (no package qualifier), creates an entity ID within the current project.
// For external functions (pkg.Func), resolves the import path and creates a reference ID.
func (p *Parser) callNameToEntityID(callName, filePath string) string {
	if callName == "" {
		return ""
	}

	// Check for package-qualified call (e.g., "pkg.Func")
	if idx := strings.Index(callName, "."); idx > 0 {
		pkgAlias := callName[:idx]
		funcPart := callName[idx+1:]

		// Look up the import path for this package alias
		if importPath, ok := p.importMap[pkgAlias]; ok {
			// External function: create reference ID with import path
			return fmt.Sprintf("external:%s.%s", importPath, funcPart)
		}

		// Not a package-qualified call - might be a method call on a local variable
		// Return as-is for now (e.g., "u.Name" where u is a local var)
		return callName
	}

	// Local function (no package qualifier)
	// Skip built-in functions that don't need resolution
	if isBuiltinFunc(callName) {
		return fmt.Sprintf("builtin:%s", callName)
	}

	// Local function: create entity ID within current project
	instance := ast.BuildInstanceID(filePath, callName, ast.TypeFunction)
	return entityid.Build(p.org, entityid.PlatformSemsource, "golang", p.project, "function", instance)
}

// isBuiltinFunc returns true if the function is a Go built-in function
func isBuiltinFunc(name string) bool {
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy",
		"delete", "imag", "len", "make", "max", "min", "new",
		"panic", "print", "println", "real", "recover":
		return true
	}
	return false
}

// typeNameToEntityID converts a type name to an entity ID reference.
// For local types (no package qualifier), it creates an entity ID within the current project.
// For external types (pkg.Type), it resolves the import path and creates a reference ID.
func (p *Parser) typeNameToEntityID(typeName, filePath string) string {
	if typeName == "" {
		return ""
	}

	// Check for package-qualified type (e.g., "pkg.Type")
	if idx := strings.Index(typeName, "."); idx > 0 {
		pkgAlias := typeName[:idx]
		typePart := typeName[idx+1:]

		// Look up the import path for this package alias
		if importPath, ok := p.importMap[pkgAlias]; ok {
			// External type: create reference ID with import path
			// Format: external:{import_path}.{type_name}
			return fmt.Sprintf("external:%s.%s", importPath, typePart)
		}

		// Package not found in imports - might be a method call on a local variable
		// Return as-is for now
		return typeName
	}

	// Local type (no package qualifier)
	// Skip built-in types that don't need resolution
	if isBuiltinType(typeName) {
		return fmt.Sprintf("builtin:%s", typeName)
	}

	// Local type: create entity ID within current project
	// Build instance ID from file path and type name
	instance := ast.BuildInstanceID(filePath, typeName, ast.TypeType)
	return entityid.Build(p.org, entityid.PlatformSemsource, "golang", p.project, "type", instance)
}

// isBuiltinType returns true if the type is a Go built-in type
func isBuiltinType(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128",
		"error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64",
		"rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"any", "comparable",
		"map", "chan", "func", "interface", "struct":
		return true
	}
	return false
}
