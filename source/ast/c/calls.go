package c

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	cgrammar "github.com/smacker/go-tree-sitter/c"

	"github.com/c360studio/semsource/internal/gitboundary"
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

// defIndexEntry caches one file's set of DEFINED function names. Revalidation
// is two-tier: a cheap os.Stat (size+mtime) per lookup, with the full
// read+hash+re-parse only when the stat changed — the initial seed would
// otherwise re-read every candidate defining file once per ParseFile,
// O(files × distinct-callee-files) of redundant I/O for freshness that only
// matters under watch (review finding). Content hash remains the truth on
// change; the stat is only the change detector.
type defIndexEntry struct {
	hash    string
	size    int64
	modTime int64
	names   map[string]bool
}

// defSkipDirs matches handler.DefaultExcludedDirNames() exactly (pinned by
// TestDefSkipDirsMatchIngester): indexing a tree the ingester never walks
// would mint edges to entities that are never created, and skipping one it
// DOES walk (vendored C is deliberately ingested, handler/excludes.go) would
// silently unbind real definitions.
//
// Known limitation: per-source CONFIGURED excludes do not reach this parser
// (the registry factory carries no config), so a configured-excluded
// directory that uniquely defines a name can still yield an edge whose target
// entity the ingester never creates. That failure is a dangling edge — loud
// at graph-query resolution, the detectable direction — never a wrong real
// one.
var defSkipDirs = map[string]bool{".git": true, "node_modules": true}

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
			// Never prune the walk root itself: filepath.Walk hands it to the
			// callback first, and its BASE name (e.g. a checkout under
			// ".cache/") must not silently empty the whole index.
			if path == p.repoRoot {
				return nil
			}
			if defSkipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if gitboundary.IsBoundary(path) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".c":
		case ".h":
			// In a watch path that declares C++ ALONGSIDE C, the router
			// assigns headers to the C++ parser, whose entities carry the
			// cpp domain — a c-domain edge to a header-inline definition
			// would dangle. The parser cannot see the config, so the
			// corpus itself decides: any C++ source present → headers are
			// left out of the index (missing edges, never dangling ones).
			if p.corpusHasCPP() {
				return nil
			}
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

// corpusHasCPP reports whether the tree contains C++ sources, computed once
// per parser. Deterministic for a fixed corpus, so edge sets stay
// order-independent.
func (p *Parser) corpusHasCPP() bool {
	if p.hasCPP != nil {
		return *p.hasCPP
	}
	found := false
	_ = filepath.Walk(p.repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return filepath.SkipAll
		}
		if info.IsDir() {
			if path != p.repoRoot && (defSkipDirs[info.Name()] ||
				strings.HasPrefix(info.Name(), ".") || gitboundary.IsBoundary(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx":
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	p.hasCPP = &found
	return found
}

// refreshDefEntry (re)indexes one file's definition names. content may be nil,
// in which case the stat gate runs first and the file is read only on change.
func (p *Parser) refreshDefEntry(relPath string, content []byte) {
	abs := filepath.Join(p.repoRoot, relPath)
	if content == nil {
		st, err := os.Stat(abs)
		if err != nil {
			p.dropDefEntry(relPath)
			return
		}
		if cached, ok := p.defIndex[relPath]; ok &&
			cached.size == st.Size() && cached.modTime == st.ModTime().UnixNano() {
			return
		}
		content, err = os.ReadFile(abs)
		if err != nil {
			p.dropDefEntry(relPath)
			return
		}
	}
	hash := ast.ComputeHash(content)
	st, statErr := os.Stat(abs)
	var size, mtime int64
	if statErr == nil {
		size, mtime = st.Size(), st.ModTime().UnixNano()
	}
	if cached, ok := p.defIndex[relPath]; ok && cached.hash == hash {
		cached.size, cached.modTime = size, mtime
		p.defIndex[relPath] = cached
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
	p.defIndex[relPath] = defIndexEntry{hash: hash, size: size, modTime: mtime, names: names}
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
// candidates (stat-gated, once per ParseFile) so watch edits are honored.
// Zero or several candidates → inert. A deleted file's lingering entry can only
// widen a candidate set, which degrades toward fewer edges — the safe direction.
//
// Watch-window caveat (accepted): a NEWLY created file joins the index only at
// its own first parse, so calls resolved in the debounce window between
// creation and that parse can bind a name as unique when the new file just
// made it ambiguous. Bounded by the watcher's debounce; edges in files parsed
// after the new file's parse are correct, and re-deriving earlier files'
// edges is the pre-existing cross-file staleness class shared with every
// language pass.
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
