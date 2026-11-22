package golang

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestParseFile_BlankAndInitDeclarationsAreNotEntities pins #332 and owner
// ruling Q4: blank declarations and package init functions name no symbol, so
// they are not emitted, while their named neighbours are.
func TestParseFile_BlankAndInitDeclarationsAreNotEntities(t *testing.T) {
	tmpDir := t.TempDir()
	code := `package example

import (
	"io"
	"os"
)

var _ io.Reader = (*os.File)(nil)
var _ io.Writer = (*os.File)(nil)

const (
	_ = iota
	A
	B
)

var x, _ = 1, 2

func _() {}

func init() {}

func init() {}

type T struct{}

func (T) init() {}
`
	path := filepath.Join(tmpDir, "blanks.go")
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := NewParser(testAuthority("acme"), "test", tmpDir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range res.Entities {
		if e == res.FileEntity {
			continue
		}
		names = append(names, string(e.Type)+":"+e.Name)
	}
	sort.Strings(names)
	want := []string{"const:A", "const:B", "method:init", "struct:T", "var:x"}
	if len(names) != len(want) {
		t.Fatalf("entities = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entities = %v, want %v", names, want)
		}
	}
}
