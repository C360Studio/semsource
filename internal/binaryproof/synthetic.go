// Package binaryproof contains narrow synthetic fixtures for validating
// SemSource's binary-source service boundary.
package binaryproof

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/storage"
)

const (
	// SyntheticFormat names the fixture family without implying any protocol.
	SyntheticFormat = "synthetic-binary-fixture"

	// DefaultStorageInstance is the storage instance name used by tests and docs.
	DefaultStorageInstance = "semsource-binary-proof"

	finding = "synthetic fixture scanned for hash, size, byte range, and storage reference only"
)

// SyntheticResult is the governed metadata produced from a synthetic binary file.
type SyntheticResult struct {
	Payload    *graph.EntityPayload
	StorageRef *message.StorageReference
	Hash       string
	Size       int64
}

// BuildSyntheticFixture stores fixture bytes by reference and builds governed
// metadata. It does not parse or validate any binary protocol.
func BuildSyntheticFixture(
	ctx context.Context,
	org string,
	fixturePath string,
	store storage.Store,
	storageInstance string,
	now time.Time,
) (*SyntheticResult, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}
	if storageInstance == "" {
		storageInstance = DefaultStorageInstance
	}

	hash, size, err := hashFile(ctx, fixturePath)
	if err != nil {
		return nil, err
	}
	if len(hash) < 12 {
		return nil, fmt.Errorf("hash %q is too short", hash)
	}

	system := "synthetic-binary"
	instance := hash[:12]
	entityID := entityid.Build(org, entityid.PlatformSemsource, "media", system, "blob", instance)
	storageKey := fmt.Sprintf("binary-proof/%s/%s/original.bin", system, instance)
	if err := handler.StoreFile(ctx, store, storageKey, fixturePath); err != nil {
		return nil, err
	}

	ref := &message.StorageReference{
		StorageInstance: storageInstance,
		Key:             storageKey,
		ContentType:     "application/octet-stream",
		Size:            size,
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: source.MediaType, Object: string(source.MediaTypeBinary)},
		{Subject: entityID, Predicate: source.MediaMimeType, Object: ref.ContentType},
		{Subject: entityID, Predicate: source.MediaFilePath, Object: filepath.Base(fixturePath)},
		{Subject: entityID, Predicate: source.MediaFileHash, Object: hash},
		{Subject: entityID, Predicate: source.MediaFileSize, Object: size},
		{Subject: entityID, Predicate: source.MediaFormat, Object: SyntheticFormat},
		{Subject: entityID, Predicate: source.MediaStorageRef, Object: storageKey},
		{Subject: entityID, Predicate: source.MediaByteRange, Object: fmt.Sprintf("0:%d", size)},
		{Subject: entityID, Predicate: source.MediaExtractionFinding, Object: finding},
	}
	for i := range triples {
		triples[i].Source = "semsource.synthetic-binary-proof"
		triples[i].Timestamp = now.UTC()
		triples[i].Confidence = 1.0
	}

	payload := &graph.EntityPayload{
		ID:                  entityID,
		TripleData:          triples,
		UpdatedAt:           now.UTC(),
		Storage:             ref,
		IndexingProfileHint: graph.IndexingProfileTrace,
	}
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("validate synthetic payload: %w", err)
	}

	return &SyntheticResult{
		Payload:    payload,
		StorageRef: ref,
		Hash:       hash,
		Size:       size,
	}, nil
}

func hashFile(ctx context.Context, path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, &contextReader{ctx: ctx, r: f})
	if err != nil {
		return "", size, fmt.Errorf("hash %q: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
