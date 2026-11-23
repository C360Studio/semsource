package urlsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	urlhandler "github.com/c360studio/semsource/handler/url"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/entitypub/entitypubtest"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// recordingTransport keeps every message the publisher writes to the stream.
type recordingTransport struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (r *recordingTransport) PublishToStream(_ context.Context, _ string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, append([]byte(nil), data...))
	return nil
}

func (r *recordingTransport) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return r.PublishToStream(ctx, subject, data)
}

// page is the content the fixture's server returns; tests swap it between
// fetches.
type page struct {
	body, etag string
}

type urlFixture struct {
	t      *testing.T
	ctx    context.Context
	c      *Component
	client *recordingTransport
	srv    *httptest.Server

	mu      sync.Mutex
	current page
}

func newURLFixture(t *testing.T, opts ...entitypub.Option) *urlFixture {
	t.Helper()
	f := &urlFixture{t: t, client: &recordingTransport{}, current: page{body: "first", etag: `"v1"`}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		p := f.current
		f.mu.Unlock()
		if p.etag != "" {
			w.Header().Set("ETag", p.etag)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, p.body)
	}))
	t.Cleanup(f.srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f.ctx = ctx
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub, err := entitypub.New(f.client, logger, opts...)
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	f.c = &Component{
		authority: entityid.Authority{Org: "acme", Platform: "test"},
		config:    Config{URLs: []string{f.srv.URL}, PollInterval: "1h"},
		logger:    logger,
		publisher: pub,
		distinct:  entitypub.NewDistinctTracker(),
		handler:   urlhandler.NewWithClient(logger, f.srv.Client()),
	}
	return f
}

func (f *urlFixture) serve(p page) {
	f.mu.Lock()
	f.current = p
	f.mu.Unlock()
}

// poll delivers what the watcher sends for a fetch: it builds the page from
// the server's current response with the handler's own builder.
func (f *urlFixture) poll(op handler.ChangeOperation) {
	f.t.Helper()
	states, err := f.c.handler.IngestEntityStates(f.ctx, &sourceCfg{rawURL: f.srv.URL}, f.c.authority)
	if err != nil {
		f.t.Fatal(err)
	}
	f.c.handleChangeEvent(f.ctx, handler.ChangeEvent{Path: f.srv.URL, Operation: op, EntityStates: states})
}

func (f *urlFixture) seed() {
	f.t.Helper()
	if err := f.c.ingestAll(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func (f *urlFixture) settle() {
	f.t.Helper()
	if err := f.c.publisher.WaitSettled(f.ctx); err != nil {
		f.t.Fatalf("publisher did not settle: %v", err)
	}
}

// groupStatements returns the sorted group-`source` statements per subject
// in the messages written since the last reset. url-source emits no
// wall-clock predicate, so none is excluded.
func (f *urlFixture) groupStatements() map[string][]string {
	f.t.Helper()
	owned := map[string]bool{}
	for _, p := range graph.OwnedPredicates() {
		owned[p] = true
	}
	f.client.mu.Lock()
	msgs := append([][]byte(nil), f.client.msgs...)
	f.client.msgs = nil
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

func predicates(statements []string) []string {
	out := make([]string, 0, len(statements))
	for _, s := range statements {
		out = append(out, strings.SplitN(s, "=", 2)[0])
	}
	return out
}

// TestChangedPagePublishCarriesTheCompleteGroupSet is the url-source
// completeness check for the #336 reconcile: when a page changes to content
// the handler describes with less (no ETag), the change publish still
// carries every other group-`source` predicate the seed carried.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestChangedPagePublishCarriesTheCompleteGroupSet(t *testing.T) {
	f := newURLFixture(t)
	f.seed()
	f.settle()
	seeded := f.groupStatements()
	if len(seeded) != 1 {
		t.Fatalf("seed published %d subjects, want the one page: %v", len(seeded), seeded)
	}

	f.serve(page{body: "second"})
	f.poll(handler.OperationModify)
	f.settle()
	changed := f.groupStatements()

	for subject, want := range seeded {
		var wantPreds []string
		for _, p := range predicates(want) {
			if p != source.WebETag {
				wantPreds = append(wantPreds, p)
			}
		}
		got := predicates(changed[subject])
		if strings.Join(got, ",") != strings.Join(wantPreds, ",") {
			t.Errorf("%s: change publish group predicates\n got: %v\nwant: %v", subject, got, wantPreds)
		}
	}
}

// TestReconcileOnlyPagesThisProcessSawChange pins the owner ruling (task
// 5.2) for url-source: the first pass and the watcher's first fetch of
// unchanged content reconcile nothing; a later fetch whose content differs
// reconciles the page.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestReconcileOnlyPagesThisProcessSawChange(t *testing.T) {
	rec := &entitypubtest.RecordingReconciler{}
	f := newURLFixture(t, entitypub.WithReconcile(rec))
	f.seed()
	f.poll(handler.OperationCreate)
	f.settle()
	if ids := rec.IDs(); len(ids) != 0 {
		t.Fatalf("first pass reconciled %v without a content change", ids)
	}

	f.serve(page{body: "second"})
	f.poll(handler.OperationModify)
	f.settle()
	ids := rec.IDs()
	if len(ids) != 1 || !strings.Contains(ids[0], ".web.page.") {
		t.Fatalf("reconciled %v; want the changed page once", ids)
	}
}
