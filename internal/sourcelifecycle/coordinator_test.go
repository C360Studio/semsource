package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

type desiredConfig struct {
	components map[string]types.ComponentConfig
	onPut      func(string, types.ComponentConfig) error
}

func (s *desiredConfig) GetConfig() *semconfig.SafeConfig {
	return semconfig.NewSafeConfig(&semconfig.Config{Components: s.components})
}
func (s *desiredConfig) PutComponentToKV(_ context.Context, k string, c types.ComponentConfig) error {
	if s.onPut != nil {
		if err := s.onPut(k, c); err != nil {
			return err
		}
	}
	s.components[k] = c
	return nil
}
func (s *desiredConfig) DeleteComponentFromKV(_ context.Context, k string) error {
	delete(s.components, k)
	return nil
}

type fakeTail struct{ err error }

func (f *fakeTail) Settled(context.Context, sourceintent.SourceScope) (sourceintent.TailEvidence, error) {
	return sourceintent.TailEvidence{Proof: sourceintent.TailProofAppliedComplete, Inputs: []sourceintent.InputEvidence{{Input: sourceintent.StreamInput{Stream: "GRAPH", Consumer: "ingest", Filters: []string{"graph.ingest.entity"}}, StreamCreated: time.Unix(1, 0), ConsumerCreated: time.Unix(2, 0)}}}, f.err
}

type fakeProjection struct {
	removals int
	err      error
}

func (p *fakeProjection) ApplyRemoval(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	p.removals++
	return sourceintent.ProjectionResult{Enumerated: 2, Marked: 2, Complete: p.err == nil}, p.err
}
func (p *fakeProjection) ApplyReactivation(context.Context, sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	return sourceintent.ProjectionResult{Complete: true}, nil
}
func coordinatorFixture(t *testing.T) (*Coordinator, *desiredConfig, *KVJournal, *fakeProjection) {
	t.Helper()
	j, _ := testJournal()
	store := &desiredConfig{components: map[string]types.ComponentConfig{"docs": {Name: "doc-source", Type: "processor", Enabled: true, Config: json.RawMessage(`{"instance_name":"docs","project":"manuals","paths":["/docs"]}`)}}}
	p := &fakeProjection{}
	c, err := NewCoordinator(CoordinatorConfig{Authority: entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}, Namespace: "acme", Inputs: []sourceintent.StreamInput{{Stream: "GRAPH", Consumer: "ingest", Filters: []string{"graph.ingest.entity"}}}, Journal: j, Store: store, Tail: &fakeTail{}, Projector: p, RepairManifest: func(context.Context, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return c, store, j, p
}
func TestRemovalIntentBeforeDesiredAck(t *testing.T) {
	c, store, j, _ := coordinatorFixture(t)
	ctx := context.Background()
	store.onPut = func(handle string, cc types.ComponentConfig) error {
		r, _, err := j.Get(ctx, handle)
		if err != nil || r.Phase != sourceintent.Prepared || cc.Enabled {
			t.Fatalf("write preceded prepared intent: record=%+v err=%v enabled=%t", r, err, cc.Enabled)
		}
		return nil
	}
	r, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PersistRemoval(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, _, err := j.Get(ctx, "docs")
	if err != nil || got.Phase != sourceintent.Pending || !got.DesiredCommitted {
		t.Fatalf("intent=%+v err=%v", got, err)
	}
}
func TestRemovalRetirementAndReaddFence(t *testing.T) {
	c, store, j, p := coordinatorFixture(t)
	ctx := context.Background()
	if _, err := c.BindBoot(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PersistRemoval(ctx, r); err != nil {
		t.Fatal(err)
	}
	_ = c.ReconcileOnce(ctx)
	if p.removals != 0 {
		t.Fatal("requesting boot mutated live source")
	}
	cc := store.components["docs"]
	cc.Enabled = true
	store.components["docs"] = cc
	if err := c.RecordReadd(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	next, _, err := j.Get(ctx, "docs")
	if err != nil || next.Operation != sourceintent.Reactivate || next.Binding.Generation != r.Binding.Generation+1 {
		t.Fatalf("replacement=%+v err=%v", next, err)
	}
	_ = c.ReconcileOnce(ctx)
	if p.removals != 0 {
		t.Fatal("superseded removal executed")
	}
}
func TestRemovalPartialWritesRepair(t *testing.T) {
	c, store, j, _ := coordinatorFixture(t)
	ctx := context.Background()
	r, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	c.repair = func(context.Context, string) error { return errors.New("manifest unavailable") }
	if err := c.PersistRemoval(ctx, r); err == nil {
		t.Fatal("partial write succeeded")
	}
	if store.components["docs"].Enabled {
		t.Fatal("disable did not commit")
	}
	c.repair = func(context.Context, string) error { return nil }
	if err := c.RepairDesired(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := j.Get(ctx, "docs")
	if err != nil || got.Phase != sourceintent.Pending {
		t.Fatalf("repair=%+v err=%v", got, err)
	}
}
