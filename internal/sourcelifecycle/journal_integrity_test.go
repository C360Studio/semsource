package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
)

func TestJournalRejectsCorruptBindingAndVersions(t *testing.T) {
	for _, kind := range []string{"schema", "phase", "operation", "foreign", "key_handle", "json"} {
		t.Run(kind, func(t *testing.T) {
			j, kv := testJournal()
			r := testRecord()
			key, _ := j.recordKey(r.Binding.Handle)
			switch kind {
			case "schema":
				r.Version++
			case "phase":
				r.Phase = "unknown"
			case "operation":
				r.Operation = "unknown"
			case "foreign":
				r.Binding.Authority.Org = "other"
			case "key_handle":
				r.Binding.Handle = "different"
			}
			raw, _ := json.Marshal(r)
			if kind == "json" {
				raw = []byte("{")
			}
			kv.set(key, raw)
			if _, _, err := j.Get(context.Background(), "doc-source-manual"); err == nil {
				t.Fatal("corrupt exact read succeeded")
			}
			if _, err := j.List(context.Background()); err == nil {
				t.Fatal("corrupt record disappeared from scan")
			}
		})
	}
}
func receiptJournal(t *testing.T) (*KVJournal, *memoryKV, sourceintent.Binding) {
	t.Helper()
	j, kv := testJournal()
	r := testRecord()
	r.Operation = sourceintent.Reactivate
	r.Binding.BootEpoch = "boot-one"
	if _, err := j.Create(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	return j, kv, r.Binding
}
func TestReceiptAndSeedImmutableAgreement(t *testing.T) {
	j, kv, b := receiptJournal(t)
	ctx := context.Background()
	r := sourceintent.Receipt{Binding: b, BatchID: "initial", EntityID: "acme.test-a1b2c3.manual.web.doc.a", Fingerprint: "one", Acknowledged: true}
	if err := j.PutReceipt(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := j.PutReceipt(ctx, r); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	changed := r
	changed.Fingerprint = "two"
	if err := j.PutReceipt(ctx, changed); !errors.Is(err, sourceintent.ErrConflict) {
		t.Fatalf("contradictory receipt=%v", err)
	}
	unacked := r
	unacked.Acknowledged = false
	if err := j.PutReceipt(ctx, unacked); err == nil {
		t.Fatal("unacked receipt accepted")
	}
	old := r
	old.Binding.BootEpoch = "older"
	if err := j.PutReceipt(ctx, old); !errors.Is(err, sourceintent.ErrSuperseded) {
		t.Fatalf("old receipt=%v", err)
	}
	m := sourceintent.SeedManifest{Binding: b, BatchID: "initial", Initial: true, Successful: true, Entries: []sourceintent.ManifestEntry{{EntityID: r.EntityID, Fingerprint: r.Fingerprint}}}
	m.Digest, _ = sourceintent.ManifestDigest(m.Entries)
	bad := m
	bad.Digest = "wrong"
	if err := j.SealSeed(ctx, bad); err == nil {
		t.Fatal("mismatched seal accepted")
	}
	if _, err := j.LoadSeed(ctx, b, "initial"); !errors.Is(err, sourceintent.ErrNotFound) {
		t.Fatalf("unsealed seed=%v", err)
	}
	if err := j.SealSeed(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := j.SealSeed(ctx, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := j.LoadSeed(ctx, b, "initial")
	if err != nil || loaded.Digest != m.Digest || len(loaded.Entries) != 1 {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	receipts, err := j.ListReceipts(ctx, b, "initial")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}
	// Missing expected entry after a previously sealed manifest must remain visible.
	prefix, _ := j.batchPrefix("manifest", b, "initial")
	id, _ := token(r.EntityID)
	delete(kv.entries, prefix+id)
	if _, err := j.LoadSeed(ctx, b, "initial"); err == nil {
		t.Fatal("partial manifest accepted")
	}
}
func TestReceiptListRejectsForgedBinding(t *testing.T) {
	j, kv, b := receiptJournal(t)
	r := sourceintent.Receipt{Binding: b, BatchID: "initial", EntityID: "entity", Fingerprint: "fp", Acknowledged: true}
	prefix, _ := j.batchPrefix("receipt", b, "initial")
	id, _ := token(r.EntityID)
	r.Binding.Generation++
	raw, _ := json.Marshal(r)
	kv.set(prefix+id, raw)
	if _, err := j.ListReceipts(context.Background(), b, "initial"); err == nil {
		t.Fatal("forged binding accepted")
	}
}
func TestCoordinatorUnknownAndPreparedRetry(t *testing.T) {
	c, s, j, _ := coordinatorFixture(t)
	ctx := context.Background()
	if _, err := c.PrepareRemoval(ctx, "absent"); err == nil {
		t.Fatal("unknown remove accepted")
	}
	r, err := c.PrepareRemoval(ctx, "docs")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := c.PrepareRemoval(ctx, "docs")
	if err != nil || r.Binding.Generation != retry.Binding.Generation {
		t.Fatal("prepared retry created new generation")
	}
	if err := c.PersistRemoval(ctx, r); err != nil {
		t.Fatal(err)
	}
	retry, err = c.PrepareRemoval(ctx, "docs")
	if err != nil || retry.Binding.Generation != r.Binding.Generation {
		t.Fatal("pending retry created new generation")
	}
	s.components["docs"] = r.RetiredConfig
	if err := c.RepairDesired(ctx); err != nil {
		t.Fatal(err)
	}
	next, _, _ := j.Get(ctx, "docs")
	if next.Operation != sourceintent.Reactivate {
		t.Fatal("enabled config did not supersede removal")
	}
}

func TestJournalRejectsGenerationRollback(t *testing.T) {
	j, _, _ := receiptJournal(t)
	ctx := context.Background()
	r, rev, err := j.Get(ctx, "doc-source-manual")
	if err != nil {
		t.Fatal(err)
	}
	r.Binding.Generation++
	rev, err = j.Update(ctx, r, rev)
	if err != nil {
		t.Fatal(err)
	}
	r.Binding.Generation--
	if _, err := j.Update(ctx, r, rev); err == nil {
		t.Fatal("current revision permitted generation rollback")
	}
}
