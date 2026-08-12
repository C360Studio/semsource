package c

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	cgrammar "github.com/smacker/go-tree-sitter/c"

	"github.com/c360studio/semsource/source/ast"
)

// C call-graph extraction (code-call-graph, C delta — design D4 of
// call-graph-completeness). C has no imports to follow, so resolution is a
// corpus-wide name binding: a direct call `f(...)` edges to `f`'s DEFINITION
// when exactly one in-tree file defines it, and stays inert otherwise.
// Measured basis for the unique-binding rule (task 4.1, via this repo's own
// parser over redis): 1.41% of function names in src/ have definitions in more
// than one file, 1.80% with vendored deps — the colliding tail drops, the rest
// resolves, and no edge is ever guessed.
//
// The index counts function_definition nodes ONLY. Prototypes also produce
// TypeFunction entities (deliberately — header-only libraries), but a
// prototype is not a definition: indexing them would make every
// header-declared function self-collide and gut the pass.
//
// Emitting nothing (never a wrong edge): a callee that is any expression but a
// bare identifier (field/pointer calls), an identifier declared as a parameter
// or local of the enclosing function (a function-pointer call — the Go rule,
// #143), a name with zero or multiple in-tree definitions, and anything
// reached through a macro (invisible post-lex, so self-skipping).

// defIndexEntry caches one file's set of DEFINED function names, validated by
// content hash on lookup so an edited file is re-read rather than served stale
// (the java memberCache pattern).
type defIndexEntry struct {
	hash  string
	names map[string]bool
}

// defSkipDirs mirrors the ingester's exclusions; indexing a tree the corpus
// never sees would report collisions nobody can fix.
var defSkipDirs = map[string]bool{".git": true, "vendor": true, "node_modules": true}

// buildDefIndex walks the whole tree once and indexes every .c/.h file's
// function definitions. Eager on first use, deliberately: filling the index
// per-ParseFile would make an edge's existence depend on parse ORDER (early
// files could not see later definitions), and a call graph that differs by
// traversal order is nondeterministic. The one-time cost is a second
// tree-sitter pass over the C files; redis's 134-file src indexes in well
// under a second.
func (p *Parser) buildDefIndex() {
	if p.defIndex != nil {
		return
	}
	p.defIndex = make(map[string]defIndexEntry)
	p.defNameFiles = make(map[string]map[string]bool)
	_ = filepath.Walk(p.repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if defSkipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".c", ".h":
		default:
			return nil
		}
		rel, rerr := filepath.Rel(p.repoRoot, path)
		if rerr != nil {
			return nil
		}
		p.refreshDefEntry(rel, nil)
		return nil
	})
}

// refreshDefEntry (re)indexes one file's definition names. content may be nil,
// in which case the file is read from disk. A hash match keeps the cached set.
func (p *Parser) refreshDefEntry(relPath string, content []byte) {
	if content == nil {
		var err error
		content, err = os.ReadFile(filepath.Join(p.repoRoot, relPath))
		if err != nil {
			p.dropDefEntry(relPath)
			return
		}
	}
	hash := ast.ComputeHash(content)
	if cached, ok := p.defIndex[relPath]; ok && cached.hash == hash {
		return
	}
	names := make(map[string]bool)
	mp := sitter.NewParser()
	mp.SetLanguage(cgrammar.GetLanguage())
	if tree, err := mp.ParseCtx(context.Background(), nil, content); err == nil {
		collectDefinitionNames(tree.RootNode(), content, names)
		tree.Close()
	}
	p.dropDefEntry(relPath)
	p.defIndex[relPath] = defIndexEntry{hash: hash, names: names}
	for name := range names {
		if p.defNameFiles[name] == nil {
			p.defNameFiles[name] = make(map[string]bool)
		}
		p.defNameFiles[name][relPath] = true
	}
}

// dropDefEntry removes a file's contribution to the name index.
func (p *Parser) dropDefEntry(relPath string) {
	old, ok := p.defIndex[relPath]
	if !ok {
		return
	}
	for name := range old.names {
		delete(p.defNameFiles[name], relPath)
		if len(p.defNameFiles[name]) == 0 {
			delete(p.defNameFiles, name)
		}
	}
	delete(p.defIndex, relPath)
}

// collectDefinitionNames gathers function_definition names, descending into
// preprocessor conditionals the same way entity extraction does.
func collectDefinitionNames(node *sitter.Node, content []byte, names map[string]bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "function_definition":
			if decl := functionDeclarator(child.ChildByFieldName("declarator")); decl != nil {
				if name := declaratorName(decl.ChildByFieldName("declarator"), content); name != "" {
					names[name] = true
				}
			}
		case "preproc_ifdef", "preproc_if", "preproc_else", "preproc_elif":
			collectDefinitionNames(child, content, names)
		}
	}
}

// resolveDefinition returns the single in-tree file defining name, revalidating
// candidates by content hash (once per ParseFile) so watch edits are honored.
// Zero or several candidates → inert. A deleted file's lingering entry can only
// widen a candidate set, which degrades toward fewer edges — the safe direction.
func (p *Parser) resolveDefinition(name string) (string, bool) {
	p.buildDefIndex()
	var candidates []string
	for rel := range p.defNameFiles[name] {
		candidates = append(candidates, rel)
	}
	for _, rel := range candidates {
		if !p.revalidated[rel] {
			p.refreshDefEntry(rel, nil)
			p.revalidated[rel] = true
		}
	}
	files := p.defNameFiles[name]
	if len(files) != 1 {
		return "", false
	}
	for rel := range files {
		return rel, true
	}
	return "", false
}

// extractCalls walks a function definition's body and returns the entity IDs
// of the callees it can confirm, deduped and in source order.
func (p *Parser) extractCalls(fnNode *sitter.Node, content []byte) []string {
	body := fnNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	locals := localValueNames(fnNode, content)
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call_expression" {
			if id := p.callTargetID(n, content, locals); id != "" && !seen[id] {
				seen[id] = true
				calls = append(calls, id)
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return calls
}

// callTargetID resolves one call site, or "" (inert).
func (p *Parser) callTargetID(call *sitter.Node, content []byte, locals map[string]bool) string {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "identifier" {
		// field_expression (s->op()), pointer deref, parenthesized — all
		// function-pointer shapes needing value tracking; inert.
		return ""
	}
	name := fn.Content(content)
	if locals[name] {
		// Declared as a parameter or local: a function-pointer call. The
		// pointee is unknowable statically — inert, same rule as Go (#143).
		return ""
	}
	defRel, ok := p.resolveDefinition(name)
	if !ok {
		return ""
	}
	return ast.NewCodeEntity(p.org, "c", p.project, ast.TypeFunction, name, defRel).ID
}

// localValueNames collects the names a function's parameters and local
// declarations bind, so a call through any of them is recognized as a
// function-pointer call.
func localValueNames(fnNode *sitter.Node, content []byte) map[string]bool {
	names := make(map[string]bool)
	if decl := functionDeclarator(fnNode.ChildByFieldName("declarator")); decl != nil {
		if params := decl.ChildByFieldName("parameters"); params != nil {
			for i := 0; i < int(params.NamedChildCount()); i++ {
				param := params.NamedChild(i)
				if param.Type() != "parameter_declaration" {
					continue
				}
				if name := declaratorName(param.ChildByFieldName("declarator"), content); name != "" {
					names[name] = true
				}
			}
		}
	}
	body := fnNode.ChildByFieldName("body")
	if body == nil {
		return names
	}
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "declaration" {
			for i := 0; i < int(n.NamedChildCount()); i++ {
				child := n.NamedChild(i)
				switch child.Type() {
				case "init_declarator", "pointer_declarator", "array_declarator",
					"parenthesized_declarator", "function_declarator", "identifier":
					if name := declaratorName(child, content); name != "" {
						names[name] = true
					}
				}
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return names
}
