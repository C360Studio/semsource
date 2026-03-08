// Package filestore provides a local filesystem implementation of storage.Store.
// Keys map to file paths under a configurable root directory. Writes are
// atomic: data is written to a temp file in the same directory as the target,
// then renamed into place, so readers never see partial writes.
package filestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/storage"
)

// Compile-time assertion that Store satisfies the storage.Store interface.
var _ storage.Store = (*Store)(nil)

// Store implements storage.Store backed by the local filesystem.
// All operations are safe for concurrent use from multiple goroutines.
type Store struct {
	rootDir string
	mu      sync.RWMutex
}

// New creates a Store rooted at rootDir.
//
// rootDir is resolved to an absolute path via filepath.Abs. If createIfMissing
// is true and the directory does not exist, it is created with permission 0755.
// If createIfMissing is false and the directory does not exist, an error is
// returned.
func New(rootDir string, createIfMissing bool) (*Store, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("filestore: resolve rootDir %q: %w", rootDir, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("filestore: stat rootDir %q: %w", abs, err)
		}
		if !createIfMissing {
			return nil, fmt.Errorf("filestore: rootDir %q does not exist", abs)
		}
		if mkErr := os.MkdirAll(abs, 0o755); mkErr != nil {
			return nil, fmt.Errorf("filestore: create rootDir %q: %w", abs, mkErr)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("filestore: rootDir %q is not a directory", abs)
	}

	return &Store{rootDir: abs}, nil
}

// Close is a no-op for the filesystem backend but satisfies lifecycle patterns
// used by other storage implementations.
func (s *Store) Close() error {
	return nil
}

// Put writes data to the given key atomically. Intermediate directories are
// created as needed. The write is performed via a temp file in the same
// directory as the target, then renamed to the final path.
func (s *Store) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("filestore: Put %q: %w", key, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, err := s.resolve(key)
	if err != nil {
		return fmt.Errorf("filestore: Put %q: %w", key, err)
	}

	// Ensure the parent directory exists before creating the temp file there.
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("filestore: Put %q: create parent dir: %w", key, err)
	}

	// Write to a temp file in the same directory so the rename is atomic on
	// POSIX systems (same filesystem, single syscall).
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("filestore: Put %q: create temp file: %w", key, err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any failure path.
	committed := false
	defer func() {
		if !committed {
			tmp.Close()       //nolint:errcheck
			os.Remove(tmpName) //nolint:errcheck
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("filestore: Put %q: write temp file: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("filestore: Put %q: close temp file: %w", key, err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("filestore: Put %q: rename to target: %w", key, err)
	}
	committed = true
	return nil
}

// Get retrieves the data stored at key. Returns an error if the key does not
// exist.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("filestore: Get %q: %w", key, err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	target, err := s.resolve(key)
	if err != nil {
		return nil, fmt.Errorf("filestore: Get %q: %w", key, err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("filestore: Get %q: %w", key, err)
	}
	return data, nil
}

// List returns all keys whose paths start with the given prefix, in
// lexicographic (sorted) order. An empty prefix matches all keys. Returns an
// empty (non-nil) slice when no keys match.
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("filestore: List %q: %w", prefix, err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Determine the directory to start walking from. When a prefix is given we
	// walk the resolved directory so we skip unrelated directory trees early.
	// We still filter by key prefix afterwards to handle prefixes that don't
	// land on a directory boundary (e.g. "foo/ba" matches "foo/bar" and
	// "foo/baz" but not "foo/qux").
	var walkRoot string
	if prefix == "" {
		walkRoot = s.rootDir
	} else {
		prefixPath, err := s.resolve(prefix)
		if err != nil {
			return nil, fmt.Errorf("filestore: List %q: %w", prefix, err)
		}
		// Walk the deepest existing ancestor so we don't error when the prefix
		// path doesn't yet correspond to a real file or directory.
		walkRoot = s.deepestExisting(prefixPath)
	}

	var keys []string
	walkErr := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip entries we cannot read (e.g. permission denied).
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Skip temp files left by in-flight Put operations.
		if strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		key := s.keyFromPath(path)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("filestore: List %q: walk: %w", prefix, walkErr)
	}

	sort.Strings(keys)
	if keys == nil {
		keys = []string{}
	}
	return keys, nil
}

// Delete removes the data stored at key. Returns nil if the key does not exist
// (idempotent).
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("filestore: Delete %q: %w", key, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, err := s.resolve(key)
	if err != nil {
		return fmt.Errorf("filestore: Delete %q: %w", key, err)
	}

	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("filestore: Delete %q: %w", key, err)
	}
	return nil
}

// resolve converts a key to an absolute filesystem path under rootDir.
//
// Keys use forward slashes as separators regardless of the host OS. The
// function rejects empty keys, keys containing ".." segments, and any resolved
// path that escapes rootDir (path traversal protection).
func (s *Store) resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("key must not be empty")
	}
	// Reject keys that explicitly contain ".." to catch obvious traversal
	// attempts before filepath.Clean has a chance to collapse them.
	for seg := range strings.SplitSeq(key, "/") {
		if seg == ".." {
			return "", fmt.Errorf("key %q contains illegal segment \"..\"", key)
		}
	}

	abs := filepath.Clean(filepath.Join(s.rootDir, filepath.FromSlash(key)))

	// Ensure the resolved path is still inside rootDir.
	// filepath.Clean guarantees no trailing separator, so we add one to rootDir
	// before the HasPrefix check to avoid false positives like:
	//   rootDir = /tmp/store
	//   abs     = /tmp/storeX/file  → HasPrefix("/tmp/store") is true but wrong
	safeRoot := s.rootDir + string(filepath.Separator)
	if !strings.HasPrefix(abs, safeRoot) {
		return "", fmt.Errorf("key %q resolves outside of rootDir", key)
	}

	return abs, nil
}

// keyFromPath converts an absolute filesystem path back to a slash-separated
// key by stripping the rootDir prefix.
func (s *Store) keyFromPath(absPath string) string {
	rel := strings.TrimPrefix(absPath, s.rootDir+string(filepath.Separator))
	return filepath.ToSlash(rel)
}

// deepestExisting walks upward from path until it finds a directory that
// actually exists on disk, then returns that directory. It never walks above
// rootDir to prevent accidental full-filesystem traversal.
func (s *Store) deepestExisting(path string) string {
	candidate := path
	for {
		if !strings.HasPrefix(candidate, s.rootDir) {
			return s.rootDir
		}
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return s.rootDir
		}
		candidate = parent
	}
}
