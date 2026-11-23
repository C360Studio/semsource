package cpp

import (
	"testing"

	"github.com/c360studio/semsource/source/ast"
)

// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestCppPrototypeAndBranchesFold(t *testing.T) {
	res := parse(t, "console.cpp", `static int helper(int x);

class Console {
public:
    void move_cursor(int x);
};

#if defined(_WIN32)
void Console::move_cursor(int x) { (void)x; }
#else
void Console::move_cursor(int x) { (void)x; }
#endif

static int helper(int x) { return x + 1; }
`)
	for _, tc := range []struct {
		name string
		typ  ast.CodeEntityType
		line int
	}{
		{"helper", ast.TypeFunction, 14},   // the definition, not the prototype
		{"move_cursor", ast.TypeMethod, 9}, // the first definition, not the in-class declaration
	} {
		var got []*ast.CodeEntity
		for _, e := range res.Entities {
			if e.Name == tc.name && e.Type == tc.typ {
				got = append(got, e)
			}
		}
		if len(got) != 1 || got[0].StartLine != tc.line {
			lines := []int{}
			for _, e := range got {
				lines = append(lines, e.StartLine)
			}
			t.Errorf("%s: want one entity at line %d, got lines %v", tc.name, tc.line, lines)
		}
	}
}
