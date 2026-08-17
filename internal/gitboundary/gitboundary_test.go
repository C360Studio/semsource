package gitboundary_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/internal/gitboundary"
)

func mk(t *testing.T, root string, rel string, isDir bool, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if isDir {
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsBoundary(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "gitdir/.git", true, "")                      // repo with .git directory
	mk(t, root, "gitlink/.git", false, "gitdir: elsewhere\n") // submodule-style gitlink file
	mk(t, root, "plain/sub", true, "")

	for dir, want := range map[string]bool{
		"gitdir":  true,
		"gitlink": true,
		"plain":   false,
	} {
		if got := gitboundary.IsBoundary(filepath.Join(root, dir)); got != want {
			t.Errorf("IsBoundary(%s) = %v, want %v", dir, got, want)
		}
	}
}

func TestUnder(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "sub/.git", false, "gitdir: elsewhere\n")
	mk(t, root, "sub/pkg/deep", true, "")
	mk(t, root, "plain/pkg", true, "")

	for path, want := range map[string]bool{
		"sub/file.go":          true,
		"sub/pkg/deep/file.go": true,
		"plain/pkg/file.go":    false,
		"file.go":              false,
	} {
		if got := gitboundary.Under(root, filepath.Join(root, path)); got != want {
			t.Errorf("Under(root, %s) = %v, want %v", path, got, want)
		}
	}
}
