package astsource

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/entitypub"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

func result(path string) *semsourceast.ParseResult {
	return &semsourceast.ParseResult{Path: path, FileEntity: &semsourceast.CodeEntity{ID: "id-" + path}}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestFileSet(t *testing.T) {
	pw := &pathWatcher{}

	// A full pass reports every path it added; a folded result (nil file
	// entity) is never in the set.
	folded := &semsourceast.ParseResult{Path: "a/folded.go"}
	got := pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go"), result("a/b/x.go"), result("a/bc.go"), folded})
	slices.Sort(got)
	if want := []string{"a/b/x.go", "a/bc.go", "a/one.go"}; !slices.Equal(got, want) {
		t.Fatalf("first replace changed %v, want %v", got, want)
	}

	// A modify of an indexed path changes nothing; a new path does.
	if pw.addFile("a/one.go", "id-a/one.go") {
		t.Error("addFile of an indexed path reported a change")
	}
	if !pw.addFile("a/b/y.go", "id-a/b/y.go") {
		t.Error("addFile of a new path reported no change")
	}

	// A watch-created file folding onto an indexed one's ID is not added.
	if pw.addFile("a-one.go", "id-a/one.go") {
		t.Error("addFile of a path whose ID another path holds reported a change")
	}

	// A directory delete removes everything under it and nothing that only
	// shares its name as a prefix (a/bc.go).
	removed := pw.removeFiles("a/b")
	slices.Sort(removed)
	if want := []string{"a/b/x.go", "a/b/y.go"}; !slices.Equal(removed, want) {
		t.Fatalf("removeFiles(a/b) = %v, want %v", removed, want)
	}
	if removed := pw.removeFiles("a/b"); len(removed) != 0 {
		t.Errorf("second removeFiles(a/b) = %v, want nothing", removed)
	}

	// The next full pass reports both additions and removals.
	got = pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go"), result("a/new.go")})
	slices.Sort(got)
	if want := []string{"a/bc.go", "a/new.go"}; !slices.Equal(got, want) {
		t.Fatalf("second replace changed %v, want %v", got, want)
	}
}

// planned returns each planned publish as "<path>:<reconcile|skip>:<contains>".
func planned(plan []containerPublish) []string {
	var out []string
	for _, p := range plan {
		mode := "reconcile"
		if p.skipReconcile {
			mode = "skip"
		}
		out = append(out, fmt.Sprintf("%s:%s:%d", p.entity.Path, mode, len(p.entity.Contains)))
	}
	return out
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestPlanContainers(t *testing.T) {
	auth := entityid.Authority{Org: "acme", Platform: "test"}
	files := func(paths ...string) map[string]string {
		m := map[string]string{}
		for _, p := range paths {
			m[p] = "id-" + p
		}
		return m
	}

	t.Run("a watch delete reconciles the repo and ancestors and empties the emptied", func(t *testing.T) {
		got := planned(planContainers(files("a/one.go", "a/b/x.go"), []string{"a/c/y.go"}, false, true, auth, "proj"))
		want := []string{":reconcile:1", "a:reconcile:2", "a/c:reconcile:0"}
		if !slices.Equal(got, want) {
			t.Fatalf("plan = %v, want %v", got, want)
		}
	})

	// a-b and a/b both sanitize to folder ID ...folder.a-b (codex review P1).
	for name, all := range map[string]bool{"watch": false, "reindex": true} {
		t.Run(name+" delete never empties a live colliding folder", func(t *testing.T) {
			plan := planContainers(files("a/b/two.go"), []string{"a-b/one.go"}, all, true, auth, "proj")
			for _, p := range plan {
				if strings.HasSuffix(p.entity.ID, ".folder.a-b") && len(p.entity.Contains) == 0 {
					t.Fatalf("plan %v empties folder a-b, which a/b still holds", planned(plan))
				}
			}
			if got := planned(plan); !slices.Contains(got, "a/b:reconcile:1") {
				t.Fatalf("plan %v does not republish the live a/b folder with its child", got)
			}
		})
	}

	t.Run("two live folders sharing an ID: a watch event writes neither, a full pass never reconciles", func(t *testing.T) {
		set := files("a-b/one.go", "a/b/two.go")
		if got := planned(planContainers(set, []string{"a/b/two.go"}, false, true, auth, "proj")); slices.ContainsFunc(got, func(p string) bool {
			return strings.HasPrefix(p, "a-b:") || strings.HasPrefix(p, "a/b:")
		}) {
			t.Errorf("watch plan %v writes a colliding folder", got)
		}
		for _, p := range planned(planContainers(set, []string{"a/b/two.go"}, true, true, auth, "proj")) {
			if (strings.HasPrefix(p, "a-b:") || strings.HasPrefix(p, "a/b:")) && !strings.Contains(p, ":skip:") {
				t.Errorf("full-pass plan reconciles colliding folder %s", p)
			}
		}
	})
}

// TestPublishContainers_RetriesRefusedPublish: an emptied folder whose
// publish the publisher refused is published by the next, unchanged, full
// pass (codex review P2).
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestPublishContainers_RetriesRefusedPublish(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	// Never started, capacity 1: the first Send fills the buffer, every
	// later one is refused at once.
	full, err := entitypub.New(&recordingClient{}, logger, entitypub.WithCapacity(1), entitypub.WithSendTimeout(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	pw := &pathWatcher{root: t.TempDir(), scopedSystem: "proj"}
	pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go"), result("a/c/y.go")})
	c := &Component{authority: entityid.Authority{Org: "acme", Platform: "test"}, logger: logger,
		publisher: full, distinct: entitypub.NewDistinctTracker()}

	pw.filesMu.Lock()
	c.publishContainers(context.Background(), pw, pw.removeFiles("a/c/y.go"), false, true, false)
	pw.filesMu.Unlock()
	if !slices.Equal(pw.pendingContainers, []string{"a/c/y.go"}) {
		t.Fatalf("pending after a refused publish = %v, want [a/c/y.go]", pw.pendingContainers)
	}

	client := &recordingClient{}
	ok, err := entitypub.New(client, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok.Start(ctx)
	c.publisher = ok
	pw.filesMu.Lock()
	c.publishContainers(ctx, pw, pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go")}), true, true, false)
	pw.filesMu.Unlock()
	if err := ok.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if len(pw.pendingContainers) != 0 {
		t.Errorf("pending after a successful publish = %v", pw.pendingContainers)
	}
	if len(client.containing(".folder.a-c")) == 0 {
		t.Fatal("the unchanged full pass did not retry the emptied folder a/c")
	}
}
