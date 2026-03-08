package java

import (
	"context"
	"fmt"
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

func TestParseFile_SimpleClass(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Calculator {
    public int add(int a, int b) {
        return a + b;
    }
}
`
	filePath := filepath.Join(tmpDir, "Calculator.java")
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
	if result.FileEntity.Language != "java" {
		t.Errorf("Language = %q, want 'java'", result.FileEntity.Language)
	}
	if result.Package != "com.example" {
		t.Errorf("Package = %q, want 'com.example'", result.Package)
	}

	// Find Calculator class
	var calcClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass && e.Name == "Calculator" {
			calcClass = e
			break
		}
	}
	if calcClass == nil {
		t.Fatal("Calculator class not found")
	}

	// Check class properties
	if calcClass.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", calcClass.Visibility, ast.VisibilityPublic)
	}
	if calcClass.Language != "java" {
		t.Errorf("Language = %q, want 'java'", calcClass.Language)
	}

	// Check that add method is in Contains
	if len(calcClass.Contains) < 1 {
		t.Errorf("Contains count = %d, want at least 1", len(calcClass.Contains))
	}

	// Check entity ID format
	if !strings.HasPrefix(calcClass.ID, "acme.semsource.java.test.class.") {
		t.Errorf("ID = %q, want prefix 'acme.semsource.java.test.class.'", calcClass.ID)
	}
}

func TestParseFile_Interface(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public interface Runnable {
    void run();
    int getPriority();
}
`
	filePath := filepath.Join(tmpDir, "Runnable.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var runnableIface *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeInterface && e.Name == "Runnable" {
			runnableIface = e
			break
		}
	}
	if runnableIface == nil {
		t.Fatal("Runnable interface not found")
	}

	if runnableIface.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", runnableIface.Visibility, ast.VisibilityPublic)
	}
	if len(runnableIface.Contains) < 2 {
		t.Errorf("Contains count = %d, want at least 2 methods", len(runnableIface.Contains))
	}
}

func TestParseFile_Enum(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public enum Status {
    PENDING,
    ACTIVE,
    COMPLETED
}
`
	filePath := filepath.Join(tmpDir, "Status.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var statusEnum *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeEnum && e.Name == "Status" {
			statusEnum = e
			break
		}
	}
	if statusEnum == nil {
		t.Fatal("Status enum not found")
	}

	if statusEnum.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", statusEnum.Visibility, ast.VisibilityPublic)
	}
}

func TestParseFile_Record(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public record Point(double x, double y) {}
`
	filePath := filepath.Join(tmpDir, "Point.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var pointRecord *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "Point" {
			pointRecord = e
			break
		}
	}
	if pointRecord == nil {
		t.Fatal("Point record not found")
	}

	// Records should be mapped to TypeStruct
	if pointRecord.Type != ast.TypeStruct {
		t.Errorf("Type = %q, want %q", pointRecord.Type, ast.TypeStruct)
	}
	if pointRecord.Visibility != ast.VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", pointRecord.Visibility, ast.VisibilityPublic)
	}
}

func TestParseFile_Inheritance(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Dog extends Animal implements Pet {
    public void bark() {
        System.out.println("Woof!");
    }
}
`
	filePath := filepath.Join(tmpDir, "Dog.java")
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

	// Check extends
	if len(dogClass.Extends) != 1 {
		t.Errorf("Extends count = %d, want 1", len(dogClass.Extends))
	}
	if len(dogClass.Extends) > 0 && !strings.Contains(dogClass.Extends[0], "Animal") {
		t.Errorf("Extends[0] = %q, want to contain 'Animal'", dogClass.Extends[0])
	}

	// Check implements
	if len(dogClass.Implements) != 1 {
		t.Errorf("Implements count = %d, want 1", len(dogClass.Implements))
	}
	if len(dogClass.Implements) > 0 && !strings.Contains(dogClass.Implements[0], "Pet") {
		t.Errorf("Implements[0] = %q, want to contain 'Pet'", dogClass.Implements[0])
	}
}

func TestParseFile_Annotations(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

import javax.annotation.Nonnull;

@Entity
@Table(name = "users")
public class User {
    @Override
    public String toString() {
        return "User";
    }
}
`
	filePath := filepath.Join(tmpDir, "User.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

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

	// Annotations should be in DocComment
	if !strings.Contains(userClass.DocComment, "@Entity") {
		t.Errorf("DocComment = %q, want to contain '@Entity'", userClass.DocComment)
	}
	if !strings.Contains(userClass.DocComment, "@Table") {
		t.Errorf("DocComment = %q, want to contain '@Table'", userClass.DocComment)
	}
}

func TestParseFile_Fields(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Counter {
    private int count;
    public static final int MAX_COUNT = 100;
    private String name, description;
}
`
	filePath := filepath.Join(tmpDir, "Counter.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var counterClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass && e.Name == "Counter" {
			counterClass = e
			break
		}
	}
	if counterClass == nil {
		t.Fatal("Counter class not found")
	}

	// Should have at least 4 fields: count, MAX_COUNT, name, description
	if len(counterClass.Contains) < 4 {
		t.Errorf("Contains count = %d, want at least 4", len(counterClass.Contains))
	}

	// Check individual fields
	fieldMap := make(map[string]*ast.CodeEntity)
	for _, e := range result.Entities {
		if e.Type == ast.TypeVar {
			fieldMap[e.Name] = e
		}
	}

	if count, ok := fieldMap["count"]; ok {
		if count.Visibility != ast.VisibilityPrivate {
			t.Errorf("count visibility = %q, want %q", count.Visibility, ast.VisibilityPrivate)
		}
	} else {
		t.Error("count field not found")
	}

	if maxCount, ok := fieldMap["MAX_COUNT"]; ok {
		if !strings.Contains(maxCount.DocComment, "static") {
			t.Errorf("MAX_COUNT DocComment = %q, want to contain 'static'", maxCount.DocComment)
		}
		if !strings.Contains(maxCount.DocComment, "final") {
			t.Errorf("MAX_COUNT DocComment = %q, want to contain 'final'", maxCount.DocComment)
		}
	} else {
		t.Error("MAX_COUNT field not found")
	}
}

func TestParseFile_Methods(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Math {
    public static int add(int a, int b) {
        return a + b;
    }

    private void helper() {
    }
}
`
	filePath := filepath.Join(tmpDir, "Math.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Find methods
	methodMap := make(map[string]*ast.CodeEntity)
	for _, e := range result.Entities {
		if e.Type == ast.TypeMethod {
			methodMap[e.Name] = e
		}
	}

	if add, ok := methodMap["add"]; ok {
		if add.Visibility != ast.VisibilityPublic {
			t.Errorf("add visibility = %q, want %q", add.Visibility, ast.VisibilityPublic)
		}
		if !strings.Contains(add.DocComment, "static") {
			t.Errorf("add DocComment = %q, want to contain 'static'", add.DocComment)
		}
		if len(add.Parameters) != 2 {
			t.Errorf("add parameters count = %d, want 2", len(add.Parameters))
		}
		if len(add.Returns) != 1 {
			t.Errorf("add returns count = %d, want 1", len(add.Returns))
		}
	} else {
		t.Error("add method not found")
	}

	if helper, ok := methodMap["helper"]; ok {
		if helper.Visibility != ast.VisibilityPrivate {
			t.Errorf("helper visibility = %q, want %q", helper.Visibility, ast.VisibilityPrivate)
		}
		if len(helper.Returns) != 0 {
			t.Errorf("helper returns count = %d, want 0 (void)", len(helper.Returns))
		}
	} else {
		t.Error("helper method not found")
	}
}

func TestParseFile_Constructor(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Person {
    private String name;

    public Person(String name) {
        this.name = name;
    }
}
`
	filePath := filepath.Join(tmpDir, "Person.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Find constructor (treated as a method)
	var constructor *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeMethod && e.Name == "Person" {
			constructor = e
			break
		}
	}
	if constructor == nil {
		t.Fatal("Person constructor not found")
	}

	if constructor.Visibility != ast.VisibilityPublic {
		t.Errorf("Constructor visibility = %q, want %q", constructor.Visibility, ast.VisibilityPublic)
	}
	if len(constructor.Parameters) != 1 {
		t.Errorf("Constructor parameters count = %d, want 1", len(constructor.Parameters))
	}
	if constructor.Receiver == "" {
		t.Error("Constructor should have Receiver set to class ID")
	}
}

func TestParseFile_Generics(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

import java.util.List;

public class Container<T> {
    private List<String> items;

    public T get(int index) {
        return null;
    }
}
`
	filePath := filepath.Join(tmpDir, "Container.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Find items field
	var itemsField *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeVar && e.Name == "items" {
			itemsField = e
			break
		}
	}
	if itemsField == nil {
		t.Fatal("items field not found")
	}

	// Generics should be stripped, leaving base type List
	if len(itemsField.References) < 1 {
		t.Error("items field should have type reference")
	}
}

func TestParseFile_InnerClass(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class Outer {
    private class Inner {
        void doSomething() {
        }
    }
}
`
	filePath := filepath.Join(tmpDir, "Outer.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	var outerClass *ast.CodeEntity
	var innerClass *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass {
			if e.Name == "Outer" {
				outerClass = e
			} else if e.Name == "Inner" {
				innerClass = e
			}
		}
	}

	if outerClass == nil {
		t.Fatal("Outer class not found")
	}
	if innerClass == nil {
		t.Fatal("Inner class not found")
	}

	// Inner class should be in Outer's Contains
	if !containsID(outerClass.Contains, innerClass.ID) {
		t.Error("Outer class should contain Inner class")
	}

	// Inner class should have ContainedBy pointing to Outer
	if innerClass.ContainedBy != outerClass.ID {
		t.Errorf("Inner ContainedBy = %q, want %q", innerClass.ContainedBy, outerClass.ID)
	}
}

func TestParseFile_Imports(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

import java.util.List;
import java.util.ArrayList;
import java.io.*;
import static java.lang.Math.PI;
`
	filePath := filepath.Join(tmpDir, "Imports.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	expectedImports := []string{"java.util.List", "java.util.ArrayList", "java.io"}

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

func TestParseFile_Visibility(t *testing.T) {
	tmpDir := t.TempDir()

	code := `package com.example;

public class PublicClass {
}

class PackagePrivateClass {
}

public class VisibilityTest {
    public int publicField;
    private int privateField;
    protected int protectedField;
    int packagePrivateField;

    public void publicMethod() {}
    private void privateMethod() {}
    protected void protectedMethod() {}
    void packagePrivateMethod() {}
}
`
	filePath := filepath.Join(tmpDir, "VisibilityTest.java")
	if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := NewParser("acme", "test", tmpDir)
	result, err := p.ParseFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	visibilityMap := make(map[string]ast.Visibility)
	for _, e := range result.Entities {
		visibilityMap[e.Name] = e.Visibility
	}

	tests := []struct {
		name       string
		visibility ast.Visibility
	}{
		{"PublicClass", ast.VisibilityPublic},
		{"PackagePrivateClass", ast.VisibilityPrivate},
		{"publicField", ast.VisibilityPublic},
		{"privateField", ast.VisibilityPrivate},
		{"protectedField", ast.VisibilityPrivate}, // Protected treated as private
		{"packagePrivateField", ast.VisibilityPrivate},
		{"publicMethod", ast.VisibilityPublic},
		{"privateMethod", ast.VisibilityPrivate},
		{"protectedMethod", ast.VisibilityPrivate},
		{"packagePrivateMethod", ast.VisibilityPrivate},
	}

	for _, tt := range tests {
		if v, ok := visibilityMap[tt.name]; !ok {
			t.Errorf("%s not found", tt.name)
		} else if v != tt.visibility {
			t.Errorf("%s visibility = %q, want %q", tt.name, v, tt.visibility)
		}
	}
}

func TestParseDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"Main.java":           "public class Main { }",
		"Utils.java":          "class Utils { }",
		"sub/Helper.java":     "package sub; public class Helper { }",
		"sub/inner/Deep.java": "package sub.inner; class Deep { }",
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

func TestParseDirectory_SkipsBuildDirs(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"src/Main.java":               "public class Main { }",
		"target/classes/Main.class":   "// Compiled class",
		"build/generated/Gen.java":    "class Gen { }",
		".gradle/wrapper/gradle.java": "class Gradle { }",
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

	// Should only find src/Main.java (target, build, .gradle are skipped)
	if len(results) != 1 {
		t.Errorf("results count = %d, want 1 (build directories should be skipped)", len(results))
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")
	_, err := p.ParseFile(context.Background(), "/tmp/nonexistent.java")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestTypeNameToEntityID_Builtin(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	builtins := []string{"int", "String", "boolean", "Integer", "Double", "Object"}
	for _, b := range builtins {
		result := p.typeNameToEntityID(b, "Test.java")
		expected := "builtin:" + b
		if result != expected {
			t.Errorf("typeNameToEntityID(%q) = %q, want %q", b, result, expected)
		}
	}
}

func TestTypeNameToEntityID_FullyQualified(t *testing.T) {
	p := NewParser("acme", "test", "/tmp")

	result := p.typeNameToEntityID("java.util.List", "Test.java")
	expected := "external:java.util.List"
	if result != expected {
		t.Errorf("typeNameToEntityID(java.util.List) = %q, want %q", result, expected)
	}

	result = p.typeNameToEntityID("com.example.MyClass", "Test.java")
	expected = "external:com.example.MyClass"
	if result != expected {
		t.Errorf("typeNameToEntityID(com.example.MyClass) = %q, want %q", result, expected)
	}
}

func TestTypeNameToEntityID_LocalType(t *testing.T) {
	p := NewParser("acme", "myproject", "/tmp")

	result := p.typeNameToEntityID("User", "models/User.java")
	if !strings.HasPrefix(result, "acme.semsource.java.myproject.type.") {
		t.Errorf("typeNameToEntityID(User) = %q, want prefix 'acme.semsource.java.myproject.type.'", result)
	}
	if !strings.Contains(result, "User") {
		t.Errorf("typeNameToEntityID(User) = %q, want to contain 'User'", result)
	}
}

func TestExtractPackageName(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		code     string
		expected string
	}{
		{"package com.example;", "com.example"},
		{"package org.acme.project;", "org.acme.project"},
		{"// No package", ""},
	}

	for i, tt := range tests {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("Test%d.java", i))
		if err := os.WriteFile(filePath, []byte(tt.code), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		p := NewParser("acme", "test", tmpDir)
		result, err := p.ParseFile(context.Background(), filePath)
		if err != nil {
			t.Fatalf("ParseFile: %v", err)
		}

		if result.Package != tt.expected {
			t.Errorf("Package = %q, want %q", result.Package, tt.expected)
		}
	}
}

func TestIsBuiltinType(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"int", true},
		{"String", true},
		{"boolean", true},
		{"Integer", true},
		{"Object", true},
		{"Exception", true},
		{"void", true},
		{"CustomType", false},
		{"MyClass", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isBuiltinType(tt.input)
		if result != tt.expected {
			t.Errorf("isBuiltinType(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestParseDirectory_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("File%d.java", i))
		code := fmt.Sprintf("public class File%d { }", i)
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

// Helper function
func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
