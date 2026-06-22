package handler

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/c360studio/semstreams/storage"
)

// ReaderStore is an optional extension for stores that can write from a stream.
type ReaderStore interface {
	PutReader(ctx context.Context, key string, r io.Reader) (int64, error)
}

// StoreFile stores path at key. Stores that implement ReaderStore avoid loading
// the whole file into memory; older Store implementations fall back to Put.
func StoreFile(ctx context.Context, store storage.Store, key, path string) error {
	if store == nil {
		return fmt.Errorf("store file %q: storage store is nil", path)
	}

	if streamStore, ok := store.(ReaderStore); ok {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %q for storage: %w", path, err)
		}
		defer f.Close()

		if _, err := streamStore.PutReader(ctx, key, f); err != nil {
			return fmt.Errorf("stream store %q: %w", key, err)
		}
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q for storage: %w", path, err)
	}
	if err := store.Put(ctx, key, content); err != nil {
		return fmt.Errorf("store %q: %w", key, err)
	}
	return nil
}
