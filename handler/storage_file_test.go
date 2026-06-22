package handler_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
)

type byteStore struct {
	data map[string][]byte
}

func newByteStore() *byteStore {
	return &byteStore{data: make(map[string][]byte)}
}

func (s *byteStore) Put(_ context.Context, key string, data []byte) error {
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *byteStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return append([]byte(nil), data...), nil
}

func (s *byteStore) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range s.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *byteStore) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

type streamStore struct {
	*byteStore
	putCalled       bool
	putReaderCalled bool
}

func (s *streamStore) Put(_ context.Context, key string, data []byte) error {
	s.putCalled = true
	return s.byteStore.Put(context.Background(), key, data)
}

func (s *streamStore) PutReader(_ context.Context, key string, r io.Reader) (int64, error) {
	s.putReaderCalled = true
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	return int64(len(data)), s.byteStore.Put(context.Background(), key, data)
}

func TestStoreFileUsesStreamWriterWhenAvailable(t *testing.T) {
	path := writeStorageFixture(t, "protocol\x00bytes")
	store := &streamStore{byteStore: newByteStore()}

	if err := handler.StoreFile(context.Background(), store, "binary/demo", path); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	if !store.putReaderCalled {
		t.Fatal("expected StoreFile to use PutReader")
	}
	if store.putCalled {
		t.Fatal("StoreFile should not call Put when PutReader is available")
	}
	got, err := store.Get(context.Background(), "binary/demo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "protocol\x00bytes" {
		t.Fatalf("stored bytes = %q", got)
	}
}

func TestStoreFileFallsBackToBytePut(t *testing.T) {
	path := writeStorageFixture(t, "fallback-bytes")
	store := newByteStore()

	if err := handler.StoreFile(context.Background(), store, "binary/fallback", path); err != nil {
		t.Fatalf("StoreFile: %v", err)
	}

	got, err := store.Get(context.Background(), "binary/fallback")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "fallback-bytes" {
		t.Fatalf("stored bytes = %q", got)
	}
}

func writeStorageFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
