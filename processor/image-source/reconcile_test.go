package imagesource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/storage"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	imghandler "github.com/c360studio/semsource/handler/image"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/entitypub/entitypubtest"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semsource/storage/filestore"
)

// recordingStream records every message the publisher puts on the stream.
type recordingStream struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (r *recordingStream) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return r.PublishToStreamWithMsgID(ctx, subject, data, "")
}

func (r *recordingStream) PublishToStreamWithMsgID(_ context.Context, _ string, data []byte, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, append([]byte(nil), data...))
	return nil
}

func (r *recordingStream) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = nil
}

// groupStatements returns, per subject, the sorted group-`source` statements
// recorded since the last reset. Media triples carry no wall-clock value, so
// nothing is excluded.
func (r *recordingStream) groupStatements(t *testing.T) map[string][]string {
	t.Helper()
	owned := map[string]bool{}
	for _, p := range graph.OwnedPredicates() {
		owned[p] = true
	}
	r.mu.Lock()
	msgs := append([][]byte(nil), r.msgs...)
	r.mu.Unlock()
	out := map[string][]string{}
	for _, m := range msgs {
		var v any
		if err := json.Unmarshal(m, &v); err != nil {
			t.Fatalf("decode published message: %v", err)
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

// flakyStore fails every Put whose key contains failPart, once one is set.
// Embedding the interface hides any streaming method, so every write is a Put.
type flakyStore struct {
	storage.Store
	mu       sync.Mutex
	failPart string
}

func (s *flakyStore) failing(part string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failPart = part
}

func (s *flakyStore) Put(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	fail := s.failPart != "" && strings.Contains(key, s.failPart)
	s.mu.Unlock()
	if fail {
		return errors.New("store down")
	}
	return s.Store.Put(ctx, key, data)
}

type reconcileFixture struct {
	t      *testing.T
	ctx    context.Context
	dir    string
	c      *Component
	stream *recordingStream
	store  *flakyStore
	rec    *entitypubtest.RecordingReconciler
	events <-chan handler.ChangeEvent
}

func newReconcileFixture(t *testing.T) *reconcileFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fs, err := filestore.New(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	store := &flakyStore{Store: fs}
	authority := entityid.Authority{Org: "acme", Platform: "test-platform"}
	stream := &recordingStream{}
	rec := &entitypubtest.RecordingReconciler{}
	pub, err := entitypub.New(stream, logger, entitypub.WithReconcile(rec))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })

	dir := t.TempDir()
	return &reconcileFixture{
		t: t, ctx: ctx, dir: dir, stream: stream, store: store, rec: rec,
		c: &Component{
			authority: authority,
			publisher: pub,
			distinct:  entitypub.NewDistinctTracker(),
			logger:    logger,
			handler: imghandler.New(imghandler.WithAuthority(authority),
				imghandler.WithStore(store), imghandler.WithLogger(logger)),
			sourceCfg: &sourceCfg{paths: []string{dir}, watchEnabled: true},
		},
	}
}

// writeImage writes a PNG larger than the thumbnail bound, so the handler
// stores both the original and a thumbnail.
func (f *reconcileFixture) writeImage(name string) {
	f.t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 600, 600))); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, name), buf.Bytes(), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *reconcileFixture) seed() {
	f.t.Helper()
	if err := f.c.ingestOnce(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	f.settle()
}

func (f *reconcileFixture) watch() {
	f.t.Helper()
	events, err := f.c.handler.Watch(f.ctx, f.c.sourceCfg)
	if err != nil || events == nil {
		f.t.Fatalf("start watch: %v", err)
	}
	f.events = events
}

// watchWrite rewrites the image and handles the first watch event that
// carries its entity state.
func (f *reconcileFixture) watchWrite(name string) {
	f.t.Helper()
	f.writeImage(name)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-f.events:
			if len(ev.EntityStates) == 0 {
				continue
			}
			f.c.handleChangeEvent(f.ctx, ev)
			f.settle()
			return
		case <-deadline:
			f.t.Fatalf("no watch event for %s", name)
		}
	}
}

func (f *reconcileFixture) settle() {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	if err := f.c.publisher.WaitSettled(ctx); err != nil {
		f.t.Fatalf("publisher did not settle: %v", err)
	}
}

// TestWatchPublishCarriesTheSeedGroupSet is the image-source completeness
// check for the #336 reconcile: a watch republish carries the same
// group-`source` statements the seed published, storage and thumbnail
// references included.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestWatchPublishCarriesTheSeedGroupSet(t *testing.T) {
	f := newReconcileFixture(t)
	f.writeImage("a.png")
	f.seed()
	seeded := f.stream.groupStatements(t)
	f.stream.reset()

	f.watch()
	f.watchWrite("a.png")
	watched := f.stream.groupStatements(t)

	if len(watched) != 1 {
		t.Fatalf("watch published %d subjects, want the one image", len(watched))
	}
	for subject, got := range watched {
		if strings.Join(got, "\n") != strings.Join(seeded[subject], "\n") {
			t.Errorf("%s: watch group set differs from seed\n got: %v\nwant: %v", subject, got, seeded[subject])
		}
		for _, p := range []string{source.MediaStorageRef, source.MediaThumbnailRef} {
			if !hasPredicate(got, p) {
				t.Errorf("%s: watch publish carries no %s; the fixture is vacuous", subject, p)
			}
		}
	}
}

func hasPredicate(statements []string, predicate string) bool {
	for _, s := range statements {
		if strings.HasPrefix(s, predicate+"=") {
			return true
		}
	}
	return false
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestReconcileOnlyWatchedChangesWithoutStoreFaults(t *testing.T) {
	t.Run("the seed makes no reconcile calls; a watch change reconciles", func(t *testing.T) {
		f := newReconcileFixture(t)
		f.writeImage("a.png")
		f.seed()
		if ids := f.rec.IDs(); len(ids) != 0 {
			t.Fatalf("seed reconciled %v", ids)
		}
		f.watch()
		f.watchWrite("a.png")
		if ids := f.rec.IDs(); len(ids) != 1 || !strings.Contains(ids[0], ".media.image.") {
			t.Errorf("watch change reconciled %v, want the image", ids)
		}
	})

	faults := map[string]struct {
		part    string
		dropped string
	}{
		"original Put fails":  {"/original", source.MediaStorageRef},
		"thumbnail Put fails": {"/thumbnail", source.MediaThumbnailRef},
	}
	for name, fault := range faults {
		t.Run(name, func(t *testing.T) {
			f := newReconcileFixture(t)
			f.writeImage("a.png")
			f.seed()
			f.stream.reset()
			f.watch()
			f.store.failing(fault.part)
			f.watchWrite("a.png")
			for subject, got := range f.stream.groupStatements(t) {
				if hasPredicate(got, fault.dropped) {
					t.Fatalf("%s: faulted publish still carries %s; the fixture is vacuous", subject, fault.dropped)
				}
			}
			if ids := f.rec.IDs(); len(ids) != 0 {
				t.Errorf("a publish missing %s was reconciled: %v", fault.dropped, ids)
			}
		})
	}
}
