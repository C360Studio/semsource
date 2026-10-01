//go:build integration

package supersession

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
)

func TestIntegrationSourceProjectionOwnsRefusalRPC(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	queries := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}
	mut := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
	c := replayComponent(queries, mut)
	c.client = tc.Client
	if _, _, _, err := c.subscribeHandlers(ctx); err != nil {
		t.Fatal(err)
	}
	if c.sourceSub == nil {
		t.Fatal("refusal subscription not retained")
	}
	raw, err := tc.Client.RequestClassified(ctx, sourceProjectionSubject, []byte(`{"removal":{}}`), time.Second)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "SOURCE_LIFECYCLE_UNAVAILABLE" || len(mut.writes) != 0 || len(queries.cursors) != 0 {
		t.Fatalf("wire authorized effects: %s", raw)
	}

	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if c.sourceSub != nil || c.running {
		t.Fatal("refusal subscription did not drain")
	}
}

func TestIntegrationCanceledQueuedProjectionCannotMutate(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	queries := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}
	mut := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
	c := replayComponent(queries, mut)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	sub, err := tc.Client.SubscribeForRequests(ctx, sourceProjectionSubject, func(serverCtx context.Context, raw []byte) ([]byte, error) {
		close(entered)
		select {
		case <-release:
		case <-serverCtx.Done():
			return nil, serverCtx.Err()
		}
		reply, err := c.handleSourceProjection(serverCtx, raw)
		close(finished)
		return reply, err
	})
	if err != nil {
		t.Fatal(err)
	}
	c.sourceSub = sub
	raw := []byte(`{"removal":{}}`)
	callerCtx, callerCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := tc.Client.RequestClassified(callerCtx, sourceProjectionSubject, raw, time.Second)
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("wire request was not queued")
	}
	callerCancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("canceled caller received success")
		}
	case <-ctx.Done():
		t.Fatal("caller cancellation not observed")
	}
	expired, expire := context.WithCancel(ctx)
	expire()
	stopErr := c.Stop(expired)
	retained := c.sourceSub != nil
	close(release)
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("delayed handler did not finish")
	}
	if stopErr == nil || !retained {
		t.Fatalf("incomplete drain lost ownership: %v retained=%v", stopErr, retained)
	}
	// Even after its original caller has returned, the old request cannot enter
	// graph enumeration or mutation. The retired route has no mutation capability.
	if len(mut.writes) != 0 || len(queries.cursors) != 0 {
		t.Fatalf("late request produced effects: writes=%v queries=%v", mut.writes, queries.cursors)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
