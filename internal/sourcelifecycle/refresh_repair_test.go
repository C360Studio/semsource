package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
)

func TestBootRepairsCommittedReactivationRefreshAfterJournalFailure(t *testing.T) {
	c, j, oldBinding, _ := activeHistory(t)
	ctx := context.Background()
	sealHistory(t, j, oldBinding, "initial", true, true)
	if err := c.ReconcileOnce(ctx); err != nil {
		t.Fatal(err)
	}
	old, _, err := j.Get(ctx, oldBinding.Handle)
	if err != nil {
		t.Fatal(err)
	}
	// Add refreshes the same deterministic handle: desired persistence commits,
	// then journal promotion fails before the next generation can be recorded.
	store := c.store.(*desiredConfig)
	changed := store.components[oldBinding.Handle]
	changed.Config = json.RawMessage(`{"instance_name":"docs","project":"manuals","paths":["/docs","/new-docs"]}`)
	if err := store.PutComponentToKV(ctx, oldBinding.Handle, changed); err != nil {
		t.Fatal(err)
	}
	c.journal = &faultJournal{Journal: j, errorUpdate: errors.New("journal unavailable after desired commit")}
	if err := c.RecordReadd(ctx, oldBinding.Handle); err == nil {
		t.Fatal("journal failure hidden")
	}
	unchanged, _, err := j.Get(ctx, oldBinding.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Binding != oldBinding || unchanged.Seed == nil {
		t.Fatal("crash fixture did not retain old history")
	}
	// Startup recovers only from durable enabled desired facts. It must not fail
	// boot or make the old process's successfully sealed proof eligible again.
	c.journal = j
	if err := c.RepairDesired(ctx); err != nil {
		t.Fatal(err)
	}
	bindings, err := c.BindBoot(ctx)
	if err != nil {
		t.Fatalf("boot rejects committed refresh: %v", err)
	}
	current, _, err := j.Get(ctx, oldBinding.Handle)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ConfigDigest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if current.Binding.Generation != oldBinding.Generation+1 || current.Binding.ConfigDigest != digest || current.Binding.BootEpoch == oldBinding.BootEpoch || bindings[oldBinding.Handle] != current.Binding {
		t.Fatalf("refresh not rebound: %+v", current.Binding)
	}
	if current.Phase != sourceintent.Pending || current.Seed != nil || current.Progress.CompletedCount != 0 {
		t.Fatalf("old proof survives refresh: %+v", current)
	}
	if !sameConfig(current.RetiredConfig, old.RetiredConfig) || current.Scope.ConfigDigest != digest {
		t.Fatal("refresh lost retired source record or current scope identity")
	}
	if seeds, err := j.ListSeeds(ctx, current.Binding); err != nil || len(seeds) != 0 {
		t.Fatalf("old batch eligible: %+v %v", seeds, err)
	}
	if err := j.PutReceipt(ctx, sourceintent.Receipt{Binding: oldBinding, BatchID: "late", EntityID: "old", Fingerprint: "old", Acknowledged: true}); !errors.Is(err, sourceintent.ErrSuperseded) {
		t.Fatalf("old receipt accepted: %v", err)
	}
	if err := c.RepairDesired(ctx); err != nil {
		t.Fatal(err)
	}
	repeated, _, err := j.Get(ctx, oldBinding.Handle)
	if err != nil || repeated.Binding != current.Binding {
		t.Fatalf("repair changed current generation again: %+v %v", repeated.Binding, err)
	}
}
