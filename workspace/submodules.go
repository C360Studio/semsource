package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// MaxSubmoduleDepth bounds submodule inventory recursion. Entries one level
// beyond the cap are reported in SubmoduleInventory.BeyondCap rather than
// silently dropped; nothing deeper is probed.
const MaxSubmoduleDepth = 10

// SubmoduleInfo describes one submodule declared by a repository's
// .gitmodules, at any nesting depth.
type SubmoduleInfo struct {
	// Path is the submodule working-tree path relative to the ROOT repo,
	// slash-separated (nested submodules compose their parents' paths).
	Path string

	// URL is the raw .gitmodules url value.
	URL string

	// ResolvedURL is URL with a relative form ("./x", "../x") resolved
	// against the declaring repo's remote. Identity derivation must use
	// this, not URL: the raw relative form differs per consumer while the
	// resolved form is canonical. Equal to URL when already absolute or
	// when the declaring repo has no remote to resolve against.
	ResolvedURL string

	// SHA is the full gitlink commit hash the declaring repo pins, read
	// from the index (mode 160000) — NOT the submodule's checked-out HEAD,
	// which can differ. Empty when .gitmodules declares a path that has no
	// gitlink (a stale entry).
	SHA string

	// Materialized reports whether the working tree at Path has content.
	// A plain (non-recursive) clone leaves declared submodule directories
	// empty; those are the silently-absent trees the loudness contract
	// surfaces.
	Materialized bool

	// Depth is 1 for submodules of the root repo, 2 for their submodules,
	// and so on.
	Depth int
}

// SubmoduleInventory is the result of ListSubmodules.
type SubmoduleInventory struct {
	// Submodules lists every declared submodule within MaxSubmoduleDepth,
	// in declaration order, parents before their children.
	Submodules []SubmoduleInfo

	// BeyondCap lists root-relative paths declared one level beyond
	// MaxSubmoduleDepth. Non-empty means the inventory is incomplete and
	// the caller must say so — never treat a capped tree as fully expanded.
	BeyondCap []string
}

// ListSubmodules inventories the submodules a repository declares, recursing
// into materialized trees up to MaxSubmoduleDepth. It never touches the
// network and never mutates the repo: declarations come from .gitmodules,
// pinned SHAs from the index, materialization from the working tree.
func ListSubmodules(ctx context.Context, repoPath string) (*SubmoduleInventory, error) {
	return listSubmodules(ctx, repoPath, MaxSubmoduleDepth)
}

func listSubmodules(ctx context.Context, repoPath string, maxDepth int) (*SubmoduleInventory, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("workspace: repo path is required")
	}
	inv := &SubmoduleInventory{}
	remote := originURL(ctx, repoPath)
	if err := listSubmodulesInto(ctx, repoPath, "", remote, 1, maxDepth, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// listSubmodulesInto appends repoPath's declared submodules to inv. prefix is
// repoPath's own root-relative path ("" for the root repo); parentURL is the
// URL relative submodule declarations resolve against. depth is the depth of
// the entries declared HERE; entries at maxDepth+1 go to BeyondCap.
func listSubmodulesInto(ctx context.Context, repoPath, prefix, parentURL string, depth, maxDepth int, inv *SubmoduleInventory) error {
	declared, err := declaredSubmodules(ctx, repoPath)
	if err != nil {
		return err
	}
	if len(declared) == 0 {
		return nil
	}

	shas, err := gitlinkSHAs(ctx, repoPath, declared)
	if err != nil {
		return err
	}

	for _, d := range declared {
		rootRel := path.Join(prefix, d.path)
		if depth > maxDepth {
			inv.BeyondCap = append(inv.BeyondCap, rootRel)
			continue
		}

		dir := filepath.Join(repoPath, filepath.FromSlash(d.path))
		info := SubmoduleInfo{
			Path:         rootRel,
			URL:          d.url,
			ResolvedURL:  resolveSubmoduleURL(parentURL, d.url),
			SHA:          shas[d.path],
			Materialized: dirHasContent(dir),
			Depth:        depth,
		}
		inv.Submodules = append(inv.Submodules, info)

		if info.Materialized {
			if err := listSubmodulesInto(ctx, dir, rootRel, info.ResolvedURL, depth+1, maxDepth, inv); err != nil {
				return err
			}
		}
	}
	return nil
}

// declaredSubmodule is one .gitmodules entry.
type declaredSubmodule struct {
	path string
	url  string
}

// declaredSubmodules parses repoPath's .gitmodules via git config. Returns
// entries in declaration order; nil when the file does not exist.
func declaredSubmodules(ctx context.Context, repoPath string) ([]declaredSubmodule, error) {
	if _, err := os.Stat(filepath.Join(repoPath, ".gitmodules")); err != nil {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, "git", "config", "--file", ".gitmodules", "--get-regexp", `^submodule\.`)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// Exit status 1 with no output means no matching keys — an empty
		// or comment-only .gitmodules, not a failure.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("workspace: parse .gitmodules in %s: %w", repoPath, err)
	}

	// Keys are submodule.<name>.path / submodule.<name>.url, where <name>
	// itself may contain dots — split on the LAST dot for the field name.
	byName := map[string]*declaredSubmodule{}
	var order []string
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		lastDot := strings.LastIndex(key, ".")
		if lastDot <= len("submodule.") {
			continue
		}
		name := key[len("submodule."):lastDot]
		field := key[lastDot+1:]
		entry := byName[name]
		if entry == nil {
			entry = &declaredSubmodule{}
			byName[name] = entry
			order = append(order, name)
		}
		switch field {
		case "path":
			entry.path = value
		case "url":
			entry.url = value
		}
	}

	var declared []declaredSubmodule
	for _, name := range order {
		e := byName[name]
		if e.path == "" {
			continue // unusable without a path
		}
		declared = append(declared, *e)
	}
	return declared, nil
}

// gitlinkSHAs reads the pinned gitlink hash for each declared path from the
// index (ls-files mode 160000). A declared path with no gitlink is simply
// absent from the result — the stale-.gitmodules case.
func gitlinkSHAs(ctx context.Context, repoPath string, declared []declaredSubmodule) (map[string]string, error) {
	args := []string{"ls-files", "-s", "--"}
	for _, d := range declared {
		args = append(args, d.path)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("workspace: git ls-files in %s: %w", repoPath, err)
	}

	shas := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		// Format: "<mode> <sha> <stage>\t<path>"
		meta, p, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 || fields[0] != "160000" {
			continue
		}
		shas[p] = fields[1]
	}
	return shas, nil
}

// originURL returns repoPath's origin remote URL, or "" when there is none
// (a purely local repo) — relative submodule URLs then stay unresolved.
func originURL(ctx context.Context, repoPath string) string {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveSubmoduleURL resolves a relative .gitmodules url ("./x", "../x")
// against the declaring repo's remote URL, per git's rule that the remote
// itself is the base ("../sib" next to https://host/org/repo →
// https://host/org/sib). Absolute urls pass through, as does everything when
// there is no base to resolve against.
func resolveSubmoduleURL(base, raw string) string {
	if !strings.HasPrefix(raw, "./") && !strings.HasPrefix(raw, "../") {
		return raw
	}
	if base == "" {
		return raw
	}

	// Split the base into an immutable prefix and a joinable path part.
	var prefix, basePath string
	switch {
	case strings.Contains(base, "://"):
		// scheme://host/path — keep scheme://host, join on /path.
		i := strings.Index(base, "://")
		rest := base[i+3:]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return raw
		}
		prefix = base[:i+3] + rest[:slash]
		basePath = rest[slash:]
	case strings.HasPrefix(base, "git@") && strings.Contains(base, ":"):
		// scp-like git@host:path.
		i := strings.Index(base, ":")
		prefix = base[:i+1]
		basePath = base[i+1:]
	default:
		// Filesystem path.
		basePath = base
	}

	joined := path.Join(basePath, raw)
	return prefix + joined
}

// dirHasContent reports whether dir exists and has at least one entry.
func dirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}
