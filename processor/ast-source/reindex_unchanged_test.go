package astsource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/entitypub"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// newContainerFixture is a fold fixture without the colliding a-b.ts, plus a
// nested folder, so every container is an ordinary one.
func newContainerFixture(t *testing.T) *foldFixture {
	t.Helper()
	f := newFoldFixture(t)
	if err := os.Remove(filepath.Join(f.root, "a-b.ts")); err != nil {
		t.Fatal(err)
	}
	f.write("a/c/y.ts", "export function why() {}\n")
	return f
}

// publishedContainers lists the folder and repo subjects the publisher sent.
func (f *foldFixture) publishedContainers() []string {
	f.t.Helper()
	f.client.mu.Lock()
	msgs := slices.Clone(f.client.msgs)
	f.client.mu.Unlock()
	seen := map[string]bool{}
	for _, m := range msgs {
		var v any
		if err := json.Unmarshal(m, &v); err != nil {
			f.t.Fatalf("decode published message: %v", err)
		}
		walkTriples(v, func(subject, _ string, _ any) {
			if strings.Contains(subject, ".folder.") || strings.Contains(subject, ".repo.") {
				seen[subject] = true
			}
		})
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: A periodic reindex does not rewrite unchanged folder and repo entities
func TestReindex_UnchangedTreePublishesNoContainers(t *testing.T) {
	f := newContainerFixture(t)
	f.seed()
	if len(f.publishedContainers()) == 0 {
		t.Fatal("seed published no containers; the fixture is vacuous")
	}
	f.reindex() // the first reindex records nothing new: the seed already recorded every container
	f.reset()
	f.reindex()
	if got := f.publishedContainers(); len(got) != 0 {
		t.Errorf("an idle reindex published containers %v", got)
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: A periodic reindex does not rewrite unchanged folder and repo entities
func TestReindex_AddedFileRepublishesItsContainers(t *testing.T) {
	f := newContainerFixture(t)
	f.seed()
	f.reindex()
	f.reset()
	f.write("a/c/z.ts", "export function zed() {}\n")
	f.reindex()

	// Only a/c's member set changed; a contains the folder a/c, not its files.
	if got := f.publishedContainers(); !slices.Equal(got, []string{"acme.test.proj.code.folder.a-c"}) {
		t.Errorf("after adding a/c/z.ts the reindex published %v, want only folder a/c", got)
	}
	var withZ bool
	for _, m := range f.client.containing(".folder.a-c") {
		if bytes.Contains(m, []byte("z.ts")) {
			withZ = true
		}
	}
	if !withZ {
		t.Error("folder a/c was republished without a/c/z.ts")
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: A periodic reindex does not rewrite unchanged folder and repo entities
func TestPublishContainers_RefusedUnchangedPublishIsRetried(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	pw := &pathWatcher{root: t.TempDir(), scopedSystem: "proj"}
	files := []*semsourceast.ParseResult{result("a/one.go"), result("a/c/y.go")}
	client := &recordingClient{}
	ok, err := entitypub.New(client, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok.Start(ctx)
	c := &Component{authority: entityid.Authority{Org: "acme", Platform: "test"}, logger: logger,
		publisher: ok, distinct: entitypub.NewDistinctTracker()}
	pass := func() {
		pw.filesMu.Lock()
		c.publishContainers(ctx, pw, pw.replaceFiles(files), true, true, true)
		pw.filesMu.Unlock()
	}
	pass() // records every container

	// Never started, capacity 1: the first Send fills the buffer, every
	// later one is refused at once.
	full, err := entitypub.New(&recordingClient{}, logger, entitypub.WithCapacity(1), entitypub.WithSendTimeout(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, result("a/new.go")) // folder a and the repo change
	c.publisher = full
	pass() // the repo is admitted into the buffer; folder a is refused

	c.publisher = ok
	client.mu.Lock()
	client.msgs = nil
	client.mu.Unlock()
	pass() // nothing changed since the refused pass
	if err := ok.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if len(client.containing(".folder.a")) == 0 {
		t.Error("the folder whose publish was refused was not retried by the next unchanged pass")
	}
}
