package sourcespawn

import (
	"context"
	"testing"

	"github.com/c360studio/semsource/config"
)

func TestRemovedSourceCanBeReadded(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	source := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}
	added, err := Add(ctx, source, store, Options{Org: "test"})
	if err != nil {
		t.Fatal(err)
	}
	handle := added[0].InstanceName
	if err := Remove(ctx, handle, store); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, handle, store); CodeOf(err) != CodeNotFound {
		t.Fatalf("repeated removal error=%v, want NOT_FOUND", err)
	}
	if current, exists := store.GetConfig().Get().Components[handle]; !exists || current.Enabled {
		t.Fatal("removed desired component must retain an explicit disabled override")
	}
	// A checker can still see the old admitted runtime before the next boot.
	readded, err := AddWithChecker(ctx, source, store, fakeChecker{known: map[string]bool{handle: true}}, Options{Org: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(readded) != 1 || !readded[0].Created || readded[0].InstanceName != handle || !store.GetConfig().Get().Components[handle].Enabled {
		t.Fatalf("re-add must reactivate the same desired handle: %+v", readded)
	}
}

func TestRemoveWriteFailureKeepsSourceEnabled(t *testing.T) {
	store := newFakeStore()
	added, err := Add(context.Background(), config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}, store, Options{Org: "test"})
	if err != nil {
		t.Fatal(err)
	}
	handle := added[0].InstanceName
	store.failPut = true
	if err := Remove(context.Background(), handle, store); CodeOf(err) != CodeKVWriteFailed {
		t.Fatalf("removal error=%v, want KV_WRITE_FAILED", err)
	}
	if !store.GetConfig().Get().Components[handle].Enabled {
		t.Fatal("failed desired write disabled the original component")
	}
}
