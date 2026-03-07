package golang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func TestNewParser(t *testing.T) {
	p := NewParser("acme", "myproject", "/tmp/repo")
	if p.org != "acme" {
		t.Errorf("org = %q, want %q", p.org, "acme")
	}
	if p.project != "myproject" {
		t.Errorf("project = %q, want %q", p.project, "myproject")
	}
	if p.repoRoot != "/tmp/repo" {
		t.Errorf("repoRoot = %q, want %q", p.repoRoot, "/tmp/repo")
	}
}

func TestParseFile_SimpleFunction(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple Go file
	code := `package example

// Add adds two integers and returns the sum.
func Add(a, b int) int {
	return a + b
}
`
	filePath := filepath.Join(tmpDir, "math.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Check file entity
	if result.FileEntity == nil {
		t.Fatal("FileEntity is nil")
	}
	if result.Package != "example" {
		t.Errorf("Package = %q, want %q", result.Package, "example")
	}
	if result.Hash == "" {
		t.Error("Hash is empty")
	}

	// Check we found the function
	var addFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction && e.Name == "Add" {
			addFunc = e
			break
		}
	}
	if addFunc == nil {
		t.Fatal("Add function not found")
	}

	// Check function properties
	if addFunc.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", addFunc.Visibility, ast.VisibilityPublic)
	}
	if addFunc.DocComment == "" || !strings.Contains(addFunc.DocComment, "adds two integers") {
		t.Errorf("DocComment = %q, want to contain 'adds two integers'", addFunc.DocComment)
	}
	if len(addFunc.Parameters) != 2 {
		t.Errorf("Parameters count = %d, want 2", len(addFunc.Parameters))
	}
	if len(addFunc.Returns) != 1 {
		t.Errorf("Returns count = %d, want 1", len(addFunc.Returns))
	}

	// Check entity ID format
	if !strings.HasPrefix(addFunc.ID, "acme.semspec.code.function.test.") {
		t.Errorf("ID = %q, want prefix 'acme.semspec.code.function.test.'", addFunc.ID)
	}
}

func TestParseFile_Struct(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

// User represents a user in the system.
type User struct {
	Name  string
	Email string
}
`
	filePath := filepath.Join(tmpDir, "user.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Find User struct
	var userStruct *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeStruct && e.Name == "User" {
			userStruct = e
			break
		}
	}
	if userStruct == nil {
		t.Fatal("User struct not found")
	}

	if userStruct.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", userStruct.Visibility, ast.VisibilityPublic)
	}
	if !strings.Contains(userStruct.DocComment, "represents a user") {
		t.Errorf("DocComment = %q, want to contain 'represents a user'", userStruct.DocComment)
	}
}

func TestParseFile_Interface(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

// Saver can save data.
type Saver interface {
	Save(data []byte) error
}
`
	filePath := filepath.Join(tmpDir, "saver.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var saverInterface *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeInterface && e.Name == "Saver" {
			saverInterface = e
			break
		}
	}
	if saverInterface == nil {
		t.Fatal("Saver interface not found")
	}

	if saverInterface.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", saverInterface.Visibility, ast.VisibilityPublic)
	}
}

func TestParseFile_Method(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

type User struct {
	Name string
}

// Greet returns a greeting message.
func (u *User) Greet() string {
	return "Hello, " + u.Name
}
`
	filePath := filepath.Join(tmpDir, "user.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var greetMethod *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeMethod && e.Name == "Greet" {
			greetMethod = e
			break
		}
	}
	if greetMethod == nil {
		t.Fatal("Greet method not found")
	}

	if greetMethod.Receiver == "" {
		t.Error("Receiver is empty")
	}
	if !strings.Contains(greetMethod.Receiver, "User") {
		t.Errorf("Receiver = %q, want to contain 'User'", greetMethod.Receiver)
	}
}

func TestParseFile_Imports(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

import (
	"context"
	"fmt"
	"github.com/example/pkg"
)

func Foo() {
	fmt.Println("hello")
}
`
	filePath := filepath.Join(tmpDir, "foo.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	expectedImports := []string{"context", "fmt", "github.com/example/pkg"}
	if len(result.Imports) != len(expectedImports) {
		t.Errorf("Imports count = %d, want %d", len(result.Imports), len(expectedImports))
	}

	importSet := make(map[string]bool)
	for _, imp := range result.Imports {
		importSet[imp] = true
	}
	for _, exp := range expectedImports {
		if !importSet[exp] {
			t.Errorf("Import %q not found", exp)
		}
	}
}

func TestParseFile_Constants(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

// MaxSize is the maximum allowed size.
const MaxSize = 1024

const (
	MinSize = 1
	defaultSize = 100
)
`
	filePath := filepath.Join(tmpDir, "const.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var maxSize, minSize, defSize *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeConst {
			switch e.Name {
			case "MaxSize":
				maxSize = e
			case "MinSize":
				minSize = e
			case "defaultSize":
				defSize = e
			}
		}
	}

	if maxSize == nil {
		t.Fatal("MaxSize constant not found")
	}
	if maxSize.Visibility != ast.VisibilityPublic {
		t.Errorf("MaxSize visibility = %q, want %q", maxSize.Visibility, ast.VisibilityPublic)
	}

	if minSize == nil {
		t.Fatal("MinSize constant not found")
	}

	if defSize == nil {
		t.Fatal("defaultSize constant not found")
	}
	if defSize.Visibility != ast.VisibilityPrivate {
		t.Errorf("defaultSize visibility = %q, want %q", defSize.Visibility, ast.VisibilityPrivate)
	}
}

func TestParseFile_EmbeddedTypes(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

type Base struct {
	ID int
}

type Derived struct {
	Base
	Name string
}
`
	filePath := filepath.Join(tmpDir, "embed.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var derived *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeStruct && e.Name == "Derived" {
			derived = e
			break
		}
	}
	if derived == nil {
		t.Fatal("Derived struct not found")
	}

	if len(derived.Embeds) != 1 {
		t.Errorf("Embeds count = %d, want 1", len(derived.Embeds))
	}
	// Embeds should now contain resolved entity IDs
	if len(derived.Embeds) > 0 && !strings.Contains(derived.Embeds[0], "Base") {
		t.Errorf("Embeds[0] = %q, want to contain 'Base'", derived.Embeds[0])
	}
}

func TestParseFile_FunctionCalls(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package example

import "fmt"

func helper() {}

func Main() {
	helper()
	fmt.Println("hello")
}
`
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var mainFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction && e.Name == "Main" {
			mainFunc = e
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("Main function not found")
	}

	// Should have at least helper() and fmt.Println calls
	if len(mainFunc.Calls) < 2 {
		t.Errorf("Calls count = %d, want at least 2", len(mainFunc.Calls))
	}

	// Calls should now contain resolved entity IDs
	var hasHelper, hasFmtPrintln bool
	for _, call := range mainFunc.Calls {
		if strings.Contains(call, "helper") {
			hasHelper = true
		}
		if strings.Contains(call, "fmt") && strings.Contains(call, "Println") {
			hasFmtPrintln = true
		}
	}
	if !hasHelper {
		t.Errorf("helper call not found in calls: %v", mainFunc.Calls)
	}
	if !hasFmtPrintln {
		t.Errorf("fmt.Println call not found in calls: %v", mainFunc.Calls)
	}
}

func TestParseDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple Go files
	files := map[string]string{
		"main.go":    "package main\n\nfunc main() {}\n",
		"util.go":    "package main\n\nfunc helper() {}\n",
		"sub/sub.go": "package sub\n\nfunc SubFunc() {}\n",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	p := NewParser("acme", "test", tmpDir)
	results, err := p.ParseDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("ParseDirectory: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("results count = %d, want 3", len(results))
	}
}

func TestParseDirectory_SkipsVendor(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files including vendor
	files := map[string]string{
		"main.go":              "package main\n\nfunc main() {}\n",
		"vendor/dep/dep.go":    "package dep\n\nfunc Dep() {}\n",
		"internal/internal.go": "package internal\n\nfunc Internal() {}\n",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	p := NewParser("acme", "test", tmpDir)
	results, err := p.ParseDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("ParseDirectory: %v", err)
	}

	// Should skip vendor, include main.go and internal/internal.go
	if len(results) != 2 {
		t.Errorf("results count = %d, want 2 (vendor should be skipped)", len(results))
	}
}

func TestParseFile_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid Go file
	code := `package example

func broken( {
`
	filePath := filepath.Join(tmpDir, "broken.go")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	_, err := p.ParseFile(context.Background(), filePath)
	if err == nil {
		t.Error("expected error for invalid Go file")
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	_, err := p.ParseFile(context.Background(), "/tmp/nonexistent.go")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestTypeNameToEntityID_Builtin(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	p.importMap = make(map[string]string)

	// Test built-in types
	builtins := []string{"int", "string", "bool", "error", "float64"}
	for _, b := range builtins {
		result := p.typeNameToEntityID(b, "test.go")
		expected := "builtin:" + b
		if result != expected {
			t.Errorf("typeNameToEntityID(%q) = %q, want %q", b, result, expected)
		}
	}
}

func TestTypeNameToEntityID_LocalType(t *testing.T) {
	p := NewParser("acme", "myproject", "/tmp")
	p.importMap = make(map[string]string)

	result := p.typeNameToEntityID("User", "models/user.go")
	if !strings.HasPrefix(result, "acme.semspec.code.type.myproject.") {
		t.Errorf("typeNameToEntityID(User) = %q, want prefix 'acme.semspec.code.type.myproject.'", result)
	}
	if !strings.Contains(result, "User") {
		t.Errorf("typeNameToEntityID(User) = %q, want to contain 'User'", result)
	}
}

func TestTypeNameToEntityID_ExternalType(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	p.importMap = map[string]string{
		"context": "context",
		"http":    "net/http",
		"json":    "encoding/json",
	}

	tests := []struct {
		typeName string
		expected string
	}{
		{"context.Context", "external:context.Context"},
		{"http.Request", "external:net/http.Request"},
		{"json.Decoder", "external:encoding/json.Decoder"},
	}

	for _, tt := range tests {
		result := p.typeNameToEntityID(tt.typeName, "test.go")
		if result != tt.expected {
			t.Errorf("typeNameToEntityID(%q) = %q, want %q", tt.typeName, result, tt.expected)
		}
	}
}

func TestTypeNameToEntityID_AliasedImport(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	p.importMap = map[string]string{
		"ctx": "context", // aliased import
	}

	result := p.typeNameToEntityID("ctx.Context", "test.go")
	if result != "external:context.Context" {
		t.Errorf("typeNameToEntityID(ctx.Context) = %q, want 'external:context.Context'", result)
	}
}

func TestCallNameToEntityID_Builtin(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	p.importMap = make(map[string]string)

	builtins := []string{"make", "len", "append", "panic", "recover"}
	for _, b := range builtins {
		result := p.callNameToEntityID(b, "test.go")
		expected := "builtin:" + b
		if result != expected {
			t.Errorf("callNameToEntityID(%q) = %q, want %q", b, result, expected)
		}
	}
}

func TestCallNameToEntityID_ExternalFunc(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	p.importMap = map[string]string{
		"fmt":  "fmt",
		"json": "encoding/json",
	}

	tests := []struct {
		callName string
		expected string
	}{
		{"fmt.Println", "external:fmt.Println"},
		{"json.Marshal", "external:encoding/json.Marshal"},
	}

	for _, tt := range tests {
		result := p.callNameToEntityID(tt.callName, "test.go")
		if result != tt.expected {
			t.Errorf("callNameToEntityID(%q) = %q, want %q", tt.callName, result, tt.expected)
		}
	}
}

func TestCallNameToEntityID_LocalFunc(t *testing.T) {
	p := NewParser("acme", "myproject", "/tmp")
	p.importMap = make(map[string]string)

	result := p.callNameToEntityID("helper", "utils/helper.go")
	if !strings.HasPrefix(result, "acme.semspec.code.function.myproject.") {
		t.Errorf("callNameToEntityID(helper) = %q, want prefix 'acme.semspec.code.function.myproject.'", result)
	}
	if !strings.Contains(result, "helper") {
		t.Errorf("callNameToEntityID(helper) = %q, want to contain 'helper'", result)
	}
}

func TestParseDirectory_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a few files
	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tmpDir, filepath.Base(tmpDir)+string(rune('a'+i))+".go")
		code := "package example\n\nfunc Foo" + string(rune('A'+i)) + "() {}\n"
		if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	p := NewParser("acme", "test", tmpDir)
	_, err := p.ParseDirectory(ctx, tmpDir)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
