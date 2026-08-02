package java

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// writeTree materialises {relPath: source} under a fresh root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// parseTree parses every file in a tree and returns all entities. Files are
// parsed in sorted order so a test's result does not depend on map iteration.
func parseTree(t *testing.T, files map[string]string) []*ast.CodeEntity {
	t.Helper()
	root := writeTree(t, files)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return parseRels(t, root, rels)
}

func parseRels(t *testing.T, root string, rels []string) []*ast.CodeEntity {
	t.Helper()
	var all []*ast.CodeEntity
	p := NewParser("acme", "test", root)
	for _, rel := range rels {
		res, err := p.ParseFile(context.Background(), filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		all = append(all, res.Entities...)
	}
	return all
}

// entityNamed returns the single entity with the given name, failing when it is
// absent or ambiguous — so a test can never silently assert against the wrong one.
func entityNamed(t *testing.T, ents []*ast.CodeEntity, name string) *ast.CodeEntity {
	t.Helper()
	var found []*ast.CodeEntity
	for _, e := range ents {
		if e.Name == name {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one entity named %q, got %d", name, len(found))
	}
	return found[0]
}

func callsOf(t *testing.T, ents []*ast.CodeEntity, name string) []string {
	t.Helper()
	return entityNamed(t, ents, name).Calls
}

func hasCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

func assertCalls(t *testing.T, ents []*ast.CodeEntity, caller, calleeName string) {
	t.Helper()
	callee := entityNamed(t, ents, calleeName)
	calls := callsOf(t, ents, caller)
	if !hasCall(calls, callee.ID) {
		t.Errorf("%s calls = %v\n  want it to contain %s definition ID %q", caller, calls, calleeName, callee.ID)
	}
}

// assertCallsExactly pins a caller's entire edge set. Asserting the exact set —
// rather than merely "no edge to a real entity" — is what makes the inert cases
// real guards: emitting a DANGLING target (an ID no entity has) is just as wrong
// as emitting the wrong real one, and only an exact assertion catches it.
func assertCallsExactly(t *testing.T, ents []*ast.CodeEntity, caller string, want ...string) {
	t.Helper()
	got := append([]string(nil), callsOf(t, ents, caller)...)
	sort.Strings(got)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if len(got) != len(sorted) {
		t.Fatalf("%s calls = %v, want exactly %v", caller, got, sorted)
	}
	for i := range got {
		if got[i] != sorted[i] {
			t.Fatalf("%s calls = %v, want exactly %v", caller, got, sorted)
		}
	}
}

// assertNoCalls is the inert case: not one edge of any kind.
func assertNoCalls(t *testing.T, ents []*ast.CodeEntity, caller string) {
	t.Helper()
	assertCallsExactly(t, ents, caller)
}

// --- receiver typing -------------------------------------------------------

func TestCallThroughFieldResolvesToDefinition(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  private Repo repo;\n" +
			"  public void run() { repo.load(); }\n}\n",
	})
	assertCalls(t, ents, "run", "load")
}

func TestCallThroughParameterAndLocal(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java":   "package a;\npublic class Repo { public void load() {} }\n",
		"a/Helper.java": "package a;\npublic class Helper { public void go() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run(Repo repo) { Helper h = new Helper(); repo.load(); h.go(); }\n}\n",
	})
	assertCalls(t, ents, "run", "load")
	assertCalls(t, ents, "run", "go")
}

func TestGenericAndArrayReceiverTypesResolve(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  private Repo[] repos;\n" +
			"  public void run() { Repo r = repos[0]; r.load(); }\n}\n",
	})
	assertCalls(t, ents, "run", "load")
}

func TestConflictingReceiverTypeIsInert(t *testing.T) {
	// `x` is declared twice with different types in one method. Flattened block
	// scopes cannot tell which is live at the call, so the receiver is dropped.
	ents := parseTree(t, map[string]string{
		"a/Repo.java":  "package a;\npublic class Repo { public void load() {} }\n",
		"a/Other.java": "package a;\npublic class Other { public void load() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run() {\n" +
			"    { Repo x = null; x.load(); }\n" +
			"    { Other x = null; x.load(); }\n" +
			"  }\n}\n",
	})
	assertNoCalls(t, ents, "run")
}

// --- intra-class and static ------------------------------------------------

func TestUnqualifiedAndThisCallsResolve(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void a() { b(); this.c(); }\n" +
			"  public void b() {}\n" +
			"  public void c() {}\n}\n",
	})
	assertCalls(t, ents, "a", "b")
	assertCalls(t, ents, "a", "c")
}

func TestStaticCallToInTreeType(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/b/Util.java": "package a.b;\npublic class Util { public static String slug(String s) { return s; } }\n",
		"x/Svc.java": "package x;\nimport a.b.Util;\npublic class Svc {\n" +
			"  public void run() { Util.slug(\"q\"); }\n}\n",
	})
	assertCalls(t, ents, "run", "slug")
}

func TestDuplicateCalleeEmittedOnce(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void a() { helper(); helper(); helper(); }\n" +
			"  public void helper() {}\n}\n",
	})
	helper := entityNamed(t, ents, "helper")
	n := 0
	for _, c := range callsOf(t, ents, "a") {
		if c == helper.ID {
			n++
		}
	}
	if n != 1 {
		t.Errorf("helper appears %d times in calls, want exactly 1", n)
	}
}

// --- ancestor walk ---------------------------------------------------------

func TestInheritedSuperclassMethodResolvesToDeclarer(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Base.java": "package a;\npublic class Base { public void start() {} }\n",
		"a/Impl.java": "package a;\npublic class Impl extends Base {}\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run(Impl x) { x.start(); }\n}\n",
	})
	// The edge must target Base.start — the declaration — not an Impl.start that
	// does not exist.
	assertCalls(t, ents, "run", "start")
}

func TestInterfaceMethodResolvesToDeclaredType(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/IModule.java": "package a;\npublic interface IModule { void init(); }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run(IModule m) { m.init(); }\n}\n",
	})
	assertCalls(t, ents, "run", "init")
}

func TestAmbiguousAncestorDeclarersAreDropped(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/I1.java": "package a;\npublic interface I1 { void run(); }\n",
		"a/I2.java": "package a;\npublic interface I2 { void run(); }\n",
		"a/C.java":  "package a;\npublic class C implements I1, I2 {}\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void go(C c) { c.run(); }\n}\n",
	})
	assertNoCalls(t, ents, "go")
}

func TestCyclicHierarchyTerminates(t *testing.T) {
	// Not legal Java, but a malformed tree must not hang the parser.
	ents := parseTree(t, map[string]string{
		"a/A.java": "package a;\npublic class A extends B {}\n",
		"a/B.java": "package a;\npublic class B extends A {}\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void go(A a) { a.missing(); }\n}\n",
	})
	assertNoCalls(t, ents, "go")
}

func TestAncestorChainLeavingCorpusIsInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Impl.java": "package a;\nimport org.third.Party;\npublic class Impl extends Party {}\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void go(Impl i) { i.somethingInherited(); }\n}\n",
	})
	assertNoCalls(t, ents, "go")
}

// --- constructors ----------------------------------------------------------

func TestConstructorCallResolvesToConstructorEntity(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java": "package a;\npublic class Repo { public Repo(String path) {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run() { new Repo(\"/tmp\"); }\n}\n",
	})
	// The Repo constructor entity is named for its class.
	repoCtor := ""
	for _, e := range ents {
		if e.Name == "Repo" && e.Type == ast.TypeMethod {
			repoCtor = e.ID
		}
	}
	if repoCtor == "" {
		t.Fatal("no Repo constructor entity was extracted")
	}
	if calls := callsOf(t, ents, "run"); !hasCall(calls, repoCtor) {
		t.Errorf("run calls = %v, want constructor %q", calls, repoCtor)
	}
}

func TestDefaultConstructorEmitsNoEdge(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Plain.java": "package a;\npublic class Plain { public void x() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run() { new Plain(); }\n}\n",
	})
	assertNoCalls(t, ents, "run")
}

// --- fail-inert boundary ---------------------------------------------------

func TestThirdPartyReceiverStaysExternal(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Svc.java": "package a;\nimport org.slf4j.Logger;\npublic class Svc {\n" +
			"  private Logger logger;\n" +
			"  public void run() { logger.info(\"x\"); }\n}\n",
	})
	calls := callsOf(t, ents, "run")
	if !hasCall(calls, "external:org.slf4j.Logger.info") {
		t.Errorf("run calls = %v, want external:org.slf4j.Logger.info", calls)
	}
}

func TestChainedSuperAndVarReceiversAreInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"a/Base.java": "package a;\npublic class Base { public void start() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc extends Base {\n" +
			"  public Repo getRepo() { return null; }\n" +
			"  public void run() {\n" +
			"    getRepo().load();\n" +
			"    super.start();\n" +
			"    var r = getRepo(); r.load();\n" +
			"  }\n}\n",
	})
	// getRepo() itself is a legitimate unqualified call and is the ONLY edge that
	// may appear; load()/start() reached through a chained, super, or var
	// receiver must contribute nothing.
	getRepo := entityNamed(t, ents, "getRepo")
	assertCallsExactly(t, ents, "run", getRepo.ID)
}

func TestAmbiguousSimpleNameIsDropped(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Config.java": "package a;\npublic class Config { public void load() {} }\n",
		"b/Config.java": "package b;\npublic class Config { public void load() {} }\n",
		"c/Svc.java": "package c;\npublic class Svc {\n" +
			"  public void run(Config cfg) { cfg.load(); }\n}\n",
	})
	assertNoCalls(t, ents, "run")
}

func TestResolvedTypeWithoutTheMethodIsInert(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  private Repo repo;\n" +
			"  public void run() { repo.nonexistent(); }\n}\n",
	})
	assertNoCalls(t, ents, "run")
}

// TestEnumMembersAreNotCallTargets guards the parity between what parser.go
// extracts and what call resolution is willing to name: extractEnum creates no
// member entities, so an enum method must never become an edge target.
func TestEnumMembersAreNotCallTargets(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"a/E.java": "package a;\npublic enum E { A;\n  public void go() {}\n}\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  public void run(E e) { e.go(); }\n}\n",
	})
	assertNoCalls(t, ents, "run")
}

// --- multi-module resolution ----------------------------------------------

func TestCrossModuleCallResolves(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"lib/core/src/main/java/a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"app/src/main/java/x/Svc.java": "package x;\nimport a.Repo;\npublic class Svc {\n" +
			"  public void run(Repo r) { r.load(); }\n}\n",
	})
	assertCalls(t, ents, "run", "load")
}

func TestTestSourceRootReachesMainSourceRoot(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"app/src/main/java/a/Repo.java": "package a;\npublic class Repo { public void load() {} }\n",
		"app/src/test/java/a/RepoTest.java": "package a;\nimport a.Repo;\npublic class RepoTest {\n" +
			"  public void check(Repo r) { r.load(); }\n}\n",
	})
	assertCalls(t, ents, "check", "load")
}

func TestSameFQNInTwoModulesIsAmbiguousAndDropped(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"m1/src/main/java/a/Dup.java": "package a;\npublic class Dup { public void run() {} }\n",
		"m2/src/main/java/a/Dup.java": "package a;\npublic class Dup { public void run() {} }\n",
		"m3/src/main/java/x/Svc.java": "package x;\nimport a.Dup;\npublic class Svc {\n" +
			"  public void go(Dup d) { d.run(); }\n}\n",
	})
	assertNoCalls(t, ents, "go")
}

// --- determinism and watch safety -----------------------------------------

func TestCallResolutionIsParseOrderIndependent(t *testing.T) {
	files := map[string]string{
		"a/Repo.java":   "package a;\npublic class Repo { public void load() {} }\n",
		"a/Helper.java": "package a;\npublic class Helper { public void go() {} }\n",
		"a/Svc.java": "package a;\npublic class Svc {\n" +
			"  private Repo repo;\n" +
			"  public void run(Helper h) { repo.load(); h.go(); }\n}\n",
	}
	root := writeTree(t, files)
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	forward := parseRels(t, root, rels)
	reversed := make([]string, len(rels))
	for i, rel := range rels {
		reversed[len(rels)-1-i] = rel
	}
	backward := parseRels(t, root, reversed)

	collect := func(ents []*ast.CodeEntity) map[string][]string {
		out := map[string][]string{}
		for _, e := range ents {
			if len(e.Calls) > 0 {
				sorted := append([]string(nil), e.Calls...)
				sort.Strings(sorted)
				out[e.ID] = sorted
			}
		}
		return out
	}
	a, b := collect(forward), collect(backward)
	if len(a) == 0 {
		t.Fatal("no call edges produced; the test would pass vacuously")
	}
	if len(a) != len(b) {
		t.Fatalf("edge-bearing entities differ: %d vs %d", len(a), len(b))
	}
	for id, calls := range a {
		other, ok := b[id]
		if !ok {
			t.Errorf("entity %q has calls in one order only", id)
			continue
		}
		if len(calls) != len(other) {
			t.Errorf("entity %q: %v vs %v", id, calls, other)
			continue
		}
		for i := range calls {
			if calls[i] != other[i] {
				t.Errorf("entity %q: %v vs %v", id, calls, other)
				break
			}
		}
	}
}

// TestEditedCalleeIsObservedOnReparse is the guard on the member cache: it is
// keyed by content hash, so adding a method to the callee must be picked up by a
// re-parse with the SAME parser instance.
func TestEditedCalleeIsObservedOnReparse(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/B.java": "package a;\npublic class B { }\n",
		"a/A.java": "package a;\npublic class A {\n" +
			"  private B b;\n" +
			"  public void go() { b.run(); }\n}\n",
	})
	p := NewParser("acme", "test", root)

	first, err := p.ParseFile(context.Background(), filepath.Join(root, "a/A.java"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range first.Entities {
		if e.Name == "go" && len(e.Calls) != 0 {
			t.Fatalf("before the edit, go should have no call edges, got %v", e.Calls)
		}
	}

	// B gains the method A was already calling.
	if err := os.WriteFile(filepath.Join(root, "a/B.java"),
		[]byte("package a;\npublic class B { public void run() {} }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bRes, err := p.ParseFile(context.Background(), filepath.Join(root, "a/B.java"))
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	for _, e := range bRes.Entities {
		if e.Name == "run" {
			runID = e.ID
		}
	}
	if runID == "" {
		t.Fatal("B.run was not extracted after the edit")
	}

	second, err := p.ParseFile(context.Background(), filepath.Join(root, "a/A.java"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range second.Entities {
		if e.Name != "go" {
			continue
		}
		if !hasCall(e.Calls, runID) {
			t.Errorf("after the edit, go calls = %v, want %q — the member cache served a stale view", e.Calls, runID)
		}
		return
	}
	t.Fatal("go entity missing on re-parse")
}

// TestAncestorSupersBindByTheAncestorsOwnImports pins that the ancestor walk
// resolves a superclass name using the imports of the file that WROTE it, not
// the file being parsed. Both files use the simple name `Target`, bound to
// different types — only the ancestor's own binding declares the method, so
// resolving with the caller's imports silently loses the edge.
func TestAncestorSupersBindByTheAncestorsOwnImports(t *testing.T) {
	ents := parseTree(t, map[string]string{
		"deep/Target.java":  "package deep;\npublic class Target { public void ping() {} }\n",
		"other/Target.java": "package other;\npublic class Target { }\n",
		"mid/Base.java":     "package mid;\nimport deep.Target;\npublic class Base extends Target {}\n",
		"app/Svc.java": "package app;\nimport mid.Base;\nimport other.Target;\n" +
			"public class Svc {\n  public void run(Base b) { b.ping(); }\n}\n",
	})
	// `ping` is declared only by deep.Target, reachable only through Base's own
	// import binding.
	assertCalls(t, ents, "run", "ping")
}
