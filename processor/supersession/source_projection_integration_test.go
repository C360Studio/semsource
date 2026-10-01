//go:build integration

package supersession

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
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
	result, err := (sourcelifecycle.NATSProjector{Client: tc.Client}).ApplyRemoval(ctx, replayRemoval())
	var blocked *sourceintent.Blocker
	if !errors.As(err, &blocked) || blocked.Code != sourceintent.CodeOwnership || result.Complete || len(mut.writes) != 0 || len(queries.cursors) != 0 {
		t.Fatalf("wire authorized effects: result=%+v error=%v", result, err)
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
	sub, err := tc.Client.SubscribeForRequests(ctx, sourcelifecycle.ProjectionSubject, func(serverCtx context.Context, raw []byte) ([]byte, error) {
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
	request := replayRemoval()
	raw, err := json.Marshal(sourcelifecycle.ProjectionRequest{Removal: &request})
	if err != nil {
		t.Fatal(err)
	}
	callerCtx, callerCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := tc.Client.RequestClassified(callerCtx, sourcelifecycle.ProjectionSubject, raw, time.Second)
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
	// graph enumeration or mutation. Actual generation admission is coordinator-owned.
	if len(mut.writes) != 0 || len(queries.cursors) != 0 {
		t.Fatalf("late request produced effects: writes=%v queries=%v", mut.writes, queries.cursors)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
