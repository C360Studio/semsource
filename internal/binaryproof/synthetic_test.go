package binaryproof

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	source "github.com/c360studio/semsource/source/vocabulary"
)

var syntheticFixtureBytes = []byte{
	0x53, 0x45, 0x4d, 0x53, 0x4f, 0x55, 0x52, 0x43,
	0x45, 0x2d, 0x42, 0x49, 0x4e, 0x2d, 0x50, 0x52,
	0x4f, 0x4f, 0x46, 0x00, 0x01, 0x02, 0x7f, 0x80,
	0xfe, 0xff,
}

func TestBuildSyntheticFixtureStoresByReference(t *testing.T) {
	ctx := context.Background()
	fixturePath := writeFixture(t, syntheticFixtureBytes)
	store := newStreamingStore()

	result, err := BuildSyntheticFixture(ctx, "acme", fixturePath, store, DefaultStorageInstance, fixedTime())
	if err != nil {
		t.Fatalf("BuildSyntheticFixture() error = %v", err)
	}

	if result.Payload.IndexingProfileHint != graph.IndexingProfileTrace {
		t.Fatalf("IndexingProfileHint = %q, want trace", result.Payload.IndexingProfileHint)
	}
	if result.StorageRef == nil {
		t.Fatal("StorageRef is nil")
	}
	if result.StorageRef.ContentType != "application/octet-stream" {
		t.Fatalf("ContentType = %q", result.StorageRef.ContentType)
	}
	if result.StorageRef.Size != int64(len(syntheticFixtureBytes)) {
		t.Fatalf("StorageRef.Size = %d, want %d", result.StorageRef.Size, len(syntheticFixtureBytes))
	}
	if !store.putReaderCalled {
		t.Fatal("expected streaming PutReader path")
	}
	if store.putCalled {
		t.Fatal("byte-slice Put must not be used when PutReader is available")
	}
	if got := store.data[result.StorageRef.Key]; string(got) != string(syntheticFixtureBytes) {
		t.Fatalf("stored bytes = %v, want fixture bytes", got)
	}

	requireTriple(t, result.Payload, source.MediaStorageRef, result.StorageRef.Key)
	requireTriple(t, result.Payload, source.MediaFileHash, result.Hash)
	requireTriple(t, result.Payload, source.MediaFileSize, result.Size)
	requireTriple(t, result.Payload, source.MediaByteRange, fmt.Sprintf("0:%d", result.Size))
	requireTriple(t, result.Payload, source.MediaExtractionFinding,
		"synthetic fixture scanned for hash, size, byte range, and storage reference only")
	assertNoRawBinaryTriple(t, result.Payload, syntheticFixtureBytes)
	assertNoProtocolClaims(t, result.Payload)
}

func TestBuildSyntheticFixtureFallsBackForByteOnlyStore(t *testing.T) {
	ctx := context.Background()
	fixturePath := writeFixture(t, syntheticFixtureBytes)
	store := newByteOnlyStore()

	result, err := BuildSyntheticFixture(ctx, "acme", fixturePath, store, "byte-only", fixedTime())
	if err != nil {
		t.Fatalf("BuildSyntheticFixture() error = %v", err)
	}

	if !store.putCalled {
		t.Fatal("expected fallback Put path for byte-only store")
	}
	if got := store.data[result.StorageRef.Key]; string(got) != string(syntheticFixtureBytes) {
		t.Fatalf("stored bytes = %v, want fixture bytes", got)
	}
	assertNoRawBinaryTriple(t, result.Payload, syntheticFixtureBytes)
}

func writeFixture(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-binary-proof.bin")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
}

func requireTriple(t *testing.T, payload *graph.EntityPayload, predicate string, want any) {
	t.Helper()
	for _, triple := range payload.TripleData {
		if triple.Predicate == predicate && triple.Object == want {
			return
		}
	}
	t.Fatalf("missing triple %s = %#v", predicate, want)
}

func assertNoRawBinaryTriple(t *testing.T, payload *graph.EntityPayload, raw []byte) {
	t.Helper()
	rawString := string(raw)
	for _, triple := range payload.TripleData {
		if got, ok := triple.Object.([]byte); ok && string(got) == rawString {
			t.Fatalf("triple %s contains raw bytes", triple.Predicate)
		}
		if got, ok := triple.Object.(string); ok && got == rawString {
			t.Fatalf("triple %s contains raw bytes as string", triple.Predicate)
		}
	}
}

func assertNoProtocolClaims(t *testing.T, payload *graph.EntityPayload) {
	t.Helper()
	forbidden := []string{
		"klv",
		"stanag",
		"sapient",
		"skg",
		"streaming-binary",
		"parser conformance",
		"protocol conformance",
	}
	for _, triple := range payload.TripleData {
		text := strings.ToLower(fmt.Sprint(triple.Object))
		for _, term := range forbidden {
			if strings.Contains(text, term) {
				t.Fatalf("triple %s contains forbidden claim term %q", triple.Predicate, term)
			}
		}
	}
}

type byteOnlyStore struct {
	putCalled bool
	data      map[string][]byte
}

func newByteOnlyStore() *byteOnlyStore {
	return &byteOnlyStore{data: make(map[string][]byte)}
}

func (s *byteOnlyStore) Put(_ context.Context, key string, data []byte) error {
	s.putCalled = true
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *byteOnlyStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return append([]byte(nil), data...), nil
}

func (s *byteOnlyStore) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range s.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (s *byteOnlyStore) Delete(_ context.Context, key string) error {
	delete(s.data, key)
	return nil
}

type streamingStore struct {
	*byteOnlyStore
	putReaderCalled bool
}

func newStreamingStore() *streamingStore {
	return &streamingStore{byteOnlyStore: newByteOnlyStore()}
}

func (s *streamingStore) PutReader(_ context.Context, key string, r io.Reader) (int64, error) {
	s.putReaderCalled = true
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.data[key] = append([]byte(nil), data...)
	return int64(len(data)), nil
}
