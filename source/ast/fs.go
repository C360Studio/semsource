package ast

import "os"

// FileExists reports whether path names an existing regular file. Shared by the
// per-language import/module resolvers (Java/TS/Python), which probe the source
// tree to map an imported name to its defining file.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
