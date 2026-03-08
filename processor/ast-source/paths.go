package astsource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolvePaths expands glob patterns to concrete directories.
// Supports both single-level wildcards (*) and recursive wildcards (**).
//
// Examples:
//   - "./services/*" → ["./services/auth", "./services/users", ...]
//   - "./frontend" → ["./frontend"]
//   - "./**" → all subdirectories recursively
//
// Returns only directories, not files.
func ResolvePaths(patterns []string) ([]string, error) {
	var resolved []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		paths, err := resolvePattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("resolve pattern %q: %w", pattern, err)
		}

		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				resolved = append(resolved, p)
			}
		}
	}

	return resolved, nil
}

// resolvePattern expands a single glob pattern to directories.
func resolvePattern(pattern string) ([]string, error) {
	// Check if the pattern contains glob characters
	if !containsGlob(pattern) {
		// No glob - return the path if it's a directory
		absPath, err := filepath.Abs(pattern)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			return nil, fmt.Errorf("path is not a directory: %s", absPath)
		}

		return []string{absPath}, nil
	}

	// Handle glob patterns
	// First, convert to absolute path for the base
	absPattern, err := makeAbsolutePattern(pattern)
	if err != nil {
		return nil, err
	}

	// Use doublestar for ** support
	matches, err := doublestar.FilepathGlob(absPattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	// Filter to directories only
	var dirs []string
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue // Skip paths that can't be stat'd
		}
		if info.IsDir() {
			dirs = append(dirs, match)
		}
	}

	if len(dirs) == 0 {
		return nil, fmt.Errorf("no directories match pattern: %s", pattern)
	}

	return dirs, nil
}

// containsGlob checks if a pattern contains glob characters.
func containsGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// makeAbsolutePattern converts a relative pattern to absolute.
// Preserves glob characters in the pattern.
func makeAbsolutePattern(pattern string) (string, error) {
	// Find the first glob character
	globIdx := -1
	for i, c := range pattern {
		if c == '*' || c == '?' || c == '[' {
			globIdx = i
			break
		}
	}

	if globIdx == -1 {
		// No glob characters - just make absolute
		return filepath.Abs(pattern)
	}

	// Split at the glob point
	// Find the directory part before the glob
	dirPart := pattern[:globIdx]
	if lastSep := strings.LastIndex(dirPart, string(filepath.Separator)); lastSep >= 0 {
		dirPart = pattern[:lastSep]
	} else if lastSep := strings.LastIndex(dirPart, "/"); lastSep >= 0 {
		// Handle Unix-style paths on any platform
		dirPart = pattern[:lastSep]
	} else {
		dirPart = "."
	}

	globPart := pattern[len(dirPart):]

	// Make the directory part absolute
	absDir, err := filepath.Abs(dirPart)
	if err != nil {
		return "", err
	}

	// Normalize the glob part separators
	globPart = filepath.FromSlash(globPart)

	return absDir + globPart, nil
}

// ResolvedPath pairs an absolute file path with its originating WatchPathConfig.
type ResolvedPath struct {
	AbsPath string
	Config  WatchPathConfig
}

// ResolveWatchPaths expands all watch path configurations.
// For each WatchPathConfig with a glob pattern, creates multiple ResolvedPath entries.
func ResolveWatchPaths(configs []WatchPathConfig) ([]ResolvedPath, error) {
	var resolved []ResolvedPath
	seen := make(map[string]bool)

	for _, cfg := range configs {
		paths, err := resolvePattern(cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", cfg.Path, err)
		}

		for _, absPath := range paths {
			if seen[absPath] {
				continue
			}
			seen[absPath] = true

			resolved = append(resolved, ResolvedPath{
				AbsPath: absPath,
				Config:  cfg,
			})
		}
	}

	return resolved, nil
}
