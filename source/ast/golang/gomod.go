package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// modOrigin records the nearest enclosing Go module for a package directory:
// the module path declared by the closest go.mod at or above the directory
// (bounded by repoRoot), and the repo-relative directory holding it. A zero
// modulePath means no module encloses the directory, and no in-repo import
// mapping is possible.
type modOrigin struct {
	sig        string
	modulePath string
	moduleDir  string
}

// moduleOrigin resolves and caches the nearest enclosing module for a
// repo-relative directory (go-callgraph-recall D2). The cache is validated by a
// size+mtime signature over go.mod at EVERY level of the walk — present or
// absent — so adding, removing, or editing any go.mod between the directory and
// the repo root invalidates the entry during watch instead of serving a stale
// module mapping.
func (p *Parser) moduleOrigin(dirRel string) modOrigin {
	// A relPath that fell back to absolute (filepath.Rel failure) cannot be
	// mapped into the repo; refuse rather than walk the host filesystem.
	if dirRel == "" || filepath.IsAbs(dirRel) {
		return modOrigin{}
	}

	var sig strings.Builder
	nearest := ""
	for d := dirRel; ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(p.repoRoot, d, "go.mod")); err == nil {
			fmt.Fprintf(&sig, "%s:%d:%d;", d, info.Size(), info.ModTime().UnixNano())
			if nearest == "" {
				nearest = d
			}
		} else {
			fmt.Fprintf(&sig, "%s:absent;", d)
		}
		if d == "." {
			break
		}
	}

	if p.modCache == nil {
		p.modCache = make(map[string]modOrigin)
	}
	if cached, ok := p.modCache[dirRel]; ok && cached.sig == sig.String() {
		return cached
	}

	origin := modOrigin{sig: sig.String()}
	if nearest != "" {
		if mp := readModulePath(filepath.Join(p.repoRoot, nearest, "go.mod")); mp != "" {
			origin.modulePath = mp
			origin.moduleDir = nearest
		}
	}
	p.modCache[dirRel] = origin
	return origin
}

// inRepoImportDir maps an import path to a repo-relative package directory when
// the path lies inside the calling file's nearest enclosing module. Anything
// outside that module — the standard library, third-party modules, a nested
// module with its own path — reports false and stays an external reference.
// Nested modules are safe by construction: their import path either carries the
// enclosing module's prefix and maps to the true directory, or it doesn't match
// at all.
func (p *Parser) inRepoImportDir(importPath, fromRelPath string) (string, bool) {
	origin := p.moduleOrigin(filepath.Dir(fromRelPath))
	if origin.modulePath == "" {
		return "", false
	}
	if importPath == origin.modulePath {
		return origin.moduleDir, true
	}
	if rest, ok := strings.CutPrefix(importPath, origin.modulePath+"/"); ok {
		return filepath.Join(origin.moduleDir, filepath.FromSlash(rest)), true
	}
	return "", false
}

// readModulePath extracts the module path from a go.mod, tolerating comments
// and the rare quoted form. Empty on any malformation — an unreadable module
// declaration disables in-repo mapping rather than guessing.
func readModulePath(gomodAbs string) string {
	data, err := os.ReadFile(gomodAbs)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "module")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = rest[:i]
		}
		return strings.Trim(strings.TrimSpace(rest), "\"`")
	}
	return ""
}
