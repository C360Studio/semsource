package ts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func TestParseFile_TypeScript(t *testing.T) {
	dir := t.TempDir()
	tsPath := writeTypeScriptFixture(t, dir)

	p := NewParser("test-org", "test-project", dir)
	result, err := p.ParseFile(context.Background(), tsPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	assertTSFileEntity(t, result)
	assertTSEntities(t, result)
	assertTSRelationships(t, result)
}

// writeTypeScriptFixture writes a comprehensive TypeScript source file for testing.
func writeTypeScriptFixture(t *testing.T, dir string) string {
	t.Helper()
	tsContent := `import { Component } from './base';
import type { Config } from './types';

export interface User {
    id: string;
    name: string;
}

export type Status = 'active' | 'inactive';

export enum Role {
    Admin = 'admin',
    User = 'user',
}

export class UserService extends Component implements Serializable {
    private users: User[] = [];
    #privateField: string;

    constructor() {
        super();
    }

    public getUsers(): User[] {
        return this.users;
    }

    private fetchData(): void {
        // implementation
    }

    async loadData(): Promise<void> {
        // async implementation
    }
}

export function createUser(name: string): User {
    return { id: '1', name };
}

export const DEFAULT_USER: User = { id: '0', name: 'Guest' };

const privateHelper = () => {
    return 'helper';
};

export const arrowFunc = async (x: number): Promise<number> => {
    return x * 2;
};

let mutableVar = 'test';
var oldStyleVar: string;
`
	tsPath := filepath.Join(dir, "user.ts")
	if err := os.WriteFile(tsPath, []byte(tsContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	return tsPath
}

// assertTSFileEntity verifies the file-level entity properties.
func assertTSFileEntity(t *testing.T, result *ast.ParseResult) {
	t.Helper()
	if result.FileEntity == nil {
		t.Fatal("FileEntity is nil")
	}
	if result.FileEntity.Language != "typescript" {
		t.Errorf("Expected language 'typescript', got '%s'", result.FileEntity.Language)
	}
	if len(result.Imports) < 1 {
		t.Errorf("Expected at least 1 import, got %d", len(result.Imports))
	}
	if !containsImport(result.Imports, "./base") {
		t.Errorf("Expected import './base', got %v", result.Imports)
	}
}

// assertTSEntities verifies that the expected code entities are extracted.
func assertTSEntities(t *testing.T, result *ast.ParseResult) {
	t.Helper()
	entities := make(map[ast.CodeEntityType][]string)
	for _, e := range result.Entities {
		entities[e.Type] = append(entities[e.Type], e.Name)
	}

	if !contains(entities[ast.TypeInterface], "User") {
		t.Errorf("Expected interface 'User', got %v", entities[ast.TypeInterface])
	}
	if !contains(entities[ast.TypeType], "Status") {
		t.Errorf("Expected type alias 'Status', got %v", entities[ast.TypeType])
	}
	if !contains(entities[ast.TypeEnum], "Role") {
		t.Errorf("Expected enum 'Role', got %v", entities[ast.TypeEnum])
	}
	if !contains(entities[ast.TypeClass], "UserService") {
		t.Errorf("Expected class 'UserService', got %v", entities[ast.TypeClass])
	}

	funcs := entities[ast.TypeFunction]
	for _, fn := range []string{"createUser", "privateHelper", "arrowFunc"} {
		if !contains(funcs, fn) {
			t.Errorf("Expected function %q, got %v", fn, funcs)
		}
	}

	methods := entities[ast.TypeMethod]
	for _, m := range []string{"getUsers", "fetchData", "loadData"} {
		if !contains(methods, m) {
			t.Errorf("Expected method %q, got %v", m, methods)
		}
	}

	if !contains(entities[ast.TypeConst], "DEFAULT_USER") {
		t.Errorf("Expected const 'DEFAULT_USER', got %v", entities[ast.TypeConst])
	}
	for _, v := range []string{"mutableVar", "oldStyleVar"} {
		if !contains(entities[ast.TypeVar], v) {
			t.Errorf("Expected var %q, got %v", v, entities[ast.TypeVar])
		}
	}
}

// assertTSRelationships verifies class inheritance and method visibility.
func assertTSRelationships(t *testing.T, result *ast.ParseResult) {
	t.Helper()
	var userService, fetchData *ast.CodeEntity
	for _, e := range result.Entities {
		switch {
		case e.Name == "UserService" && e.Type == ast.TypeClass:
			userService = e
		case e.Name == "fetchData" && e.Type == ast.TypeMethod:
			fetchData = e
		}
	}
	if userService == nil {
		t.Fatal("UserService entity not found")
	}
	if len(userService.Extends) == 0 {
		t.Error("Expected UserService to extend Component")
	}
	if len(userService.Implements) == 0 {
		t.Error("Expected UserService to implement Serializable")
	}
	if fetchData != nil && fetchData.Visibility != ast.VisibilityPrivate {
		t.Errorf("Expected 'fetchData' to be private, got %s", fetchData.Visibility)
	}
}

func TestParseFile_JavaScript(t *testing.T) {
	dir := t.TempDir()

	jsContent := `import { helper } from './utils';
const { readFile } = require('fs');

export class Counter {
    count = 0;

    increment() {
        this.count++;
    }

    #privateMethod() {
        // private
    }
}

export function add(a, b) {
    return a + b;
}

export const multiply = (a, b) => a * b;

const SECRET = 'secret';

export default class DefaultExport {
    method() {}
}

export async function fetchData() {
    return await Promise.resolve();
}

const asyncArrow = async () => {
    return 'data';
};
`

	jsPath := filepath.Join(dir, "counter.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), jsPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.FileEntity.Language != "javascript" {
		t.Errorf("Expected language 'javascript', got '%s'", result.FileEntity.Language)
	}

	// Collect entity names by type
	entities := make(map[ast.CodeEntityType][]string)
	for _, e := range result.Entities {
		entities[e.Type] = append(entities[e.Type], e.Name)
	}

	// Check classes
	classes := entities[ast.TypeClass]
	if !contains(classes, "Counter") {
		t.Errorf("Expected class 'Counter', got %v", classes)
	}
	if !contains(classes, "DefaultExport") {
		t.Errorf("Expected class 'DefaultExport', got %v", classes)
	}

	// Check functions
	funcs := entities[ast.TypeFunction]
	if !contains(funcs, "add") {
		t.Errorf("Expected function 'add', got %v", funcs)
	}
	if !contains(funcs, "multiply") {
		t.Errorf("Expected arrow function 'multiply', got %v", funcs)
	}
	if !contains(funcs, "fetchData") {
		t.Errorf("Expected function 'fetchData', got %v", funcs)
	}
	if !contains(funcs, "asyncArrow") {
		t.Errorf("Expected arrow function 'asyncArrow', got %v", funcs)
	}

	// Check consts
	if !contains(entities[ast.TypeConst], "SECRET") {
		t.Errorf("Expected const 'SECRET', got %v", entities[ast.TypeConst])
	}

	// Check imports (both ES6 and CommonJS)
	if len(result.Imports) < 2 {
		t.Errorf("Expected at least 2 imports, got %d", len(result.Imports))
	}
	if !containsImport(result.Imports, "./utils") {
		t.Errorf("Expected import './utils', got %v", result.Imports)
	}
	if !containsImport(result.Imports, "fs") {
		t.Errorf("Expected require 'fs', got %v", result.Imports)
	}
}

func TestParseFile_JSX(t *testing.T) {
	dir := t.TempDir()

	jsxContent := `import React from 'react';

export function Button({ onClick, children }) {
    return (
        <button onClick={onClick}>
            {children}
        </button>
    );
}

export const Card = ({ title, content }) => {
    return (
        <div className="card">
            <h2>{title}</h2>
            <p>{content}</p>
        </div>
    );
};

export class ClassComponent extends React.Component {
    render() {
        return <div>Hello</div>;
    }
}
`

	jsxPath := filepath.Join(dir, "components.jsx")
	if err := os.WriteFile(jsxPath, []byte(jsxContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), jsxPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Collect entity names by type
	entities := make(map[ast.CodeEntityType][]string)
	for _, e := range result.Entities {
		entities[e.Type] = append(entities[e.Type], e.Name)
	}

	// Check functions (JSX components)
	funcs := entities[ast.TypeFunction]
	if !contains(funcs, "Button") {
		t.Errorf("Expected function 'Button', got %v", funcs)
	}
	if !contains(funcs, "Card") {
		t.Errorf("Expected arrow function 'Card', got %v", funcs)
	}

	// Check class component
	if !contains(entities[ast.TypeClass], "ClassComponent") {
		t.Errorf("Expected class 'ClassComponent', got %v", entities[ast.TypeClass])
	}
}

func TestParseFile_TSX(t *testing.T) {
	dir := t.TempDir()

	tsxContent := `import React from 'react';

interface Props {
    name: string;
    onClick: () => void;
}

export const Greeting: React.FC<Props> = ({ name, onClick }) => {
    return <div onClick={onClick}>Hello, {name}!</div>;
};

export function TypedButton({ label }: { label: string }) {
    return <button>{label}</button>;
}

export class TypedComponent extends React.Component<Props> {
    render() {
        return <div>{this.props.name}</div>;
    }
}
`

	tsxPath := filepath.Join(dir, "typed-components.tsx")
	if err := os.WriteFile(tsxPath, []byte(tsxContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), tsxPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.FileEntity.Language != "typescript" {
		t.Errorf("Expected language 'typescript', got '%s'", result.FileEntity.Language)
	}

	// Collect entity names by type
	entities := make(map[ast.CodeEntityType][]string)
	for _, e := range result.Entities {
		entities[e.Type] = append(entities[e.Type], e.Name)
	}

	// Check interface
	if !contains(entities[ast.TypeInterface], "Props") {
		t.Errorf("Expected interface 'Props', got %v", entities[ast.TypeInterface])
	}

	// Check functions
	funcs := entities[ast.TypeFunction]
	if !contains(funcs, "Greeting") {
		t.Errorf("Expected function 'Greeting', got %v", funcs)
	}
	if !contains(funcs, "TypedButton") {
		t.Errorf("Expected function 'TypedButton', got %v", funcs)
	}

	// Check class
	if !contains(entities[ast.TypeClass], "TypedComponent") {
		t.Errorf("Expected class 'TypedComponent', got %v", entities[ast.TypeClass])
	}
}

func TestParseDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create subdirectory structure
	srcDir := filepath.Join(dir, "src")
	nodeModules := filepath.Join(dir, "node_modules")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatalf("Failed to create node_modules dir: %v", err)
	}

	// Create source file
	srcContent := `export function hello() { return 'world'; }`
	if err := os.WriteFile(filepath.Join(srcDir, "index.ts"), []byte(srcContent), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Create file in node_modules (should be skipped)
	nodeContent := `export function excluded() {}`
	if err := os.WriteFile(filepath.Join(nodeModules, "lib.ts"), []byte(nodeContent), 0644); err != nil {
		t.Fatalf("Failed to write node_modules file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	results, err := parser.ParseDirectory(context.Background(), dir)
	if err != nil {
		t.Fatalf("ParseDirectory failed: %v", err)
	}

	// Should only have 1 result (node_modules should be excluded)
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Verify it's the src file
	if len(results) > 0 && !strings.Contains(results[0].Path, "index.ts") {
		t.Errorf("Expected index.ts, got %s", results[0].Path)
	}
}

func TestVisibility(t *testing.T) {
	dir := t.TempDir()

	content := `export function publicFunc() {}
function privateFunc() {}
export const PUBLIC_CONST = 1;
const PRIVATE_CONST = 2;

export class PublicClass {
    public publicMethod() {}
    private privateMethod() {}
    #reallyPrivate() {}
}

class PrivateClass {}
`

	path := filepath.Join(dir, "visibility.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	for _, e := range result.Entities {
		switch e.Name {
		case "publicFunc", "PUBLIC_CONST", "PublicClass":
			if e.Visibility != ast.VisibilityPublic {
				t.Errorf("Expected '%s' to be public, got %s", e.Name, e.Visibility)
			}
		case "privateFunc", "PRIVATE_CONST", "PrivateClass":
			if e.Visibility != ast.VisibilityPrivate {
				t.Errorf("Expected '%s' to be private, got %s", e.Name, e.Visibility)
			}
		case "privateMethod", "#reallyPrivate":
			if e.Visibility != ast.VisibilityPrivate {
				t.Errorf("Expected method '%s' to be private, got %s", e.Name, e.Visibility)
			}
		}
	}
}

func TestAsyncFunctions(t *testing.T) {
	dir := t.TempDir()

	content := `export async function asyncFunc() {
    return await Promise.resolve();
}

export const asyncArrow = async () => {
    return 'data';
};

export class AsyncClass {
    async asyncMethod() {
        return 'result';
    }
}
`

	path := filepath.Join(dir, "async.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Find async function
	var asyncFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "asyncFunc" && e.Type == ast.TypeFunction {
			asyncFunc = e
			break
		}
	}
	if asyncFunc == nil {
		t.Fatal("asyncFunc not found")
	}
	if !strings.Contains(asyncFunc.DocComment, "Async") {
		t.Errorf("Expected asyncFunc to have Async in DocComment, got '%s'", asyncFunc.DocComment)
	}

	// Find async arrow
	var asyncArrow *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "asyncArrow" && e.Type == ast.TypeFunction {
			asyncArrow = e
			break
		}
	}
	if asyncArrow == nil {
		t.Fatal("asyncArrow not found")
	}
	if !strings.Contains(asyncArrow.DocComment, "Async") {
		t.Errorf("Expected asyncArrow to have Async in DocComment, got '%s'", asyncArrow.DocComment)
	}
}

func TestInterfaceExtends(t *testing.T) {
	dir := t.TempDir()

	content := `export interface Base {
    id: string;
}

export interface Extended extends Base {
    name: string;
}

export interface Multiple extends Base, Extended {
    age: number;
}
`

	path := filepath.Join(dir, "interfaces.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Find Extended interface
	var extended *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "Extended" && e.Type == ast.TypeInterface {
			extended = e
			break
		}
	}
	if extended == nil {
		t.Fatal("Extended interface not found")
	}
	if len(extended.Extends) == 0 {
		t.Error("Expected Extended to extend Base")
	}

	// Find Multiple interface
	var multiple *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "Multiple" && e.Type == ast.TypeInterface {
			multiple = e
			break
		}
	}
	if multiple == nil {
		t.Fatal("Multiple interface not found")
	}
	if len(multiple.Extends) < 2 {
		t.Errorf("Expected Multiple to extend 2 interfaces, got %d", len(multiple.Extends))
	}
}

func TestEntityTriples(t *testing.T) {
	dir := t.TempDir()

	content := `export class MyClass extends BaseClass {
    myMethod() {}
}
`

	path := filepath.Join(dir, "triples.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Find class entity
	var classEntity *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeClass && e.Name == "MyClass" {
			classEntity = e
			break
		}
	}

	if classEntity == nil {
		t.Fatal("MyClass entity not found")
	}

	// Convert to triples
	triples := classEntity.Triples()

	// Verify we have triples
	if len(triples) == 0 {
		t.Error("Expected triples, got none")
	}

	// Check for language predicate
	hasLang := false
	for _, tr := range triples {
		if tr.Predicate == ast.CodeLanguage && tr.Object == "typescript" {
			hasLang = true
			break
		}
	}
	if !hasLang {
		t.Error("Expected CodeLanguage triple with 'typescript'")
	}

	// Check for extends predicate
	hasExtends := false
	for _, tr := range triples {
		if tr.Predicate == ast.CodeExtends {
			hasExtends = true
			break
		}
	}
	if !hasExtends {
		t.Error("Expected CodeExtends triple")
	}
}

func TestIsTargetFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"file.ts", true},
		{"file.tsx", true},
		{"file.js", true},
		{"file.jsx", true},
		{"file.mts", true},
		{"file.mjs", true},
		{"file.cts", true},
		{"file.cjs", true},
		{"file.go", false},
		{"file.py", false},
		{"file.txt", false},
		{"tsconfig.json", false},
	}

	for _, tc := range tests {
		result := IsTargetFile(tc.path)
		if result != tc.expected {
			t.Errorf("IsTargetFile(%q) = %v, expected %v", tc.path, result, tc.expected)
		}
	}
}

func TestPrivateFieldsAndMethods(t *testing.T) {
	dir := t.TempDir()

	content := `export class TestClass {
    #privateField: string;
    public publicField: number;

    #privateMethod() {
        return 'private';
    }

    public publicMethod() {
        return 'public';
    }

    private tsPrivate() {
        return 'ts private';
    }
}
`

	path := filepath.Join(dir, "private.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	for _, e := range result.Entities {
		if e.Type == ast.TypeMethod {
			switch e.Name {
			case "#privateMethod", "tsPrivate":
				if e.Visibility != ast.VisibilityPrivate {
					t.Errorf("Expected method '%s' to be private, got %s", e.Name, e.Visibility)
				}
			case "publicMethod":
				if e.Visibility != ast.VisibilityPublic {
					t.Errorf("Expected method '%s' to be public, got %s", e.Name, e.Visibility)
				}
			}
		}
	}
}

func TestFunctionSignatures(t *testing.T) {
	dir := t.TempDir()

	content := `export function typedFunc(name: string, age: number): User {
    return { name, age };
}

export const arrowTyped = (x: number, y: string): Promise<Result> => {
    return Promise.resolve({ x, y });
};

export class SignatureClass {
    method(param: CustomType): ReturnType {
        return {} as ReturnType;
    }
}
`

	path := filepath.Join(dir, "signatures.ts")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	parser := NewParser("test-org", "test-project", dir)
	result, err := parser.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Find typedFunc
	var typedFunc *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Name == "typedFunc" && e.Type == ast.TypeFunction {
			typedFunc = e
			break
		}
	}
	if typedFunc == nil {
		t.Fatal("typedFunc not found")
	}
	if len(typedFunc.Parameters) == 0 {
		t.Error("Expected typedFunc to have parameters")
	}
	if len(typedFunc.Returns) == 0 {
		t.Error("Expected typedFunc to have return type")
	}
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// containsImport checks if import path is in the slice
func containsImport(imports []string, path string) bool {
	for _, imp := range imports {
		if imp == path {
			return true
		}
	}
	return false
}
