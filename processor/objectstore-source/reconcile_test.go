package objectstoresource

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/entitypub/entitypubtest"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semsource/storage/s3store"
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
// recorded since the last reset. Document and passage triples carry no
// wall-clock value, so nothing is excluded.
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

// memStore is a bucket whose objects a test can put while the source polls.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	etags   map[string]string
	writes  int
}

func (m *memStore) put(key, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writes++
	m.objects[key] = []byte(content)
	m.etags[key] = fmt.Sprintf("etag-%d", m.writes)
}

func (m *memStore) Objects(_ context.Context, prefix string) ([]s3store.ObjectInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []s3store.ObjectInfo{}
	for key, body := range m.objects {
		if strings.HasPrefix(key, prefix) {
			out = append(out, s3store.ObjectInfo{Key: key, ETag: m.etags[key], Size: int64(len(body))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *memStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.objects[key]...), nil
}

type reconcileFixture struct {
	t         *testing.T
	ctx       context.Context
	c         *Component
	bucket    *memStore
	stream    *recordingStream
	rec       *entitypubtest.RecordingReconciler
	authority entityid.Authority
	events    <-chan handler.ChangeEvent
}

func newReconcileFixture(t *testing.T) *reconcileFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bodies, err := filestore.New(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	stream := &recordingStream{}
	rec := &entitypubtest.RecordingReconciler{}
	pub, err := entitypub.New(stream, discardLogger(), entitypub.WithReconcile(rec))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })

	authority := entityid.Authority{Org: "acme", Platform: "test"}
	bucket := &memStore{objects: map[string][]byte{}, etags: map[string]string{}}
	h := objectstore.New(bucket,
		dochandler.New(dochandler.WithBodyStore(bodies, "objectstore-1")), authority,
		objectstore.WithPollInterval(10*time.Millisecond))
	c := &Component{
		logger:    discardLogger(),
		publisher: pub,
		distinct:  entitypub.NewDistinctTracker(),
		handler:   h,
		system:    h.System("artifacts"),
		sourceCfg: &sourceCfg{url: objectstore.SourceURL("artifacts", "reports/"), watchEnabled: true},
	}
	c.config.Org = "acme"
	c.config.Bucket = "artifacts"
	c.config.InstanceName = "objectstore-1"
	return &reconcileFixture{t: t, ctx: ctx, c: c, bucket: bucket, stream: stream, rec: rec, authority: authority}
}

const twoSections = "# Q3\n\n## Revenue\n\nRevenue grew.\n\n## Costs\n\nCosts fell.\n"

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

// putAndPoll puts an object and handles the poll event that carries it.
func (f *reconcileFixture) putAndPoll(key, content string) handler.ChangeOperation {
	f.t.Helper()
	f.bucket.put(key, content)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-f.events:
			if ev.Path != key {
				continue
			}
			f.c.handleChangeEvent(f.ctx, ev)
			f.settle()
			return ev.Operation
		case <-deadline:
			f.t.Fatalf("no poll event for %s", key)
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

// TestPollPublishCarriesTheSeedGroupSet is the objectstore-source
// completeness check for the #336 reconcile: an object re-uploaded with the
// same bytes is republished by the poll with the same group-`source`
// statements the seed published, for the document and every passage.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestPollPublishCarriesTheSeedGroupSet(t *testing.T) {
	f := newReconcileFixture(t)
	f.bucket.put("reports/q3.md", twoSections)
	f.seed()
	seeded := f.stream.groupStatements(t)
	f.stream.reset()

	f.watch()
	f.putAndPoll("reports/q3.md", twoSections)
	polled := f.stream.groupStatements(t)

	if len(polled) < 2 || len(polled) != len(seeded) {
		t.Fatalf("poll published %d subjects, seed %d; want the document and its passages", len(polled), len(seeded))
	}
	for subject, got := range polled {
		if strings.Join(got, "\n") != strings.Join(seeded[subject], "\n") {
			t.Errorf("%s: poll group set differs from seed\n got: %v\nwant: %v", subject, got, seeded[subject])
		}
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestReconcileOnlyModifiedObjects(t *testing.T) {
	f := newReconcileFixture(t)
	f.bucket.put("reports/q3.md", twoSections)
	f.seed()
	if ids := f.rec.IDs(); len(ids) != 0 {
		t.Fatalf("seed reconciled %v", ids)
	}

	f.watch()
	if op := f.putAndPoll("reports/q4.md", twoSections); op != handler.OperationCreate {
		t.Fatalf("new key polled as %v, want create", op)
	}
	if ids := f.rec.IDs(); len(ids) != 0 {
		t.Fatalf("a key this process never saw was reconciled: %v", ids)
	}

	// The edit drops a section: the reconcile is what removes it.
	if op := f.putAndPoll("reports/q3.md", "# Q3\n\n## Revenue\n\nRevenue grew.\n"); op != handler.OperationModify {
		t.Fatalf("edited key polled as %v, want modify", op)
	}
	doc := dochandler.DocumentEntityID(f.authority, f.c.system, "reports/q3.md")
	ids := f.rec.IDs()
	found := false
	for _, id := range ids {
		found = found || id == doc
		if strings.Contains(id, "q4") {
			t.Errorf("reconciled an entity of the unmodified object: %s", id)
		}
	}
	if !found {
		t.Errorf("modified document %s not reconciled; reconciled %v", doc, ids)
	}
}
