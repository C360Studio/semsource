package fusiontest

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/storage"
)

// MemStore is an in-memory storage.Store for exercising the fusion body
// hydration path (ADR-062 increment 4) without a live ObjectStore. Register a
// body with Put or the Set helper, then hand it to fusion.NewBodyResolver via a
// MapStoreResolver keyed by the StorageReference.StorageInstance the lens stamps.
type MemStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// compile-time check that MemStore satisfies the storage interface.
var _ storage.Store = (*MemStore)(nil)

// NewMemStore returns an empty in-memory store.
func NewMemStore() *MemStore { return &MemStore{data: map[string][]byte{}} }

// Set registers a body under key (test convenience over Put).
func (m *MemStore) Set(key, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = []byte(body)
}

// Put stores data at key.
func (m *MemStore) Put(_ context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append([]byte(nil), data...)
	return nil
}

// Get returns the data at key, or an error when absent (matching a real store).
func (m *MemStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("memstore: key %q not found", key)
	}
	return append([]byte(nil), v...), nil
}

// List returns keys under prefix in no particular order.
func (m *MemStore) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// Delete removes key (idempotent).
func (m *MemStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
