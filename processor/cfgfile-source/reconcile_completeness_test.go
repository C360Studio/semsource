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
	source "github.com/c360studio/semsource/source/vocabulary"
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

// cfgFixture is a cfgfile-source built by NewComponent, so its reconcile
// gate is the production one, publishing through a recording transport and a
// recording reconciler. Each root holds one manifest, so the watch event for
// it carries exactly the states a full ingest of the root builds — what the
// handler's watcher sends after re-parsing the whole changed manifest.
type cfgFixture struct {
	t      *testing.T
	ctx    context.Context
	c      *Component
	client *recordingClient
	rec    *entitypubtest.RecordingReconciler
}

func newCfgFixture(t *testing.T, root string) *cfgFixture {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"paths": []string{root}, "org": "acme"})
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
	f := &cfgFixture{t: t, ctx: context.Background(), c: disc.(*Component), client: &recordingClient{},
		rec: &entitypubtest.RecordingReconciler{}}
	pub, err := entitypub.New(f.client, logger, entitypub.WithReconcile(f.rec))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(f.ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	f.c.publisher = pub
	return f
}

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
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

// watchModify delivers the change event for path in root: the manifest
// re-parsed from disk.
func (f *cfgFixture) watchModify(root, path string) {
	f.t.Helper()
	states, err := f.c.handler.IngestEntityStates(f.ctx, &sourceCfg{paths: []string{root}}, f.c.authority)
	if err != nil {
		f.t.Fatalf("build watch states: %v", err)
	}
	if len(states) == 0 {
		f.t.Fatalf("no states for %s; the fixture is vacuous", path)
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
// in every message recorded since the last reset.
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

const twoDeps = "module example.com/svc\n\ngo 1.22\n\nrequire (\n\tgithub.com/a/one v1.0.0\n\tgithub.com/b/two v2.0.0\n)\n"
const oneDep = "module example.com/svc\n\ngo 1.22\n\nrequire github.com/a/one v1.0.0\n"

// The two-declaration POM folds into one dependency carrying both scopes
// (#375); the edit drops the test declaration, so the dependency loses a
// scope value without losing its ID.
const pomTwoScopes = `<project><groupId>g</groupId><artifactId>app</artifactId><version>1</version><dependencies>
<dependency><groupId>org.x</groupId><artifactId>lib</artifactId><version>1.0</version><scope>compile</scope></dependency>
<dependency><groupId>org.x</groupId><artifactId>lib</artifactId><version>1.0</version><scope>test</scope></dependency>
</dependencies></project>`
const pomOneScope = `<project><groupId>g</groupId><artifactId>app</artifactId><version>1</version><dependencies>
<dependency><groupId>org.x</groupId><artifactId>lib</artifactId><version>1.0</version><scope>compile</scope></dependency>
</dependencies></project>`

// A watch republish of a manifest that now emits less carries, for every
// entity it publishes, the complete group set a fresh seed of the new content
// produces, so the reconcile that follows removes only what is gone.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestCfgfileWatchRepublishCarriesTheCompleteGroupSet(t *testing.T) {
	for _, tc := range []struct{ name, file, before, after string }{
		{"go.mod drops a requirement", "go.mod", twoDeps, oneDep},
		{"pom.xml drops a folded scope", "pom.xml", pomTwoScopes, pomOneScope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			f := newCfgFixture(t, root)
			path := writeFile(t, root, tc.file, tc.before)
			f.seed()
			seeded := f.groupStatements()

			writeFile(t, root, tc.file, tc.after)
			f.client.reset()
			f.watchModify(root, path)
			watched := f.groupStatements()

			fresh := newCfgFixture(t, root)
			fresh.seed()
			want := fresh.groupStatements()

			if len(watched) == 0 {
				t.Fatal("watch published nothing; the fixture is vacuous")
			}
			shrank := false
			for subject, got := range watched {
				if strings.Join(got, "\n") != strings.Join(want[subject], "\n") {
					t.Errorf("%s: watch group set differs from a seed of the same content\n got: %v\nwant: %v",
						subject, got, want[subject])
				}
				shrank = shrank || (len(seeded[subject]) > 0 && len(got) < len(seeded[subject]))
			}
			if !shrank {
				t.Error("the edit emitted no fewer statements for any entity; the test proves nothing")
			}
		})
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestCfgfileReconcileOnlyKnownChanges(t *testing.T) {
	t.Run("a first pass, an unchanged republish and a new manifest make no reconcile calls", func(t *testing.T) {
		root := t.TempDir()
		f := newCfgFixture(t, root)
		path := writeFile(t, root, "go.mod", twoDeps)
		f.seed()
		f.watchModify(root, path)

		other := t.TempDir()
		g := newCfgFixture(t, other)
		g.watchModify(other, writeFile(t, other, "package.json", `{"name":"web","version":"1.0.0"}`))
		if ids := append(f.rec.IDs(), g.rec.IDs()...); len(ids) != 0 {
			t.Errorf("reconciled %v without a known content change", ids)
		}
	})

	t.Run("a dropped requirement reconciles the module, which loses its requires edge", func(t *testing.T) {
		root := t.TempDir()
		f := newCfgFixture(t, root)
		path := writeFile(t, root, "go.mod", twoDeps)
		f.seed()
		writeFile(t, root, "go.mod", oneDep)
		f.client.reset()
		f.watchModify(root, path)

		ids := f.rec.IDs()
		if len(ids) != 1 || !strings.Contains(ids[0], ".config.gomod.") {
			t.Fatalf("reconciled %v; want only the go module entity", ids)
		}
		requires := 0
		for _, st := range f.groupStatements()[ids[0]] {
			if strings.HasPrefix(st, source.ConfigRequires+"=") {
				requires++
			}
		}
		if requires != 1 {
			t.Errorf("module publish carries %d requires edges; want 1", requires)
		}
	})
}
