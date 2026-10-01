//go:build qualification

package setup03a

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHarnessFailsFastAfterApplicationExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := t.TempDir()
	binary := filepath.Join(out, "failed-start")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 23\n"), 0700); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, ctx: ctx, out: out, binary: binary}
	h.start()
	defer h.stop(false)
	select {
	case <-h.exited:
	case <-ctx.Done():
		t.Fatal("failed-start subprocess did not exit")
	}
	queried := false
	if h.poll(time.Minute, func() bool { queried = true; return false }) {
		t.Fatal("exited application reported ready")
	}
	if queried {
		t.Fatal("readiness continued polling after the application exited")
	}
	if err := h.applicationExit(); err == nil || !strings.Contains(err.Error(), "exit status 23") {
		t.Fatalf("lost concrete application exit: %v", err)
	}
}
