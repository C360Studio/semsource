package cfgfilesource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/entitypub/entitypubtest"
)

// recordingClient captures every message the publisher delivers.
type recordingClient struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (r *recordingClient) PublishToStream(_ context.Context, _ string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, append([]byte(nil), data...))
	return nil
}

func (r *recordingClient) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return r.PublishToStream(ctx, subject, data)
}

func (r *recordingClient) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = nil
}

// cfgFixture is a cfgfile-source built by NewComponent, so its reconcile gate
// is the production one, publishing through a recording transport and a
// recording reconciler. Each manifest sits in its own directory under one
// root with a project override, so the handler's states for that directory
// are exactly the states its watcher builds for the one file.
type cfgFixture struct {
	t      *testing.T
	ctx    context.Context
	root   string
	c      *Component
	client *recordingClient
	rec    *entitypubtest.RecordingReconciler
}

// newCfgFixture writes manifests under the first root.
func newCfgFixture(t *testing.T, roots ...string) *cfgFixture {
	t.Helper()
	root := roots[0]
	raw, err := json.Marshal(map[string]any{"paths": roots, "project": "svc", "org": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	disc, err := NewComponent(raw, component.Dependencies{
		Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"},
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	f := &cfgFixture{t: t, ctx: context.Background(), root: root, c: disc.(*Component),
		client: &recordingClient{}, rec: &entitypubtest.RecordingReconciler{}}
	pub, err := entitypub.New(f.client, logger, entitypub.WithReconcile(f.rec))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(f.ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	f.c.publisher = pub
	return f
}

func (f *cfgFixture) write(rel, content string) string {
	f.t.Helper()
	p := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *cfgFixture) seed() {
	f.t.Helper()
	if err := f.c.ingestOnce(f.ctx); err != nil {
		f.t.Fatalf("seed: %v", err)
	}
	f.settle()
}

// watchModify delivers the change event the watcher builds for path.
func (f *cfgFixture) watchModify(path string) {
	f.t.Helper()
	states, err := f.c.handler.IngestEntityStates(f.ctx, &sourceCfg{paths: []string{filepath.Dir(path)}}, f.c.authority)
	if err != nil || len(states) == 0 {
		f.t.Fatalf("build watch states for %s: %d states, %v", path, len(states), err)
	}
	f.c.handleChangeEvent(f.ctx, handler.ChangeEvent{Path: path, Operation: handler.OperationModify, EntityStates: states})
	f.settle()
}

func (f *cfgFixture) settle() {
	f.t.Helper()
	if err := f.c.publisher.WaitSettled(f.ctx); err != nil {
		f.t.Fatalf("settle: %v", err)
	}
}

// groupStatements returns, per subject, the sorted group-`source` statements
// in every message recorded since the last reset. The cfgfile handler emits
// no wall-clock predicate (no dc.terms.created), so none is excluded.
func (f *cfgFixture) groupStatements() map[string][]string {
	f.t.Helper()
	owned := map[string]bool{}
	for _, p := range graph.OwnedPredicates() {
		owned[p] = true
	}
	f.client.mu.Lock()
	msgs := append([][]byte(nil), f.client.msgs...)
	f.client.mu.Unlock()
	out := map[string][]string{}
	for _, m := range msgs {
		var v any
		if err := json.Unmarshal(m, &v); err != nil {
			f.t.Fatalf("decode published message: %v", err)
		}
		walkTriples(v, func(subject, predicate string, object any) {
			if owned[predicate] {
				out[subject] = append(out[subject], fmt.Sprintf("%s=%v", predicate, object))
			}
		})
	}
	for s := range out {
		sort.Strings(out[s])
	}
	return out
}

func walkTriples(v any, fn func(subject, predicate string, object any)) {
	switch x := v.(type) {
	case map[string]any:
		s, sok := x["subject"].(string)
		p, pok := x["predicate"].(string)
		if sok && pok {
			fn(s, p, x["object"])
		}
		for _, child := range x {
			walkTriples(child, fn)
		}
	case []any:
		for _, child := range x {
			walkTriples(child, fn)
		}
	}
}

const (
	goModTwoDeps = "module example.com/svc\n\ngo 1.22\n\nrequire (\n\tgithub.com/a/b v1.0.0\n\tgithub.com/c/d v1.2.0\n)\n"
	goModOneDep  = "module example.com/svc\n\ngo 1.22\n\nrequire github.com/a/b v1.0.0 // indirect\n"
	pomTwoMods   = "<project><groupId>org.x</groupId><artifactId>app</artifactId><version>1</version>" +
		"<modules><module>core</module><module>web</module></modules></project>"
	pomOneMod = "<project><groupId>org.x</groupId><artifactId>app</artifactId><version>1</version>" +
		"<modules><module>core</module></modules></project>"
	gradleTwoDeps = "dependencies {\n    implementation 'org.a:b:1.0'\n    api 'org.c:d:2.0'\n}\n"
	gradleOneDep  = "dependencies {\n    implementation 'org.a:b:1.0'\n}\n"
)

// A watch republish of a manifest that now emits less carries, for every
// entity it still produces, the complete group set a fresh seed of the new
// content produces.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestCfgfileWatchRepublishCarriesTheCompleteGroupSet(t *testing.T) {
	for _, tc := range []struct{ rel, before, after string }{
		{"gomod/go.mod", goModTwoDeps, goModOneDep},
		{"maven/pom.xml", pomTwoMods, pomOneMod},
	} {
		t.Run(tc.rel, func(t *testing.T) {
			root := t.TempDir()
			f := newCfgFixture(t, root)
			path := f.write(tc.rel, tc.before)
			f.seed()
			seeded := f.groupStatements()

			f.write(tc.rel, tc.after)
			f.client.reset()
			f.watchModify(path)
			watched := f.groupStatements()

			fresh := newCfgFixture(t, root)
			fresh.seed()
			want := fresh.groupStatements()

			shrank := false
			for subject, got := range watched {
				if strings.Join(got, "\n") != strings.Join(want[subject], "\n") {
					t.Errorf("%s: watch group set differs from a seed of the same content\n got: %v\nwant: %v",
						subject, got, want[subject])
				}
				shrank = shrank || len(got) < len(seeded[subject])
			}
			if !shrank {
				t.Error("the edit emitted no fewer statements for any entity; the test proves nothing")
			}
		})
	}
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestCfgfileReconcileOnlyKnownChangesOfSingleWriterTypes(t *testing.T) {
	typeOf := func(id string) string { return strings.Split(id, ".")[4] }

	t.Run("a first pass and an unchanged republish make no reconcile calls", func(t *testing.T) {
		f := newCfgFixture(t, t.TempDir())
		path := f.write("gomod/go.mod", goModTwoDeps)
		f.write("maven/pom.xml", pomTwoMods)
		f.seed()
		f.watchModify(path)
		if ids := f.rec.IDs(); len(ids) != 0 {
			t.Errorf("reconciled %v without a known content change", ids)
		}
	})

	t.Run("changed modules reconcile; dependencies, images and projects never", func(t *testing.T) {
		f := newCfgFixture(t, t.TempDir())
		edits := []struct{ rel, before, after string }{
			{"gomod/go.mod", goModTwoDeps, goModOneDep},
			{"maven/pom.xml", pomTwoMods, pomOneMod},
			{"gradle/build.gradle", gradleTwoDeps, gradleOneDep},
			{"docker/Dockerfile", "FROM golang:1.22\nEXPOSE 8080\nEXPOSE 9090\n", "FROM golang:1.22\nEXPOSE 8080\n"},
		}
		for _, e := range edits {
			f.write(e.rel, e.before)
		}
		f.seed()
		for _, e := range edits {
			f.watchModify(f.write(e.rel, e.after))
		}

		got := map[string]int{}
		for _, id := range f.rec.IDs() {
			got[typeOf(id)]++
		}
		// The go module changed and reconciles. The projects, the image and a
		// dependency (github.com/a/b, now indirect) changed under IDs other
		// manifests can share (#348), so none of them may reconcile.
		if got["gomod"] != 1 || len(got) != 1 {
			t.Errorf("reconciled types %v; want the go module only", got)
		}
	})

	t.Run("roots sharing one project never reconcile", func(t *testing.T) {
		f := newCfgFixture(t, t.TempDir(), t.TempDir())
		f.write("gomod/go.mod", goModTwoDeps)
		f.seed()
		f.watchModify(f.write("gomod/go.mod", goModOneDep))
		if ids := f.rec.IDs(); len(ids) != 0 {
			t.Errorf("reconciled %v though two roots can write one entity ID (#349)", ids)
		}
	})
}
