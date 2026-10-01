package astsource

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/seedproof"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

func TestReactivationSeedReportsSkippedASTFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "current.py"), []byte("def current():\n    return 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "unreadable.py")); err != nil {
		t.Fatal(err)
	}
	parser, err := semsourceast.DefaultRegistry.CreateParser("python", entityid.Authority{Org: "acme", Platform: "test"}, "proj", root)
	if err != nil {
		t.Fatal(err)
	}
	pw := &pathWatcher{root: root, parsers: map[string]semsourceast.FileParser{"python": parser}, routes: map[string]string{".py": "python"}, excludes: map[string]bool{}}
	c := &Component{logger: slog.Default()}
	ctx, proof := seedproof.Begin(t.Context())
	results, err := c.parseDirectory(ctx, pw)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("valid sibling results = %d", len(results))
	}
	if proof.Err() == nil {
		t.Fatal("partial AST enumeration falsely proved complete")
	}
}
