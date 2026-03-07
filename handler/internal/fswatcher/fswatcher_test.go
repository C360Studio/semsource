package fswatcher_test

import (
	"testing"

	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// ---------------------------------------------------------------------------
// ContentHash is exported for test seeding
// ---------------------------------------------------------------------------

func TestContentHash_Deterministic(t *testing.T) {
	content := []byte("hello world")
	h1 := fswatcher.ContentHash(content)
	h2 := fswatcher.ContentHash(content)
	if h1 != h2 {
		t.Errorf("ContentHash not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("ContentHash returned empty string")
	}
}

func TestContentHash_DifferentContent(t *testing.T) {
	h1 := fswatcher.ContentHash([]byte("v1"))
	h2 := fswatcher.ContentHash([]byte("v2"))
	if h1 == h2 {
		t.Error("ContentHash collision for different content")
	}
}
