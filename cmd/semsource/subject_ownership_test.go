package main

import (
	"sort"
	"testing"

	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semsource/processor/supersession"
)

// TestNoSemSourceSubjectCollidesWithTheSubstrate pins subject ownership.
//
// SemSource and the pinned SemStreams target run in the SAME process. NATS
// request/reply with two subscribers on one subject is not load balancing: both
// handlers receive the request, both publish to the reply inbox, and the
// requester keeps whichever arrives first and discards the other. When the two
// return different payload shapes — as source-manifest's SummaryPayload and the
// substrate's SummaryData do — the subject has no contract at all.
//
// That is exactly what happened with graph.query.summary, and nothing caught it:
// the only test that touched the subject starts graph-query alone, so no handler
// ever competed. It took booting a real stack to see it.
//
// LIMIT, stated rather than assumed: this compares SemSource's subscriptions
// against a PINNED declaration of the substrate surface (the per-operation
// subjects graph-query's setupQueryHandlers registers on beta.160, read from
// the module source — semstreams keeps the registration unexported). On
// beta.160 the component's PORT declares the graph.query.* family, but its
// actual subscriptions are per-operation, so a non-operation subject under
// graph.query.* (our versionDiff) does not race replies. A substrate release
// that adds an operation SemSource already claims stays green here until this
// pin is updated — refresh it on every semstreams bump.
func TestNoSemSourceSubjectCollidesWithTheSubstrate(t *testing.T) {
	// Pinned from semstreams@v1.0.0-beta.160 processor/graph-query (query.go
	// operation table) plus the prefix operation served from the index side.
	substrateSubjects := []string{
		"graph.query.batch",
		"graph.query.entity",
		"graph.query.entityByAlias",
		"graph.query.globalSearch",
		"graph.query.hierarchyStats",
		"graph.query.localSearch",
		"graph.query.pathSearch",
		"graph.query.prefix",
		"graph.query.relationships",
		"graph.query.semantic",
		"graph.query.similar",
		"graph.query.spatial",
		"graph.query.temporal",
		"graph.query.byName",
	}
	substrate := make(map[string]bool)
	for _, subject := range substrateSubjects {
		substrate[subject] = true
	}
	if len(substrate) == 0 {
		t.Fatal("no substrate query subjects declared — the guard would pass vacuously")
	}

	claims := map[string][]string{
		"source-manifest": sourcemanifest.RequestSubjects(),
		"supersession":    supersession.RequestSubjects(),
	}

	var contested []string
	for component, subjects := range claims {
		for _, subject := range subjects {
			if substrate[subject] {
				contested = append(contested,
					subject+" (claimed by semsource "+component+" and served by the substrate's graph-query)")
			}
		}
	}

	if len(contested) > 0 {
		sort.Strings(contested)
		for _, c := range contested {
			t.Errorf("contested request subject: %s", c)
		}
		t.Fatalf("%d SemSource subject(s) collide with the substrate query surface; "+
			"a requester gets whichever handler replies first", len(contested))
	}
}

// TestSemSourceSubjectsAreDeclaredNonEmpty guards the guard: if a component's
// RequestSubjects list is emptied, the collision check above would pass
// vacuously.
func TestSemSourceSubjectsAreDeclaredNonEmpty(t *testing.T) {
	if len(sourcemanifest.RequestSubjects()) == 0 {
		t.Error("source-manifest declares no request subjects")
	}
	if len(supersession.RequestSubjects()) == 0 {
		t.Error("supersession declares no request subjects")
	}
}
