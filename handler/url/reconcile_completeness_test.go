package urlhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/message"
)

func statementSet(triples []message.Triple) string {
	out := make([]string, 0, len(triples))
	for _, t := range triples {
		out = append(out, fmt.Sprintf("%s %s=%v", t.Subject, t.Predicate, t.Object))
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// TestWatchStateMatchesIngestState holds the url-source completeness check
// (processor/url-source) for the poll path: the page a watch event carries is
// the page IngestEntityStates builds for the same response, and its
// statements carry no timestamp, so graph-ingest stamps the envelope time a
// reconcile fences on.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestWatchStateMatchesIngestState(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := NewWithClient(nil, srv.Client())
	h.authority = testAuthority("acme")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ingested, err := h.IngestEntityStates(ctx, watchSourceConfig{url: srv.URL}, h.authority)
	if err != nil || len(ingested) != 1 {
		t.Fatalf("IngestEntityStates = %d states, %v", len(ingested), err)
	}
	events, err := h.Watch(ctx, watchSourceConfig{url: srv.URL})
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	event := <-events
	if len(event.EntityStates) != 1 {
		t.Fatalf("watch event carries %d states, want 1", len(event.EntityStates))
	}
	got, want := event.EntityStates[0].Triples, ingested[0].Triples
	if statementSet(got) != statementSet(want) {
		t.Errorf("watch page differs from ingest page\n got: %s\nwant: %s", statementSet(got), statementSet(want))
	}
	for _, tr := range got {
		if !tr.Timestamp.IsZero() {
			t.Errorf("%s carries its own timestamp; graph-ingest must stamp it", tr.Predicate)
		}
	}
}
