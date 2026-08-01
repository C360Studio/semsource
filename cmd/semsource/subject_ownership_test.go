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
// against SemSource's DECLARATION of the substrate surface
// (graphQueryInputPorts), not against the substrate's own handler
// registrations, which semstreams keeps unexported (setupQueryHandlers). A
// substrate release that adds a subject SemSource already claims stays green
// here until that declaration is updated. Tracked as an upstream ask.
func TestNoSemSourceSubjectCollidesWithTheSubstrate(t *testing.T) {
	substrate := make(map[string]bool)
	for _, port := range graphQueryInputPorts() {
		subject, ok := port["subject"].(string)
		if !ok || subject == "" {
			t.Fatalf("graph-query input port has no subject: %v", port)
		}
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
