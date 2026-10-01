package sourcelifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/types"
)

type inventoryProjection struct {
	requests []sourceintent.ReactivationProjection
	err      error
}

func (p *inventoryProjection) ApplyRemoval(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	return sourceintent.ProjectionResult{}, errors.New("unexpected remove")
}
func (p *inventoryProjection) ApplyReactivation(_ context.Context, r sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	p.requests = append(p.requests, r)
	return sourceintent.ProjectionResult{Enumerated: len(r.Manifest.Entries), Complete: p.err == nil}, p.err
}
func activeHistory(t *testing.T) (*Coordinator, *KVJournal, sourceintent.Binding, *inventoryProjection) {
	t.Helper()
	c, s, j, _ := prepareRetired(t)
	cc := s.components["docs"]
	cc.Enabled = true
	s.components["docs"] = cc
	if err := c.RecordReadd(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	bindings, err := c.BindBoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := &inventoryProjection{}
	c.projector = p
	return c, j, bindings["docs"], p
}
func sealHistory(t *testing.T, j *KVJournal, b sourceintent.Binding, batch string, initial, success bool) sourceintent.SeedManifest {
	t.Helper()
	m := sourceintent.SeedManifest{Binding: b, BatchID: batch, Initial: initial, Successful: success, Entries: []sourceintent.ManifestEntry{{EntityID: b.Authority.Build("manuals", "web", "doc", batch), Fingerprint: batch}}}
	m.Digest, _ = sourceintent.ManifestDigest(m.Entries)
	if err := j.PutReceipt(context.Background(), sourceintent.Receipt{Binding: b, BatchID: batch, EntityID: m.Entries[0].EntityID, Fingerprint: batch, Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
	if err := j.SealSeed(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	return m
}
func TestCompletedHistoryRenewsEveryBoot(t *testing.T) {
	c, j, b, _ := activeHistory(t)
	ctx := context.Background()
	sealHistory(t, j, b, "initial", true, true)
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	bindings, err := c.BindBoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next, ok := bindings[b.Handle]
	if !ok || next.BootEpoch == b.BootEpoch {
		t.Fatal("completed history lost current producer binding")
	}
	r, _, _ := j.Get(ctx, b.Handle)
	if r.Phase != sourceintent.Pending || r.Seed != nil {
		t.Fatalf("old process seed survives: %+v", r)
	}
	if err := j.PutReceipt(ctx, sourceintent.Receipt{Binding: b, BatchID: "late", EntityID: "old", Fingerprint: "old", Acknowledged: true}); !errors.Is(err, sourceintent.ErrSuperseded) {
		t.Fatalf("old epoch accepted: %v", err)
	}
}
func TestLiveInventoryDoesNotRepairFailedInitial(t *testing.T) {
	c, j, b, p := activeHistory(t)
	sealHistory(t, j, b, "initial", true, false)
	sealHistory(t, j, b, "live", false, true)
	if err := c.ReconcileOnce(context.Background()); err == nil || len(p.requests) != 0 {
		t.Fatal("live batch granted eligibility after failed initial")
	}
}
func TestLiveInventorySurvivesLostWakeAndReopensFailure(t *testing.T) {
	c, j, b, p := activeHistory(t)
	ctx := context.Background()
	initial := sealHistory(t, j, b, "initial", true, true)
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	live := sealHistory(t, j, b, "live", false, true)
	// A seal is durable before the optional pending status update. Recreate that
	// crash boundary: the periodic scan must inspect completed history too.
	r, rev, _ := j.Get(ctx, b.Handle)
	r.Phase = sourceintent.Complete
	if _, err := j.Update(ctx, r, rev); err != nil {
		t.Fatal(err)
	}
	p.requests = nil
	p.err = blocker(sourceintent.CodeConditionalReconcileUnavailable, "pin cannot conditionally clear")
	if c.ReconcileOnce(ctx) == nil {
		t.Fatal("projection blocker hidden")
	}
	if len(p.requests) != 2 {
		t.Fatalf("sealed live batch omitted: %+v", p.requests)
	}
	var found bool
	for _, request := range p.requests {
		if request.Manifest.BatchID == live.BatchID {
			found = true
			if request.InitialManifest == nil || request.InitialManifest.BatchID != initial.BatchID || len(request.InitialReceipts) != 1 {
				t.Fatal("live batch lacks independent initial proof")
			}
		}
	}
	if !found {
		t.Fatal("live batch not projected")
	}
	r, _, _ = j.Get(ctx, b.Handle)
	if r.Phase != sourceintent.Pending || r.Progress.Blocker == nil {
		t.Fatal("completed history hid new failure")
	}
}
func TestListSeedsOnlySealedCurrentEpoch(t *testing.T) {
	c, j, b, _ := activeHistory(t)
	ctx := context.Background()
	sealHistory(t, j, b, "initial", true, true)
	sealHistory(t, j, b, "live", false, true)
	if err := j.PutReceipt(ctx, sourceintent.Receipt{Binding: b, BatchID: "unsealed", EntityID: "id", Fingerprint: "fp", Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
	seeds, err := j.ListSeeds(ctx, b)
	if err != nil || len(seeds) != 2 {
		t.Fatalf("seeds=%+v err=%v", seeds, err)
	}
	bindings, err := c.BindBoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seeds, err = j.ListSeeds(ctx, bindings[b.Handle])
	if err != nil || len(seeds) != 0 {
		t.Fatalf("previous epoch leaked: %+v %v", seeds, err)
	}
}
func TestDisableReceiptReadsCommittedEnvelopeAfterMemoryFailure(t *testing.T) {
	c, s, j, _ := coordinatorFixture(t)
	ctx := context.Background()
	durable := s.components["docs"]
	c.readDesired = func(context.Context, string) (types.ComponentConfig, bool, error) { return durable, true, nil }
	record, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	s.onPut = func(_ string, cc types.ComponentConfig) error {
		durable = cc
		return errors.New("memory apply failed after durable commit")
	}
	if c.PersistRemoval(ctx, record) == nil {
		t.Fatal("partial write claimed success")
	}
	committed, err := c.RemovalCommitted(ctx, record)
	if err != nil || !committed || !s.components["docs"].Enabled {
		t.Fatalf("receipt inferred from stale memory: %v %v", committed, err)
	}
	s.onPut = nil
	if err := c.RepairDesired(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ := j.Get(ctx, "docs")
	if r.Phase != sourceintent.Pending {
		t.Fatal("durable disable not repaired")
	}
}
