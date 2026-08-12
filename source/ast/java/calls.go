package java

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/c360studio/semsource/source/ast"
)

// Java call-graph extraction (code-call-graph, Java delta). A call resolves to
// the entity ID of its callee's DEFINITION so `code.relationship.calls` edges
// connect instead of dangling. Resolution is driven entirely by DECLARATIONS —
// Java requires them, so a receiver's type is knowable without any inference —
// and it FAILS INERT: an edge is emitted only once the resolved target file is
// confirmed to declare a method of that name.
//
// Emitting nothing (never a wrong edge): a chained call (`a().b()`), `super.`,
// a `var`-declared receiver, a receiver name declared with conflicting types in
// one method, a type name that cannot be bound to exactly one in-tree file, and
// a resolved target whose class and ancestors declare no such method.
//
// Receiver tables include INHERITED fields: the enclosing class's own fields
// plus every visible (non-private) ancestor field through the same supers walk
// and cap, nearest declaration winning and same-depth type conflicts dropped
// (#141 — the OSH `rootPerm` gap, where the field lives on a superclass in
// another file).

// ancestorDepthCap bounds the extends/implements walk. Java hierarchies are far
// shallower than this; the cap exists so a malformed or cyclic tree terminates.
const ancestorDepthCap = 12

// classMembers records, for one type declaration, exactly the member entities
// parser.go creates for it — nothing more. That parity is load-bearing: an enum
// or record body yields no member entities there, so naming one here would point
// a call edge at an entity that does not exist.
type classMembers struct {
	name string
	// methodScope is the scope chain entities declared in this type are built
	// with — the enclosing chain plus this type's own name, matching the
	// childScope extractClass/extractInterface pass to their members.
	methodScope []string
	methods     map[string]bool
	hasCtor     bool
	supers      []string // extends + implements, as written
	// fields maps a class body's field names to declared types, and
	// privateFields marks the subset invisible to subclasses. Carried here so
	// an INHERITED field can type a receiver: `rootPerm.m()` in a subclass of
	// the class declaring `public final ModulePermissions rootPerm` must
	// resolve like an own field (the OSH P01 gap, #141). Interface constants
	// stay excluded, same as the own-class table.
	fields        map[string]string
	privateFields map[string]bool
}

// classRef pairs a resolved class with the file that defines it, which is what
// an entity ID needs.
type classRef struct {
	cm  *classMembers
	rel string
}

// callScope is the per-method context call resolution runs against.
type callScope struct {
	relPath    string
	self       classRef
	vars       map[string]string // receiver name -> declared type
	conflicted map[string]bool   // names declared with more than one type (inert)
}

// collectFileMembers indexes every type declaration in a file by simple name and
// by scope chain. The by-name index resolves receivers; the by-scope index tells
// a method being extracted which class encloses it.
func (p *Parser) collectFileMembers(root *sitter.Node, content []byte) (map[string][]*classMembers, map[string]*classMembers) {
	byName := make(map[string][]*classMembers)
	byScope := make(map[string]*classMembers)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		p.collectTypeMembers(root.NamedChild(i), content, nil, byName, byScope)
	}
	return byName, byScope
}

// collectTypeMembers records one type declaration and recurses into the nested
// types that parser.go also extracts. The recursion mirrors extractClassBody
// exactly — a class body yields nested class/interface/enum entities, an
// interface body yields only methods — so the two views cannot diverge.
func (p *Parser) collectTypeMembers(node *sitter.Node, content []byte, scope []string,
	byName map[string][]*classMembers, byScope map[string]*classMembers) {
	switch node.Type() {
	case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
	default:
		return
	}
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := string(content[nameNode.StartByte():nameNode.EndByte()])

	cm := &classMembers{
		name:        name,
		methodScope: extendScope(scope, name),
		methods:     make(map[string]bool),
		supers:      p.superNames(node, content),
	}
	byName[name] = append(byName[name], cm)
	byScope[strings.Join(cm.methodScope, ".")] = cm

	body := node.ChildByFieldName("body")
	if body == nil {
		return
	}
	switch node.Type() {
	case "class_declaration":
		cm.fields, cm.privateFields = p.collectMemberFieldTypes(body, content)
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			switch child.Type() {
			case "method_declaration":
				if mn := child.ChildByFieldName("name"); mn != nil {
					cm.methods[string(content[mn.StartByte():mn.EndByte()])] = true
				}
			case "constructor_declaration":
				cm.hasCtor = true
			case "class_declaration", "interface_declaration", "enum_declaration":
				p.collectTypeMembers(child, content, cm.methodScope, byName, byScope)
			}
		}
	case "interface_declaration":
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			if child.Type() != "method_declaration" {
				continue
			}
			if mn := child.ChildByFieldName("name"); mn != nil {
				cm.methods[string(content[mn.StartByte():mn.EndByte()])] = true
			}
		}
	}
	// enum_declaration / record_declaration deliberately expose no members:
	// extractEnum and extractRecord create the type entity and stop, so a call
	// to an enum method has no entity to target and must stay inert.
}

// superNames returns a type's extends and implements targets as written.
func (p *Parser) superNames(node *sitter.Node, content []byte) []string {
	var supers []string
	if superclass := node.ChildByFieldName("superclass"); superclass != nil {
		if name := p.extractTypeReference(superclass, content); name != "" {
			supers = append(supers, name)
		}
	}
	if extends := node.ChildByFieldName("extends"); extends != nil {
		for i := 0; i < int(extends.NamedChildCount()); i++ {
			child := extends.NamedChild(i)
			if child.Type() == "type_list" {
				for j := 0; j < int(child.NamedChildCount()); j++ {
					if name := p.extractTypeReference(child.NamedChild(j), content); name != "" {
						supers = append(supers, name)
					}
				}
				continue
			}
			if name := p.extractTypeReference(child, content); name != "" {
				supers = append(supers, name)
			}
		}
	}
	return append(supers, p.extractInterfaceNames(node, content)...)
}

// fileMembers returns another file's type index.
//
// The cache is VALIDATED BY CONTENT, not scoped to one ParseFile: every lookup
// re-reads the file and compares its hash, and only a hash match reuses the
// parsed member set. That keeps the watch guarantee exact — an edited file
// always yields its new members, since edited content cannot hash equal — while
// letting the expensive part (the tree-sitter parse) be shared across the whole
// run. Re-parsing per ParseFile instead cost 7x wall-clock on a 1,932-file tree,
// because each file re-parsed every file it references.
func (p *Parser) fileMembers(relPath string) map[string][]*classMembers {
	if relPath == p.selfRel && p.selfMembers != nil {
		return p.selfMembers
	}
	content, err := os.ReadFile(filepath.Join(p.repoRoot, relPath))
	if err != nil {
		return nil
	}
	hash := ast.ComputeHash(content)
	if cached, ok := p.memberCache[relPath]; ok && cached.hash == hash {
		return cached.members
	}
	members := make(map[string][]*classMembers)
	imports := make(map[string]string)
	mp := sitter.NewParser()
	mp.SetLanguage(java.GetLanguage())
	if tree, terr := mp.ParseCtx(context.Background(), nil, content); terr == nil {
		members, _ = p.collectFileMembers(tree.RootNode(), content)
		imports = extractImportMap(tree.RootNode(), content)
		tree.Close()
	}
	if p.memberCache == nil {
		p.memberCache = make(map[string]cachedMembers)
	}
	p.memberCache[relPath] = cachedMembers{hash: hash, members: members, imports: imports}
	return members
}

// importsFor returns the import bindings of the file a name was written in. For
// the file being parsed that is the live per-ParseFile map; for any other file it
// is the one cached beside its members.
func (p *Parser) importsFor(relPath string) map[string]string {
	if relPath == p.selfRel {
		return p.importMap
	}
	if cached, ok := p.memberCache[relPath]; ok {
		return cached.imports
	}
	p.fileMembers(relPath) // populates the cache
	return p.memberCache[relPath].imports
}

// cachedMembers pairs a file's parsed type index with the content hash it was
// built from, so a stale entry is detected rather than served. It also carries
// that file's OWN import map: an ancestor walk resolves names as they were
// written in the ancestor's file, where a simple name may bind to a different
// fully-qualified type than it does in the file being parsed.
type cachedMembers struct {
	hash    string
	members map[string][]*classMembers
	imports map[string]string
}

// uniqueMember returns the single class a simple name denotes in one file, or nil
// when the file declares more than one (ambiguous — dropped, never guessed).
func uniqueMember(index map[string][]*classMembers, name string) *classMembers {
	if defs := index[name]; len(defs) == 1 {
		return defs[0]
	}
	return nil
}

// resolveClass resolves a type name, as written in fromRelPath, to the class it
// denotes. Returns the class and its file, or an `external:` payload when the
// name binds to an import that leaves the source tree, or nothing at all.
func (p *Parser) resolveClass(typeName, fromRelPath string) (cm *classMembers, rel, external string) {
	// A busy method resolves the same type at every call site through it. The
	// memo is keyed by the referrer too, because an ancestor walk resolves names
	// as written in ANOTHER file, and it lives only for this ParseFile.
	key := typeName + "\x00" + fromRelPath
	if hit, ok := p.classMemo[key]; ok {
		return hit.cm, hit.rel, hit.external
	}
	cm, rel, external = p.resolveClassUncached(typeName, fromRelPath)
	if p.classMemo == nil {
		p.classMemo = make(map[string]classResolution)
	}
	p.classMemo[key] = classResolution{cm: cm, rel: rel, external: external}
	return cm, rel, external
}

// classResolution is one memoized type-name resolution.
type classResolution struct {
	cm       *classMembers
	rel      string
	external string
}

func (p *Parser) resolveClassUncached(typeName, fromRelPath string) (cm *classMembers, rel, external string) {
	typeName = strings.TrimSpace(typeName)
	// `var` carries no declared type, and a builtin's methods are not in-tree.
	if typeName == "" || typeName == "var" || isBuiltinType(typeName) {
		return nil, "", ""
	}
	if strings.Contains(typeName, ".") {
		// An already-qualified name can still name an in-tree file; probing is
		// exact (the file must exist), so it is safe to try before giving up.
		if probe, ok := p.fqnToRelPath(typeName, fromRelPath); ok {
			if m := uniqueMember(p.fileMembers(probe), simpleTypeName(typeName)); m != nil {
				return m, probe, ""
			}
		}
		return nil, "", typeName
	}
	// A type declared in the REFERRER's own file cannot be shadowed by an import,
	// so that file's declarations bind first. fromRelPath is the file the name was
	// written in, which during an ancestor walk is not the file being parsed.
	if m := uniqueMember(p.fileMembers(fromRelPath), typeName); m != nil {
		return m, fromRelPath, ""
	}
	defRel, ext, ok := p.resolveJavaTypeIn(typeName, fromRelPath, p.importsFor(fromRelPath))
	if !ok {
		// An import that matched several in-tree source roots is ambiguous, not
		// external: naming it `external:` would misreport an in-repo type as a
		// third-party one. It emits nothing instead.
		if ext != "" && p.fqnIsAmbiguous(ext) {
			return nil, "", ""
		}
		return nil, "", ext
	}
	if m := uniqueMember(p.fileMembers(defRel), typeName); m != nil {
		return m, defRel, ""
	}
	return nil, "", ""
}

// simpleTypeName strips a qualified name down to its last segment.
func simpleTypeName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// declaringClass finds the class that actually declares method, starting at the
// receiver's own class and walking extends/implements breadth-first to the
// NEAREST declaring ancestor. It reports failure when nothing declares the
// method, when an ancestor name cannot be bound, or when more than one ancestor
// at the same depth declares it — a tie is dropped rather than picked.
func (p *Parser) declaringClass(start classRef, method string) (classRef, bool) {
	if start.cm == nil || method == "" {
		return classRef{}, false
	}
	if start.cm.methods[method] {
		return start, true
	}
	visited := map[string]bool{classKey(start): true}
	level := []classRef{start}
	for depth := 0; depth < ancestorDepthCap && len(level) > 0; depth++ {
		var next []classRef
		for _, cr := range level {
			for _, super := range cr.cm.supers {
				scm, srel, _ := p.resolveClass(super, cr.rel)
				if scm == nil {
					continue
				}
				ref := classRef{cm: scm, rel: srel}
				if visited[classKey(ref)] {
					continue
				}
				visited[classKey(ref)] = true
				next = append(next, ref)
			}
		}
		var found []classRef
		for _, cr := range next {
			if cr.cm.methods[method] {
				found = append(found, cr)
			}
		}
		if len(found) == 1 {
			return found[0], true
		}
		if len(found) > 1 {
			return classRef{}, false // ambiguous inheritance — drop, never choose
		}
		level = next
	}
	return classRef{}, false
}

func classKey(cr classRef) string {
	return cr.rel + "#" + strings.Join(cr.cm.methodScope, ".")
}

// receiverTypes builds the declaration-derived type table a method's call sites
// resolve against: the enclosing class's fields, then this method's parameters,
// then every local declaration in its body. Block scopes are deliberately
// FLATTENED; the safety comes instead from marking any name declared with more
// than one distinct type as conflicted, which makes it inert. Measured on OSH
// Core, that costs 0.5% of call sites and removes the whole class of mis-typed
// receivers a flattened table would otherwise admit.
func (p *Parser) receiverTypes(node *sitter.Node, content []byte) (map[string]string, map[string]bool) {
	vars := make(map[string]string, len(p.classFields))
	conflicted := make(map[string]bool)
	for name, typeName := range p.classFields {
		vars[name] = typeName
	}
	note := func(name, typeName string) {
		if name == "" || typeName == "" {
			return
		}
		if prev, ok := vars[name]; ok && prev != typeName {
			conflicted[name] = true
		}
		vars[name] = typeName
	}
	if params := node.ChildByFieldName("parameters"); params != nil {
		p.collectParamTypes(params, content, note)
	}
	if body := node.ChildByFieldName("body"); body != nil {
		p.collectLocalTypes(body, content, note)
	}
	return vars, conflicted
}

// collectParamTypes records a parameter list's declared types.
func (p *Parser) collectParamTypes(params *sitter.Node, content []byte, note func(name, typeName string)) {
	for i := 0; i < int(params.NamedChildCount()); i++ {
		param := params.NamedChild(i)
		if param.Type() != "formal_parameter" && param.Type() != "spread_parameter" {
			continue
		}
		typeName := p.extractTypeReference(param.ChildByFieldName("type"), content)
		if nameNode := param.ChildByFieldName("name"); nameNode != nil {
			note(string(content[nameNode.StartByte():nameNode.EndByte()]), typeName)
			continue
		}
		// A spread parameter carries its name in a variable_declarator.
		for j := 0; j < int(param.NamedChildCount()); j++ {
			decl := param.NamedChild(j)
			if decl.Type() != "variable_declarator" {
				continue
			}
			if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
				note(string(content[nameNode.StartByte():nameNode.EndByte()]), typeName)
			}
		}
	}
}

// collectLocalTypes records every declared local in a body: plain declarations,
// enhanced-for variables, catch parameters, and try-with-resources resources.
func (p *Parser) collectLocalTypes(node *sitter.Node, content []byte, note func(name, typeName string)) {
	switch node.Type() {
	case "local_variable_declaration":
		typeName := p.extractTypeReference(node.ChildByFieldName("type"), content)
		for i := 0; i < int(node.NamedChildCount()); i++ {
			decl := node.NamedChild(i)
			if decl.Type() != "variable_declarator" {
				continue
			}
			if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
				note(string(content[nameNode.StartByte():nameNode.EndByte()]), typeName)
			}
		}
	case "enhanced_for_statement", "resource":
		typeName := p.extractTypeReference(node.ChildByFieldName("type"), content)
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			note(string(content[nameNode.StartByte():nameNode.EndByte()]), typeName)
		}
	case "catch_formal_parameter":
		typeName := p.catchType(node.ChildByFieldName("type"), content)
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			note(string(content[nameNode.StartByte():nameNode.EndByte()]), typeName)
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		p.collectLocalTypes(node.NamedChild(i), content, note)
	}
}

// catchType reads a catch clause's type. A multi-catch (`catch (A | B e)`) wraps
// several types; it has no single declared type, so it resolves to nothing.
func (p *Parser) catchType(typeNode *sitter.Node, content []byte) string {
	if typeNode == nil {
		return ""
	}
	if typeNode.Type() == "catch_type" {
		if typeNode.NamedChildCount() != 1 {
			return ""
		}
		return p.extractTypeReference(typeNode.NamedChild(0), content)
	}
	return p.extractTypeReference(typeNode, content)
}

// collectFieldTypes maps a class body's field names to their declared types.
func (p *Parser) collectFieldTypes(body *sitter.Node, content []byte) map[string]string {
	fields, _ := p.collectMemberFieldTypes(body, content)
	return fields
}

// collectMemberFieldTypes additionally reports which fields are private —
// invisible to subclasses, so the inherited-field merge must skip them while
// the own-class table keeps them.
func (p *Parser) collectMemberFieldTypes(body *sitter.Node, content []byte) (map[string]string, map[string]bool) {
	fields := make(map[string]string)
	private := make(map[string]bool)
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() != "field_declaration" {
			continue
		}
		typeName := p.extractTypeReference(child.ChildByFieldName("type"), content)
		if typeName == "" {
			continue
		}
		isPrivate := fieldIsPrivate(child, content)
		for j := 0; j < int(child.NamedChildCount()); j++ {
			decl := child.NamedChild(j)
			if decl.Type() != "variable_declarator" {
				continue
			}
			if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
				name := string(content[nameNode.StartByte():nameNode.EndByte()])
				fields[name] = typeName
				if isPrivate {
					private[name] = true
				}
			}
		}
	}
	return fields, private
}

// fieldIsPrivate reports whether a field_declaration carries the `private`
// modifier.
func fieldIsPrivate(field *sitter.Node, content []byte) bool {
	for i := 0; i < int(field.NamedChildCount()); i++ {
		child := field.NamedChild(i)
		if child.Type() != "modifiers" {
			continue
		}
		if strings.Contains(" "+string(content[child.StartByte():child.EndByte()])+" ", " private ") {
			return true
		}
	}
	return false
}

// classFieldsWithInherited merges a class's own field table with the visible
// (non-private) fields of its ancestors, walked breadth-first through the same
// resolution and cap as declaringClass. Nearest declaration wins — an own
// field shadows an inherited one, a depth-1 field shadows depth-2 — and a name
// two SAME-depth ancestors declare with different types is dropped from the
// table entirely (inert, never guessed). This is what lets an inherited-field
// receiver resolve: the OSH P01 gap (#141), where `rootPerm` lives on the
// superclass in another file.
func (p *Parser) classFieldsWithInherited(start classRef, own map[string]string) map[string]string {
	merged := make(map[string]string, len(own))
	for name, typeName := range own {
		merged[name] = typeName
	}
	if start.cm == nil {
		return merged
	}
	visited := map[string]bool{classKey(start): true}
	level := []classRef{start}
	for depth := 0; depth < ancestorDepthCap && len(level) > 0; depth++ {
		var next []classRef
		for _, cr := range level {
			for _, super := range cr.cm.supers {
				scm, srel, _ := p.resolveClass(super, cr.rel)
				if scm == nil {
					continue
				}
				ref := classRef{cm: scm, rel: srel}
				if visited[classKey(ref)] {
					continue
				}
				visited[classKey(ref)] = true
				next = append(next, ref)
			}
		}
		// Merge one depth level at a time so nearer declarations always win
		// and same-depth type conflicts are detectable.
		levelSeen := make(map[string]string)
		levelDrop := make(map[string]bool)
		for _, cr := range next {
			for name, typeName := range cr.cm.fields {
				if cr.cm.privateFields[name] {
					continue
				}
				if _, shadowed := merged[name]; shadowed {
					continue
				}
				if prev, ok := levelSeen[name]; ok && prev != typeName {
					levelDrop[name] = true
					continue
				}
				levelSeen[name] = typeName
			}
		}
		for name, typeName := range levelSeen {
			if !levelDrop[name] {
				merged[name] = typeName
			}
		}
		level = next
	}
	return merged
}

// extractCalls walks a method or constructor body and returns the entity IDs of
// the callees it can confirm, deduped and in source order.
func (p *Parser) extractCalls(body *sitter.Node, content []byte, cs callScope) []string {
	if body == nil {
		return nil
	}
	var calls []string
	seen := make(map[string]bool)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		calls = append(calls, id)
	}
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "method_invocation":
			add(p.callTargetID(n, content, cs))
		case "object_creation_expression":
			add(p.constructorTargetID(n, content, cs))
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return calls
}

// callTargetID resolves one invocation to a callee entity ID, or "" when the
// target cannot be confirmed.
func (p *Parser) callTargetID(node *sitter.Node, content []byte, cs callScope) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	method := string(content[nameNode.StartByte():nameNode.EndByte()])
	obj := node.ChildByFieldName("object")

	// Unqualified and `this.` calls target the enclosing class, or the nearest
	// ancestor that declares the method.
	if obj == nil || obj.Type() == "this" {
		return p.methodIDIn(cs.self, method)
	}
	// Anything that is not a plain name receiver — a chained call, `super`, a
	// field access, a literal, a cast — stays inert: typing it would need
	// expression inference, and a guess here is exactly the wrong edge.
	if obj.Type() != "identifier" {
		return ""
	}
	name := string(content[obj.StartByte():obj.EndByte()])
	if cs.conflicted[name] {
		return ""
	}
	if declType, ok := cs.vars[name]; ok {
		cm, rel, external := p.resolveClass(declType, cs.relPath)
		if external != "" {
			return "external:" + external + "." + method
		}
		return p.methodIDIn(classRef{cm: cm, rel: rel}, method)
	}
	// Not a declared variable: treat the name as a type, i.e. a static call.
	cm, rel, external := p.resolveClass(name, cs.relPath)
	if external != "" {
		return "external:" + external + "." + method
	}
	return p.methodIDIn(classRef{cm: cm, rel: rel}, method)
}

// methodIDIn builds the callee ID once a class is confirmed to declare it,
// through the same NewScopedCodeEntity path the definition uses so the edge
// byte-matches the definition instead of dangling.
func (p *Parser) methodIDIn(start classRef, method string) string {
	decl, ok := p.declaringClass(start, method)
	if !ok {
		return ""
	}
	return ast.NewScopedCodeEntity(p.org, "java", p.project, ast.TypeMethod,
		decl.cm.methodScope, method, decl.rel).ID
}

// constructorTargetID resolves `new Type(...)` to Type's constructor entity. A
// class with only the implicit default constructor has no constructor entity, so
// it emits nothing rather than a dangling target.
func (p *Parser) constructorTargetID(node *sitter.Node, content []byte, cs callScope) string {
	typeName := p.extractTypeReference(node.ChildByFieldName("type"), content)
	cm, rel, _ := p.resolveClass(typeName, cs.relPath)
	if cm == nil || !cm.hasCtor {
		return ""
	}
	return ast.NewScopedCodeEntity(p.org, "java", p.project, ast.TypeMethod,
		cm.methodScope, cm.name, rel).ID
}

// callScopeFor builds the resolution context for a member declared at scope.
func (p *Parser) callScopeFor(node *sitter.Node, content []byte, filePath string, scope []string) callScope {
	vars, conflicted := p.receiverTypes(node, content)
	return callScope{
		relPath:    filePath,
		self:       classRef{cm: p.selfByScope[strings.Join(scope, ".")], rel: p.selfRel},
		vars:       vars,
		conflicted: conflicted,
	}
}
