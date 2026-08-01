package c

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// writeFile writes content into dir and returns the full path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return full
}

// parse parses one file written into a fresh repo root.
func parse(t *testing.T, name, content string) (*ast.ParseResult, string) {
	t.Helper()
	root := t.TempDir()
	path := writeFile(t, root, name, content)
	p := NewParser("acme", "proj", root)
	res, err := p.ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if res == nil {
		t.Fatal("ParseFile returned no result")
	}
	return res, root
}

// byName indexes entities by name for assertions.
func byName(res *ast.ParseResult) map[string]*ast.CodeEntity {
	out := map[string]*ast.CodeEntity{}
	for _, e := range res.Entities {
		out[e.Name] = e
	}
	return out
}

const sample = `
#include <stdio.h>
#include "local.h"

#define MAX_NODES 32
#define SQUARE(x) ((x)*(x))

struct Bare { int a; };
union Value { int i; float f; };
enum Mode { MODE_IDLE, MODE_RUN };

typedef struct PointTag { int x; int y; } Point;
typedef int MyInt;

static int counter = 0;
const char *g_name = "hi";

/** Sends a packet. */
int mesh_send(const char *msg, int len) { return 0; }

void proto_only(void);

struct Node *next_node(struct Node *n) { return n; }
`

// TestExtractsEveryDeclaredKind covers the kinds the spec requires C to yield.
func TestExtractsEveryDeclaredKind(t *testing.T) {
	res, _ := parse(t, "mesh.c", sample)
	got := byName(res)

	for name, wantType := range map[string]ast.CodeEntityType{
		"mesh_send":  ast.TypeFunction,
		"next_node":  ast.TypeFunction, // pointer return unwraps to the function
		"proto_only": ast.TypeFunction, // prototype, not a definition
		"Bare":       ast.TypeStruct,
		"Value":      ast.TypeStruct, // union recorded as a struct
		"Mode":       ast.TypeEnum,
		"PointTag":   ast.TypeStruct, // the struct tag
		"Point":      ast.TypeType,   // the typedef alias
		"MyInt":      ast.TypeType,
		"counter":    ast.TypeVar,
		"g_name":     ast.TypeVar,
		"MAX_NODES":  ast.TypeConst,
		"SQUARE":     ast.TypeConst,
	} {
		entity, ok := got[name]
		if !ok {
			t.Errorf("%s was not extracted", name)
			continue
		}
		if entity.Type != wantType {
			t.Errorf("%s has type %q, want %q", name, entity.Type, wantType)
		}
	}
}

// TestTypedefKeepsBothNames pins that `typedef struct Point {…} Point;` yields
// the tag and the alias. Both names are usable in C source, so indexing only
// one makes the other unsearchable.
func TestTypedefKeepsBothNames(t *testing.T) {
	res, _ := parse(t, "types.c", "typedef struct Point { int x; } PointAlias;")

	var tag, alias *ast.CodeEntity
	for _, e := range res.Entities {
		switch e.Name {
		case "Point":
			tag = e
		case "PointAlias":
			alias = e
		}
	}
	if tag == nil {
		t.Error("struct tag Point was not extracted")
	}
	if alias == nil {
		t.Error("typedef alias PointAlias was not extracted")
	}
	if tag != nil && alias != nil && tag.ID == alias.ID {
		t.Error("tag and alias collapsed onto one entity ID")
	}
}

// TestSameNamedStaticFunctionsInDifferentFilesDoNotCollide is the identity
// guarantee C needs most. C has no module system, so two translation units may
// each define a file-local function of the same name and they are different
// functions. If their IDs matched, one would silently overwrite the other in
// the graph.
func TestSameNamedStaticFunctionsInDifferentFilesDoNotCollide(t *testing.T) {
	root := t.TempDir()
	a := writeFile(t, root, "radio.c", "static int helper(int x) { return x; }")
	b := writeFile(t, root, "gps.c", "static int helper(int x) { return x * 2; }")

	p := NewParser("acme", "proj", root)
	resA, err := p.ParseFile(context.Background(), a)
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	resB, err := p.ParseFile(context.Background(), b)
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}

	idA, idB := byName(resA)["helper"], byName(resB)["helper"]
	if idA == nil || idB == nil {
		t.Fatal("helper missing from one of the files")
	}
	if idA.ID == idB.ID {
		t.Fatalf("two distinct functions share entity ID %s", idA.ID)
	}
}

// TestIdentityIsStableAcrossReparses pins that IDs are intrinsic: reparsing
// unchanged bytes must not move them, or every re-ingest would churn the graph.
func TestIdentityIsStableAcrossReparses(t *testing.T) {
	root := t.TempDir()
	path := writeFile(t, root, "mesh.c", sample)
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
		t.Fatalf("entity count moved between parses: %d then %d",
			len(first.Entities), len(second.Entities))
	}
	for i := range first.Entities {
		if first.Entities[i].ID != second.Entities[i].ID {
			t.Errorf("entity %d: ID %q became %q", i, first.Entities[i].ID, second.Entities[i].ID)
		}
	}
}

// TestEntityIDShape checks the six-part ID and that C claims its own domain
// rather than borrowing another language's.
func TestEntityIDShape(t *testing.T) {
	res, _ := parse(t, "mesh.c", sample)
	entity := byName(res)["mesh_send"]
	if entity == nil {
		t.Fatal("mesh_send missing")
	}
	// {org}.{platform}.{domain}.{system}.{type}.{instance}
	const wantPrefix = "acme.semsource.c.proj.function."
	if got := entity.ID; len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("entity ID %q does not start with %q", got, wantPrefix)
	}
	if entity.Language != "c" {
		t.Errorf("language is %q, want \"c\"", entity.Language)
	}
}

// TestFileEntityAndContainment checks the parent file entity is emitted and
// owns its children, which is what makes a file navigable in the graph.
func TestFileEntityAndContainment(t *testing.T) {
	res, _ := parse(t, "mesh.c", sample)
	if res.FileEntity == nil {
		t.Fatal("no file entity")
	}
	if len(res.FileEntity.Contains) == 0 {
		t.Error("file entity contains nothing")
	}
	for _, e := range res.Entities {
		if e == res.FileEntity {
			continue
		}
		if e.ContainedBy != res.FileEntity.ID {
			t.Errorf("%s is not contained by the file entity", e.Name)
		}
	}
}

// TestIncludesAreRecorded checks both include spellings reach the file entity.
func TestIncludesAreRecorded(t *testing.T) {
	res, _ := parse(t, "mesh.c", sample)
	want := map[string]bool{"stdio.h": false, "local.h": false}
	for _, inc := range res.Imports {
		if _, ok := want[inc]; ok {
			want[inc] = true
		}
	}
	for inc, found := range want {
		if !found {
			t.Errorf("include %q was not recorded (got %v)", inc, res.Imports)
		}
	}
}

// TestSignatureAndDocComment checks the fields retrieval ranks on are populated.
func TestSignatureAndDocComment(t *testing.T) {
	res, _ := parse(t, "mesh.c", sample)
	entity := byName(res)["mesh_send"]
	if entity == nil {
		t.Fatal("mesh_send missing")
	}
	if entity.Signature == "" {
		t.Error("function has no signature")
	}
	if entity.DocComment == "" {
		t.Error("function has no doc comment although one precedes it")
	}
}

// TestAnonymousSpecifierIsSkipped checks an unnamed struct produces nothing
// rather than an entity with an empty name, which could not be found or cited.
func TestAnonymousSpecifierIsSkipped(t *testing.T) {
	res, _ := parse(t, "anon.c", "struct { int a; } instance;")
	for _, e := range res.Entities {
		if e.Name == "" {
			t.Errorf("emitted an entity with no name: %s", e.ID)
		}
	}
}

// TestHeaderIsParsed checks a .h file is handled — the case that matters most,
// since a header-only C library declares its entire API there.
func TestHeaderIsParsed(t *testing.T) {
	res, _ := parse(t, "mavlink.h", `
#define MAVLINK_MSG_ID_HEARTBEAT 0
typedef struct __mavlink_heartbeat_t { uint8_t type; } mavlink_heartbeat_t;
uint16_t mavlink_msg_heartbeat_pack(uint8_t sysid, uint8_t compid);
`)
	got := byName(res)
	for _, name := range []string{
		"MAVLINK_MSG_ID_HEARTBEAT",
		"mavlink_heartbeat_t",
		"mavlink_msg_heartbeat_pack",
	} {
		if _, ok := got[name]; !ok {
			t.Errorf("%s was not extracted from the header", name)
		}
	}
}

// TestDoxygenCommentForms covers the conventions C actually uses. The shared
// Javadoc-style helper recognises only `/** */`, which would have left most of a
// firmware codebase's documentation unindexed — and doc comments are embedded
// and ranked, so that is retrieval quality, not cosmetics.
func TestDoxygenCommentForms(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"block /** */": {"/** Sends a packet. */\nint f(void);", "Sends a packet."},
		"block /*! */": {"/*! Sends a packet. */\nint f(void);", "Sends a packet."},
		"line ///":     {"/// Sends a packet.\nint f(void);", "Sends a packet."},
		"line //!":     {"//! Sends a packet.\nint f(void);", "Sends a packet."},
		"multi-line ///": {
			"/// Sends a packet.\n/// Blocks until the radio is idle.\nint f(void);",
			"Sends a packet.\nBlocks until the radio is idle.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			res, _ := parse(t, "doc.c", tc.src)
			entity := byName(res)["f"]
			if entity == nil {
				t.Fatal("f was not extracted")
			}
			if entity.DocComment != tc.want {
				t.Errorf("doc comment = %q, want %q", entity.DocComment, tc.want)
			}
		})
	}
}

// TestUnrelatedCommentIsNotAttributed keeps a comment separated by a blank line
// from being folded into the next symbol's documentation — that would describe
// one function using another's prose, which is worse than having none.
func TestUnrelatedCommentIsNotAttributed(t *testing.T) {
	res, _ := parse(t, "doc.c", "/// About the file, not the function.\n\n\nint f(void);")
	entity := byName(res)["f"]
	if entity == nil {
		t.Fatal("f was not extracted")
	}
	if entity.DocComment != "" {
		t.Errorf("a detached comment was attributed to f: %q", entity.DocComment)
	}
}
