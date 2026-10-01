package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

func TestConfigDigestPreservesLargeNumbers(t *testing.T) {
	a := types.ComponentConfig{Name: "doc-source", Config: json.RawMessage(`{"n":9007199254740992}`)}
	b := a
	b.Config = json.RawMessage(`{"n":9007199254740993}`)
	x, _ := ConfigDigest(a)
	y, _ := ConfigDigest(b)
	if x == y {
		t.Fatal("different exact config envelopes alias")
	}
}

type faultJournal struct {
	sourceintent.Journal
	createErr, errorUpdate error
}

func (j *faultJournal) Create(ctx context.Context, r sourceintent.Record) (uint64, error) {
	if j.createErr != nil {
		return 0, j.createErr
	}
	return j.Journal.Create(ctx, r)
}
func (j *faultJournal) Update(ctx context.Context, r sourceintent.Record, rev uint64) (uint64, error) {
	if j.errorUpdate != nil {
		return 0, j.errorUpdate
	}
	return j.Journal.Update(ctx, r, rev)
}
func TestRemovalCrashBoundaries(t *testing.T) {
	for _, point := range []string{"journal_prepare", "before_disable", "after_disable_ambiguous", "manifest", "pending_promotion"} {
		t.Run(point, func(t *testing.T) {
			c, store, j, _ := coordinatorFixture(t)
			ctx := context.Background()
			fault := &faultJournal{Journal: j}
			c.journal = fault
			injected := errors.New("injected write failure")
			if point == "journal_prepare" {
				fault.createErr = injected
			}
			r, err := c.PrepareRemoval(ctx, "docs")
			if point == "journal_prepare" {
				if err == nil || !store.components["docs"].Enabled {
					t.Fatal("prepare failure changed desired source")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch point {
			case "before_disable":
				store.onPut = func(string, types.ComponentConfig) error { return injected }
			case "after_disable_ambiguous":
				store.onPut = func(k string, v types.ComponentConfig) error { store.components[k] = v; return injected }
			case "manifest":
				c.repair = func(context.Context, string) error { return injected }
			case "pending_promotion":
				fault.errorUpdate = injected
			}
			if err := c.PersistRemoval(ctx, r); err == nil {
				t.Fatal("partial operation claimed success")
			}
			store.onPut = nil
			c.repair = func(context.Context, string) error { return nil }
			fault.errorUpdate = nil
			if err := c.RepairDesired(ctx); err != nil {
				t.Fatal(err)
			}
			got, _, err := j.Get(ctx, "docs")
			if err != nil {
				t.Fatal(err)
			}
			if point == "before_disable" {
				if got.Phase != sourceintent.Prepared || !store.components["docs"].Enabled {
					t.Fatal("startup removed an enabled prepared source")
				}
				if err := c.PersistRemoval(ctx, r); err != nil {
					t.Fatal(err)
				}
			} else if got.Phase != sourceintent.Pending {
				t.Fatalf("committed disable not repaired: %+v", got)
			}
		})
	}
}
func prepareRetired(t *testing.T) (*Coordinator, *desiredConfig, *KVJournal, *fakeProjection) {
	t.Helper()
	c, s, j, p := coordinatorFixture(t)
	ctx := context.Background()
	r, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PersistRemoval(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BindBoot(ctx); err != nil {
		t.Fatal(err)
	}
	return c, s, j, p
}
func TestRemovalReplayProofAndErrorRedrive(t *testing.T) {
	c, _, j, p := prepareRetired(t)
	ctx := context.Background()
	p.err = errors.New("mutation reply lost")
	if c.ReconcileOnce(ctx) == nil {
		t.Fatal("partial projection succeeded")
	}
	r, _, _ := j.Get(ctx, "docs")
	if r.Phase != sourceintent.Pending || r.Progress.Blocker == nil {
		t.Fatalf("error hidden: %+v", r)
	}
	p.err = nil
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ = j.Get(ctx, "docs")
	if r.Phase != sourceintent.Complete || p.removals != 2 {
		t.Fatalf("retry did not complete: %+v calls=%d", r, p.removals)
	}
}

type proofTail struct {
	fakeTail
	proof   sourceintent.TailProof
	changed bool
}

func (tail proofTail) Settled(ctx context.Context, s sourceintent.SourceScope) (sourceintent.TailEvidence, error) {
	e, err := tail.fakeTail.Settled(ctx, s)
	e.Proof = tail.proof
	if tail.changed {
		e.Inputs[0].ConsumerCreated = e.Inputs[0].ConsumerCreated.Add(1)
	}
	return e, err
}
func TestCurrentRetainedPassRemainsPending(t *testing.T) {
	c, _, j, p := prepareRetired(t)
	c.tail = proofTail{proof: sourceintent.TailProofCurrentRetained}
	for i := 0; i < 2; i++ {
		err := c.ReconcileOnce(context.Background())
		var b *sourceintent.Blocker
		if !errors.As(err, &b) || b.Code != sourceintent.CodeAppliedTailUnproven {
			t.Fatalf("err=%v", err)
		}
	}
	r, _, _ := j.Get(context.Background(), "docs")
	if r.Phase != sourceintent.Pending || p.removals != 2 || r.Progress.LastProgress.IsZero() {
		t.Fatalf("pending progress missing %+v calls=%d", r, p.removals)
	}
}
func TestRemovalBarrierDefersUnsafeProjection(t *testing.T) {
	for _, point := range []string{"changed_consumer", "unknown_proof", "backlog", "sibling", "desired_replaced"} {
		t.Run(point, func(t *testing.T) {
			c, s, _, p := prepareRetired(t)
			switch point {
			case "changed_consumer":
				c.tail = proofTail{proof: sourceintent.TailProofCurrentRetained, changed: true}
			case "unknown_proof":
				c.tail = proofTail{}
			case "backlog":
				c.tail = &fakeTail{err: blocker(sourceintent.CodeTailBacklog, "pending")}
			case "sibling":
				cc := s.components["docs"]
				cc.Enabled = true
				s.components["other"] = cc
			case "desired_replaced":
				cc := s.components["docs"]
				cc.Config = json.RawMessage(`{"paths":["/different"]}`)
				s.components["docs"] = cc
			}
			if c.ReconcileOnce(context.Background()) == nil || p.removals != 0 {
				t.Fatal("unsafe projection executed")
			}
		})
	}
}
func TestReactivationReceiptEpochAndManifest(t *testing.T) {
	c, s, j, _ := prepareRetired(t)
	ctx := context.Background()
	cc := s.components["docs"]
	cc.Enabled = true
	s.components["docs"] = cc
	if err := c.RecordReadd(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	bindings, err := c.BindBoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := bindings["docs"]
	id := "acme.test-a1b2c3.manuals.web.doc.manual"
	payload := &graph.EntityPayload{ID: id, IndexingProfileHint: graph.IndexingProfileContent, TripleData: []message.Triple{{Subject: id, Predicate: "source.doc.file-path", Object: "manual.md"}}}
	fp, err := sourceintent.PublicationFingerprint(payload)
	if err != nil {
		t.Fatal(err)
	}
	m := sourceintent.SeedManifest{Binding: b, BatchID: "initial", Initial: true, Successful: true, Entries: []sourceintent.ManifestEntry{{EntityID: id, Fingerprint: fp}}}
	m.Digest, _ = sourceintent.ManifestDigest(m.Entries)
	if err := c.SeedFinished(ctx, m); err != nil {
		t.Fatal(err)
	}
	if c.ReconcileOnce(ctx) == nil {
		t.Fatal("manifest without receipt granted freshness")
	}
	if err := c.PublisherObserver().Published(ctx, sourceintent.Publication{Binding: b, BatchID: "initial", Initial: true, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ := j.Get(ctx, "docs")
	if r.Phase != sourceintent.Complete {
		t.Fatalf("reactivation incomplete: %+v", r)
	}
	// A newer remove invalidates the old observer permanently, without turning
	// stale evidence into a retry loop or freshness for the new generation.
	if _, err := c.PrepareRemoval(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	if err := c.Published(ctx, sourceintent.Publication{Binding: b, BatchID: "initial", Payload: payload}); !errors.Is(err, sourceintent.ErrSuperseded) {
		t.Fatalf("old receipt=%v", err)
	}
}
