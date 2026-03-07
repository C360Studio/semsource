package python

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

	code := `"""Module for math operations."""

def add(a: int, b: int) -> int:
    """Add two integers and return the sum."""
    return a + b
`
	filePath := filepath.Join(tmpDir, "math.py")
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
	if result.Hash == "" {
		t.Error("Hash is empty")
	}
	if result.FileEntity.Language != "python" {
		t.Errorf("Language = %q, want 'python'", result.FileEntity.Language)
	}

	// Check we found the function
	var addFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction && e.Name == "add" {
			addFunc = e
			break
		}
	}
	if addFunc == nil {
		t.Fatal("add function not found")
	}

	// Check function properties
	if addFunc.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", addFunc.Visibility, ast.VisibilityPublic)
	}
	if addFunc.DocComment == "" || !strings.Contains(addFunc.DocComment, "Add two integers") {
		t.Errorf("DocComment = %q, want to contain 'Add two integers'", addFunc.DocComment)
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

func TestParseFile_Class(t *testing.T) {
	tmpDir := t.TempDir()

	code := `class User:
    """Represents a user in the system."""

    def __init__(self, name: str, email: str):
        self.name = name
        self.email = email

    def greet(self) -> str:
        """Return a greeting message."""
        return f"Hello, {self.name}"
`
	filePath := filepath.Join(tmpDir, "user.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Find User class
	var userClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass && e.Name == "User" {
			userClass = e
			break
		}
	}
	if userClass == nil {
		t.Fatal("User class not found")
	}

	if userClass.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", userClass.Visibility, ast.VisibilityPublic)
	}
	if !strings.Contains(userClass.DocComment, "Represents a user") {
		t.Errorf("DocComment = %q, want to contain 'Represents a user'", userClass.DocComment)
	}
	if userClass.Language != "python" {
		t.Errorf("Language = %q, want 'python'", userClass.Language)
	}

	// Check that methods are extracted (at least __init__ and greet)
	if len(userClass.Contains) < 2 {
		t.Errorf("Contains count = %d, want at least 2", len(userClass.Contains))
	}
}

func TestParseFile_ClassInheritance(t *testing.T) {
	tmpDir := t.TempDir()

	code := `class Animal:
    def speak(self) -> str:
        pass

class Dog(Animal):
    def speak(self) -> str:
        return "Woof!"
`
	filePath := filepath.Join(tmpDir, "animals.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var dogClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass && e.Name == "Dog" {
			dogClass = e
			break
		}
	}
	if dogClass == nil {
		t.Fatal("Dog class not found")
	}

	if len(dogClass.Extends) != 1 {
		t.Errorf("Extends count = %d, want 1", len(dogClass.Extends))
	}
	if len(dogClass.Extends) > 0 && !strings.Contains(dogClass.Extends[0], "Animal") {
		t.Errorf("Extends[0] = %q, want to contain 'Animal'", dogClass.Extends[0])
	}
}

func TestParseFile_DataClass(t *testing.T) {
	tmpDir := t.TempDir()

	code := `from dataclasses import dataclass

@dataclass
class Point:
    """A 2D point."""
    x: float
    y: float
`
	filePath := filepath.Join(tmpDir, "point.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var pointClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "Point" {
			pointClass = e
			break
		}
	}
	if pointClass == nil {
		t.Fatal("Point class not found")
	}

	// Dataclass should be recognized as TypeStruct
	if pointClass.Type != ast.TypeStruct {
		t.Errorf("Type = %q, want %q", pointClass.Type, ast.TypeStruct)
	}
	if !strings.Contains(pointClass.DocComment, "@dataclass") {
		t.Errorf("DocComment = %q, want to contain '@dataclass'", pointClass.DocComment)
	}
}

func TestParseFile_AsyncFunction(t *testing.T) {
	tmpDir := t.TempDir()

	code := `async def fetch_data(url: str) -> dict:
    """Fetch data from a URL asynchronously."""
    pass
`
	filePath := filepath.Join(tmpDir, "async_utils.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var fetchFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction && e.Name == "fetch_data" {
			fetchFunc = e
			break
		}
	}
	if fetchFunc == nil {
		t.Fatal("fetch_data function not found")
	}

	if !strings.Contains(fetchFunc.DocComment, "async") {
		t.Errorf("DocComment = %q, want to contain 'async'", fetchFunc.DocComment)
	}
}

func TestParseFile_PrivateVisibility(t *testing.T) {
	tmpDir := t.TempDir()

	code := `def public_func():
    pass

def _private_func():
    pass

class PublicClass:
    pass

class _PrivateClass:
    pass

PUBLIC_CONST = 1
_PRIVATE_CONST = 2
`
	filePath := filepath.Join(tmpDir, "visibility.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	visibility := make(map[string]ast.Visibility)
	for _, e := range result.Entities {
		visibility[e.Name] = e.Visibility
	}

	tests := []struct {
		name       string
		visibility ast.Visibility
	}{
		{"public_func", ast.VisibilityPublic},
		{"_private_func", ast.VisibilityPrivate},
		{"PublicClass", ast.VisibilityPublic},
		{"_PrivateClass", ast.VisibilityPrivate},
		{"PUBLIC_CONST", ast.VisibilityPublic},
		{"_PRIVATE_CONST", ast.VisibilityPrivate},
	}

	for _, tt := range tests {
		if v, ok := visibility[tt.name]; !ok {
			t.Errorf("%s not found", tt.name)
		} else if v != tt.visibility {
			t.Errorf("%s visibility = %q, want %q", tt.name, v, tt.visibility)
		}
	}
}

func TestParseFile_Imports(t *testing.T) {
	tmpDir := t.TempDir()

	code := `import os
import sys
from collections import defaultdict
from typing import List, Dict
from mypackage.submodule import MyClass
`
	filePath := filepath.Join(tmpDir, "imports.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	expectedImports := []string{"os", "sys", "collections", "typing", "mypackage.submodule"}

	importSet := make(map[string]bool)
	for _, imp := range result.Imports {
		importSet[imp] = true
	}

	for _, exp := range expectedImports {
		if !importSet[exp] {
			t.Errorf("Import %q not found in %v", exp, result.Imports)
		}
	}
}

func TestParseFile_Constants(t *testing.T) {
	tmpDir := t.TempDir()

	code := `MAX_SIZE = 1024
MIN_SIZE = 1
DEFAULT_NAME = "unknown"
_INTERNAL_FLAG = True
`
	filePath := filepath.Join(tmpDir, "constants.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var maxSize, minSize *ast.CodeEntity
	for _, e := range result.Entities {
		switch e.Name {
		case "MAX_SIZE":
			maxSize = e
		case "MIN_SIZE":
			minSize = e
		}
	}

	if maxSize == nil {
		t.Fatal("MAX_SIZE constant not found")
	}
	if maxSize.Type != ast.TypeConst {
		t.Errorf("MAX_SIZE type = %q, want %q", maxSize.Type, ast.TypeConst)
	}
	if maxSize.Visibility != ast.VisibilityPublic {
		t.Errorf("MAX_SIZE visibility = %q, want %q", maxSize.Visibility, ast.VisibilityPublic)
	}

	if minSize == nil {
		t.Fatal("MIN_SIZE constant not found")
	}
}

func TestParseFile_DecoratedFunction(t *testing.T) {
	tmpDir := t.TempDir()

	code := `from functools import lru_cache

@lru_cache(maxsize=128)
def expensive_computation(n: int) -> int:
    """Perform expensive computation with caching."""
    return n * n
`
	filePath := filepath.Join(tmpDir, "decorated.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var expFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction && e.Name == "expensive_computation" {
			expFunc = e
			break
		}
	}
	if expFunc == nil {
		t.Fatal("expensive_computation function not found")
	}

	// Check decorator is captured in docstring
	if !strings.Contains(expFunc.DocComment, "@lru_cache") {
		t.Errorf("DocComment = %q, want to contain '@lru_cache'", expFunc.DocComment)
	}
}

func TestParseFile_ModuleDocstring(t *testing.T) {
	tmpDir := t.TempDir()

	code := `"""This is the module docstring.

It can span multiple lines.
"""

def some_function():
    pass
`
	filePath := filepath.Join(tmpDir, "documented.py")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if result.FileEntity == nil {
		t.Fatal("FileEntity is nil")
	}
	if !strings.Contains(result.FileEntity.DocComment, "module docstring") {
		t.Errorf("FileEntity.DocComment = %q, want to contain 'module docstring'", result.FileEntity.DocComment)
	}
}

func TestParseDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"main.py":         "def main(): pass\n",
		"utils.py":        "def helper(): pass\n",
		"sub/module.py":   "def sub_func(): pass\n",
		"sub/__init__.py": "# Package marker\n",
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

	if len(results) != 4 {
		t.Errorf("results count = %d, want 4", len(results))
	}
}

func TestParseDirectory_SkipsVenv(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"main.py":                         "def main(): pass\n",
		"venv/lib/site-packages/dep.py":   "def dep(): pass\n",
		".venv/lib/site-packages/dep.py":  "def dep(): pass\n",
		"__pycache__/main.cpython-39.pyc": "# Bytecode\n",
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

	// Should only find main.py
	if len(results) != 1 {
		t.Errorf("results count = %d, want 1 (venv and __pycache__ should be skipped)", len(results))
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	_, err := p.ParseFile(context.Background(), "/tmp/nonexistent.py")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestTypeNameToEntityID_Builtin(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	builtins := []string{"int", "str", "bool", "float", "dict", "list", "Any", "Optional"}
	for _, b := range builtins {
		result := p.typeNameToEntityID(b, "test.py")
		expected := "builtin:" + b
		if result != expected {
			t.Errorf("typeNameToEntityID(%q) = %q, want %q", b, result, expected)
		}
	}
}

func TestTypeNameToEntityID_Generic(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	// Generic types should resolve to their base type
	result := p.typeNameToEntityID("List[int]", "test.py")
	if result != "builtin:List" {
		t.Errorf("typeNameToEntityID(List[int]) = %q, want 'builtin:List'", result)
	}

	result = p.typeNameToEntityID("Dict[str, Any]", "test.py")
	if result != "builtin:Dict" {
		t.Errorf("typeNameToEntityID(Dict[str, Any]) = %q, want 'builtin:Dict'", result)
	}
}

func TestTypeNameToEntityID_LocalType(t *testing.T) {
	p := NewParser("acme", "myproject", "/tmp")

	result := p.typeNameToEntityID("User", "models/user.py")
	if !strings.HasPrefix(result, "acme.semspec.code.type.myproject.") {
		t.Errorf("typeNameToEntityID(User) = %q, want prefix 'acme.semspec.code.type.myproject.'", result)
	}
	if !strings.Contains(result, "User") {
		t.Errorf("typeNameToEntityID(User) = %q, want to contain 'User'", result)
	}
}

func TestTypeNameToEntityID_ExternalType(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	result := p.typeNameToEntityID("module.ClassName", "test.py")
	if result != "external:module.ClassName" {
		t.Errorf("typeNameToEntityID(module.ClassName) = %q, want 'external:module.ClassName'", result)
	}
}

func TestExtractModuleName(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	tests := []struct {
		relPath  string
		expected string
	}{
		{"main.py", "main"},
		{"auth/service.py", "auth.service"},
		{"auth/__init__.py", "auth"},
		{"src/models/user.py", "src.models.user"},
	}

	for _, tt := range tests {
		result := p.extractModuleName(tt.relPath)
		if result != tt.expected {
			t.Errorf("extractModuleName(%q) = %q, want %q", tt.relPath, result, tt.expected)
		}
	}
}

func TestIsAllCaps(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"MAX_SIZE", true},
		{"API_KEY", true},
		{"HTTP_200_OK", true},
		{"lowercase", false},
		{"MixedCase", false},
		{"UPPER", true},
		{"", false},
	}

	for _, tt := range tests {
		result := isAllCaps(tt.input)
		if result != tt.expected {
			t.Errorf("isAllCaps(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestParseDirectory_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tmpDir, "file"+string(rune('a'+i))+".py")
		code := "def func_" + string(rune('a'+i)) + "(): pass\n"
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
