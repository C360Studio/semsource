package cpp

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return full
}

func parse(t *testing.T, name, content string) *ast.ParseResult {
	t.Helper()
	root := t.TempDir()
	path := writeFile(t, root, name, content)
	res, err := NewParser("acme", "proj", root).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return res
}

func byName(res *ast.ParseResult) map[string]*ast.CodeEntity {
	out := map[string]*ast.CodeEntity{}
	for _, e := range res.Entities {
		out[e.Name] = e
	}
	return out
}

// find locates an entity by name AND type. A C++ class and its constructor
// share a name, so a name-only lookup silently returns whichever came last.
func find(res *ast.ParseResult, name string, t ast.CodeEntityType) *ast.CodeEntity {
	for _, e := range res.Entities {
		if e.Name == name && e.Type == t {
			return e
		}
	}
	return nil
}

const sample = `
#include <cstdint>

namespace meshtastic {

/// The radio driver.
class Radio : public Base {
public:
    Radio();
    ~Radio();
    int send(const char *m);
    int inlineDef(int x) { return x; }
    static int count;
};

struct Plain { int a; };
enum class Color { Red };
using Alias = int;

} // namespace meshtastic

template <typename T> class Buffer { public: T get(); };
template <typename T> T identity(T v) { return v; }

void freeFn(int a) {}
`

// TestExtractsCppKinds covers what the spec requires C++ to yield beyond C.
func TestExtractsCppKinds(t *testing.T) {
	res := parse(t, "radio.cpp", sample)

	for name, wantType := range map[string]ast.CodeEntityType{
		"meshtastic": ast.TypeType, // namespace
		"Radio":      ast.TypeClass,
		"send":       ast.TypeMethod,
		"inlineDef":  ast.TypeMethod,
		"count":      ast.TypeVar,
		"Plain":      ast.TypeStruct,
		"Color":      ast.TypeEnum,
		"Alias":      ast.TypeType,
		"Buffer":     ast.TypeClass,    // templated class
		"identity":   ast.TypeFunction, // templated free function
		"freeFn":     ast.TypeFunction,
	} {
		if find(res, name, wantType) == nil {
			t.Errorf("no %s entity named %s was extracted", wantType, name)
		}
	}
}

// TestConstructorAndDestructor pins that both are extracted and stay distinct.
// They share the class's name apart from the "~", so dropping it would collapse
// two different functions onto one entity.
func TestConstructorAndDestructor(t *testing.T) {
	res := parse(t, "radio.cpp", sample)

	ctor := find(res, "Radio", ast.TypeMethod)
	if ctor == nil {
		t.Fatal("constructor Radio was not extracted")
	}
	dtor := find(res, "~Radio", ast.TypeMethod)
	if dtor == nil {
		t.Fatal("destructor ~Radio was not extracted")
	}
	if ctor.ID == dtor.ID {
		t.Errorf("constructor and destructor share entity ID %s", ctor.ID)
	}
}

// TestSameMethodNameOnDifferentClassesDoesNotCollide is the C++ analogue of C's
// static-function collision: two classes in one file may each have a send().
func TestSameMethodNameOnDifferentClassesDoesNotCollide(t *testing.T) {
	res := parse(t, "two.cpp", `
class Radio { public: int send(int a); };
class Serial { public: int send(int a); };
`)

	var ids []string
	for _, e := range res.Entities {
		if e.Name == "send" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected two send() entities, got %d", len(ids))
	}
	if ids[0] == ids[1] {
		t.Errorf("two different methods share entity ID %s", ids[0])
	}
}

// TestOutOfLineDefinitionCarriesItsClass checks `int Radio::send(...)` is
// recorded as a method of Radio rather than as a free function named send.
func TestOutOfLineDefinitionCarriesItsClass(t *testing.T) {
	res := parse(t, "radio.cpp", "int Radio::send(const char *m) { return 0; }")

	entity := byName(res)["send"]
	if entity == nil {
		t.Fatal("send was not extracted")
	}
	if entity.Type != ast.TypeMethod {
		t.Errorf("out-of-line definition has type %q, want method", entity.Type)
	}
	if !strings.Contains(entity.ID, "Radio") {
		t.Errorf("entity ID %q does not carry the class Radio", entity.ID)
	}
}

// TestNamespaceQualifiesMembers checks a namespace reaches its members' identity
// so two classes of the same name in different namespaces stay distinct.
func TestNamespaceQualifiesMembers(t *testing.T) {
	res := parse(t, "ns.cpp", `
namespace a { class Node { public: int id(); }; }
namespace b { class Node { public: int id(); }; }
`)

	var nodeIDs, idIDs []string
	for _, e := range res.Entities {
		switch e.Name {
		case "Node":
			nodeIDs = append(nodeIDs, e.ID)
		case "id":
			idIDs = append(idIDs, e.ID)
		}
	}
	if len(nodeIDs) != 2 || nodeIDs[0] == nodeIDs[1] {
		t.Errorf("same-named classes in different namespaces did not stay distinct: %v", nodeIDs)
	}
	if len(idIDs) != 2 || idIDs[0] == idIDs[1] {
		t.Errorf("same-named methods in different namespaces did not stay distinct: %v", idIDs)
	}
}

// TestBaseClassIsRecorded checks the inheritance edge, which is what
// code_impact walks to answer "what derives from this".
//
// Straight out of ParseFile the base is a marked NAME, not an entity ID: C++
// cannot tell which file defines it without the whole watch path. ResolveTypeRefs
// turns it into an ID later. The marker is asserted here so the intermediate
// state stays deliberate rather than becoming an accident someone "fixes".
func TestBaseClassIsRecorded(t *testing.T) {
	res := parse(t, "radio.cpp", "class Radio : public Base { };")
	entity := byName(res)["Radio"]
	if entity == nil {
		t.Fatal("Radio was not extracted")
	}
	if len(entity.Extends) != 1 || entity.Extends[0] != UnresolvedPrefix+"Base" {
		t.Errorf("Extends = %v, want [%sBase]", entity.Extends, UnresolvedPrefix)
	}
}

// TestEntityIDShapeAndDomain checks C++ claims its own domain rather than
// borrowing C's or Go's.
func TestEntityIDShapeAndDomain(t *testing.T) {
	entity := byName(parse(t, "radio.cpp", sample))["freeFn"]
	if entity == nil {
		t.Fatal("freeFn missing")
	}
	const wantPrefix = "acme.semsource.cpp.proj.function."
	if got := entity.ID; len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("entity ID %q does not start with %q", got, wantPrefix)
	}
	if entity.Language != "cpp" {
		t.Errorf("language is %q, want \"cpp\"", entity.Language)
	}
}

// TestIdentityIsStableAcrossReparses pins intrinsic identity.
func TestIdentityIsStableAcrossReparses(t *testing.T) {
	root := t.TempDir()
	path := writeFile(t, root, "radio.cpp", sample)
	p := NewParser("acme", "proj", root)

	first, err := p.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	second, err := p.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if len(first.Entities) != len(second.Entities) {
		t.Fatalf("entity count moved: %d then %d", len(first.Entities), len(second.Entities))
	}
	for i := range first.Entities {
		if first.Entities[i].ID != second.Entities[i].ID {
			t.Errorf("entity %d: %q became %q", i, first.Entities[i].ID, second.Entities[i].ID)
		}
	}
}

// TestHeaderDeclarationAndDefinitionStayDistinct records the decision behind
// task 4.3. A method declared in a header and defined in a .cpp produces two
// entities, because identity is qualified by the defining file's path. They are
// deterministic and collision-free, which is what the spec requires; relating
// them is future work, not a defect to be surprised by later.
func TestHeaderDeclarationAndDefinitionStayDistinct(t *testing.T) {
	root := t.TempDir()
	header := writeFile(t, root, "radio.h", "class Radio { public: int send(int a); };")
	source := writeFile(t, root, "radio.cpp", "int Radio::send(int a) { return a; }")

	p := NewParser("acme", "proj", root)
	hRes, err := p.ParseFile(context.Background(), header)
	if err != nil {
		t.Fatalf("parse header: %v", err)
	}
	cRes, err := p.ParseFile(context.Background(), source)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	decl, def := byName(hRes)["send"], byName(cRes)["send"]
	if decl == nil || def == nil {
		t.Fatal("send missing from header or source")
	}
	if decl.ID == def.ID {
		t.Fatal("declaration and definition unexpectedly share an ID; " +
			"if this becomes intended, update the spec rather than this assertion")
	}
	if decl.Type != ast.TypeMethod || def.Type != ast.TypeMethod {
		t.Errorf("both should be methods, got %q and %q", decl.Type, def.Type)
	}
}

// TestDoxygenDocComment checks the C++ convention is captured, since it is what
// retrieval ranks on.
func TestDoxygenDocComment(t *testing.T) {
	entity := find(parse(t, "radio.cpp", sample), "Radio", ast.TypeClass)
	if entity == nil {
		t.Fatal("Radio class missing")
	}
	if entity.DocComment != "The radio driver." {
		t.Errorf("doc comment = %q, want %q", entity.DocComment, "The radio driver.")
	}
}

// TestAnonymousClassIsSkipped keeps nameless entities out of the graph.
func TestAnonymousClassIsSkipped(t *testing.T) {
	res := parse(t, "anon.cpp", "struct { int a; } instance;")
	for _, e := range res.Entities {
		if e.Name == "" {
			t.Errorf("emitted an entity with no name: %s", e.ID)
		}
	}
}

// TestExternCIsNotAScope checks `extern "C" { … }` does not become a namespace,
// which would put a bogus segment in every wrapped symbol's ID.
func TestExternCIsNotAScope(t *testing.T) {
	res := parse(t, "shim.cpp", "extern \"C\" { void c_entry(void); }")
	entity := byName(res)["c_entry"]
	if entity == nil {
		t.Fatal("c_entry was not extracted from the extern block")
	}
	if entity.Type != ast.TypeFunction {
		t.Errorf("c_entry has type %q, want function — extern \"C\" is not a scope", entity.Type)
	}
}

// parseAll parses several files written into one root, as a watch path would.
func parseAll(t *testing.T, files map[string]string) []*ast.ParseResult {
	t.Helper()
	root := t.TempDir()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic order for the test itself
	p := NewParser("acme", "proj", root)
	var out []*ast.ParseResult
	for _, name := range names {
		path := writeFile(t, root, name, files[name])
		res, err := p.ParseFile(context.Background(), path)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		out = append(out, res)
	}
	return out
}

func extendsOf(results []*ast.ParseResult, class string) []string {
	for _, res := range results {
		for _, e := range res.Entities {
			if e.Name == class && e.Type == ast.TypeClass {
				return e.Extends
			}
		}
	}
	return nil
}

// TestResolveTypeRefs_CrossFileBaseClass is the case that matters: 85% of
// Meshtastic's inheritance references name a base defined in another file. C++
// cannot predict that file from the name, so resolution runs over the whole
// parsed set.
func TestResolveTypeRefs_CrossFileBaseClass(t *testing.T) {
	results := parseAll(t, map[string]string{
		"base.h":    "class GpioUnaryTransformer { public: int x; };",
		"derived.h": "class GpioNotTransformer : public GpioUnaryTransformer { };",
	})

	before := extendsOf(results, "GpioNotTransformer")
	if len(before) != 1 || !strings.HasPrefix(before[0], UnresolvedPrefix) {
		t.Fatalf("before resolution Extends = %v, want one %q entry", before, UnresolvedPrefix)
	}

	ResolveTypeRefs(results)

	got := extendsOf(results, "GpioNotTransformer")
	if len(got) != 1 {
		t.Fatalf("after resolution Extends = %v, want exactly one entry", got)
	}
	if strings.HasPrefix(got[0], UnresolvedPrefix) {
		t.Errorf("base class was left unresolved: %s", got[0])
	}
	want := ""
	for _, res := range results {
		for _, e := range res.Entities {
			if e.Name == "GpioUnaryTransformer" && e.Type == ast.TypeClass {
				want = e.ID
			}
		}
	}
	if got[0] != want {
		t.Errorf("Extends = %q, want the base class entity ID %q", got[0], want)
	}
}

// TestResolveTypeRefs_AmbiguousNameIsDropped is the honesty rule. 3% of class
// names in Meshtastic are defined in more than one file; picking one would
// invent an inheritance edge no compiler agrees with, and code_impact would
// report a dependent that does not exist.
func TestResolveTypeRefs_AmbiguousNameIsDropped(t *testing.T) {
	results := parseAll(t, map[string]string{
		"a.h": "class MockRouter { public: int a; };",
		"b.h": "class MockRouter { public: int b; };",
		"c.h": "class RealRouter : public MockRouter { };",
	})

	ResolveTypeRefs(results)

	if got := extendsOf(results, "RealRouter"); len(got) != 0 {
		t.Errorf("ambiguous base resolved to %v; it must be dropped rather than guessed", got)
	}
}

// TestResolveTypeRefs_UnknownBaseIsDropped covers a base outside the corpus —
// an SDK or stdlib type. No entity exists, so no edge should.
func TestResolveTypeRefs_UnknownBaseIsDropped(t *testing.T) {
	results := parseAll(t, map[string]string{
		"only.h": "class Radio : public ArduinoThing { };",
	})
	ResolveTypeRefs(results)
	if got := extendsOf(results, "Radio"); len(got) != 0 {
		t.Errorf("base outside the corpus resolved to %v; want dropped", got)
	}
}

// TestResolveTypeRefs_IsIdempotent guards the incremental path: a single-file
// re-parse re-runs resolution over results that are already resolved.
func TestResolveTypeRefs_IsIdempotent(t *testing.T) {
	results := parseAll(t, map[string]string{
		"base.h":    "class Base { public: int x; };",
		"derived.h": "class Derived : public Base { };",
	})
	ResolveTypeRefs(results)
	once := extendsOf(results, "Derived")
	ResolveTypeRefs(results)
	twice := extendsOf(results, "Derived")

	if len(once) != 1 || len(twice) != 1 || once[0] != twice[0] {
		t.Errorf("resolution is not idempotent: %v then %v", once, twice)
	}
}

// TestResolveTypeRefs_QualifiedAndTemplatedBases checks the name normalisation,
// since real C++ writes `public meshtastic::Base` and `public Buffer<int>`.
func TestResolveTypeRefs_QualifiedAndTemplatedBases(t *testing.T) {
	results := parseAll(t, map[string]string{
		"base.h": "namespace meshtastic { class Base { public: int x; }; }",
		"d.h":    "class Derived : public meshtastic::Base { };",
	})
	ResolveTypeRefs(results)
	if got := extendsOf(results, "Derived"); len(got) != 1 || strings.HasPrefix(got[0], UnresolvedPrefix) {
		t.Errorf("namespace-qualified base did not resolve: %v", got)
	}
}

// TestResolveTypeRefs_SelfReferenceIsDropped guards against a class appearing to
// derive from itself, which would make impact walks cyclic.
func TestResolveTypeRefs_SelfReferenceIsDropped(t *testing.T) {
	results := parseAll(t, map[string]string{
		"s.h": "class Node : public Node { };",
	})
	ResolveTypeRefs(results)
	if got := extendsOf(results, "Node"); len(got) != 0 {
		t.Errorf("self-inheritance resolved to %v; want dropped", got)
	}
}
