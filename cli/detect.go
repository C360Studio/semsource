package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProjectInfo holds auto-detected facts about the current working directory.
type ProjectInfo struct {
	// HasGit is true if a .git directory exists.
	HasGit bool
	// GitRemote is the origin remote URL, if available.
	GitRemote string
	// Language detected from manifest files ("go", "typescript", etc.).
	Language string
	// HasDocs is true if a docs/ directory or README.md exists.
	HasDocs bool
	// DocPaths lists detected documentation paths.
	DocPaths []string
	// ConfigFiles lists detected config files (go.mod, package.json, Dockerfile, etc.).
	ConfigFiles []string
	// HasImages is true if one or more common image directories are found.
	HasImages bool
	// ImagePaths lists detected image directories.
	ImagePaths []string
	// DirName is the base name of the working directory.
	DirName string
	// Namespace is the best-guess org name (from git remote or dir name).
	Namespace string
}

// DetectProject scans the given directory and returns what it finds.
func DetectProject(dir string) *ProjectInfo {
	info := &ProjectInfo{
		DirName: filepath.Base(dir),
	}

	// Git detection.
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		info.HasGit = true
		info.GitRemote = detectGitRemote(dir)
	}

	// Language detection from manifest files.
	manifests := map[string]string{
		"go.mod":       "go",
		"package.json": "typescript",
		"Cargo.toml":   "rust",
		"pom.xml":      "java",
		"build.gradle": "java",
		"pyproject.toml": "python",
		"requirements.txt": "python",
	}
	for file, lang := range manifests {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			if info.Language == "" {
				info.Language = lang
			}
			info.ConfigFiles = append(info.ConfigFiles, file)
		}
	}

	// Additional config files.
	extras := []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "Makefile", ".env.example"}
	for _, f := range extras {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			info.ConfigFiles = append(info.ConfigFiles, f)
		}
	}

	// Documentation detection.
	if _, err := os.Stat(filepath.Join(dir, "docs")); err == nil {
		info.HasDocs = true
		info.DocPaths = append(info.DocPaths, "docs/")
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		info.HasDocs = true
		info.DocPaths = append(info.DocPaths, "README.md")
	}

	// Image directory detection.
	imageCandidates := []struct{ subdir, display string }{
		{"assets", "assets/"},
		{"images", "images/"},
		{"screenshots", "screenshots/"},
	}
	// static/images and docs/images require two-level check.
	twoLevel := []struct{ subdir, display string }{
		{filepath.Join("static", "images"), "static/images/"},
		{filepath.Join("docs", "images"), "docs/images/"},
	}
	for _, c := range imageCandidates {
		if fi, err := os.Stat(filepath.Join(dir, c.subdir)); err == nil && fi.IsDir() {
			info.HasImages = true
			info.ImagePaths = append(info.ImagePaths, c.display)
		}
	}
	for _, c := range twoLevel {
		if fi, err := os.Stat(filepath.Join(dir, c.subdir)); err == nil && fi.IsDir() {
			info.HasImages = true
			info.ImagePaths = append(info.ImagePaths, c.display)
		}
	}

	// Derive namespace.
	info.Namespace = deriveNamespace(info)

	return info
}

// detectGitRemote runs git to get the origin remote URL.
func detectGitRemote(dir string) string {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// deriveNamespace extracts an org/owner name from the git remote URL,
// falling back to the directory name.
func deriveNamespace(info *ProjectInfo) string {
	if info.GitRemote != "" {
		if ns := orgFromRemote(info.GitRemote); ns != "" {
			return ns
		}
	}
	return sanitizeName(info.DirName)
}

// orgFromRemote extracts the org/owner segment from common git remote formats:
//   - https://github.com/acme/repo.git  -> acme
//   - git@github.com:acme/repo.git      -> acme
func orgFromRemote(remote string) string {
	// SSH format: git@host:org/repo.git
	if strings.Contains(remote, ":") && strings.Contains(remote, "@") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			path := strings.TrimSuffix(parts[1], ".git")
			segs := strings.Split(path, "/")
			if len(segs) >= 2 {
				return sanitizeName(segs[0])
			}
		}
	}

	// HTTPS format: https://host/org/repo.git
	remote = strings.TrimSuffix(remote, ".git")
	// Remove scheme.
	if idx := strings.Index(remote, "://"); idx >= 0 {
		remote = remote[idx+3:]
	}
	segs := strings.Split(remote, "/")
	// host/org/repo -> segs[1] is org
	if len(segs) >= 3 {
		return sanitizeName(segs[1])
	}

	return ""
}

// sanitizeName lowercases and replaces non-alphanumeric chars with hyphens.
func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
