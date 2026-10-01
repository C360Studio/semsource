package video

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/internal/seedproof"
)

func TestUnreadableGeneratedKeyframeFailsBoundSeedProof(t *testing.T) {
	bin := t.TempDir()
	// A dangling generated JPEG reliably exercises ReadFile failure even when
	// tests run as root; a second readable JPEG proves partial results survive.
	script := `#!/bin/sh
for last do :; done
dir=${last%/*}
printf jpeg > "$dir/frame_0001.jpg"
ln -s "$dir/missing.jpg" "$dir/frame_0002.jpg"
`
	if err := os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := New()
	ctx, proof := seedproof.Begin(t.Context())
	frames, err := h.extractKeyframes(ctx, "fixture.mp4", "interval", "1s", 0)
	if err != nil || len(frames) != 1 {
		t.Fatalf("ordinary partial enumeration changed: frames=%d err=%v", len(frames), err)
	}
	if proof.Err() == nil {
		t.Fatal("skipped generated keyframe granted a complete bound seed proof")
	}
	frames, err = h.extractKeyframes(context.Background(), "fixture.mp4", "interval", "1s", 0)
	if err != nil || len(frames) != 1 {
		t.Fatalf("unbound ingestion no longer preserves partial results: frames=%d err=%v", len(frames), err)
	}
}
