package sourcelifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

type memoryKV struct {
	entries  map[string]*natsclient.KVEntry
	revision uint64
}

func (m *memoryKV) Get(_ context.Context, k string) (*natsclient.KVEntry, error) {
	e := m.entries[k]
	if e == nil {
		return nil, natsclient.ErrKVKeyNotFound
	}
	v := *e
	v.Value = append([]byte(nil), e.Value...)
	return &v, nil
}
func (m *memoryKV) Create(_ context.Context, k string, v []byte) (uint64, error) {
	if m.entries[k] != nil {
		return 0, natsclient.ErrKVKeyExists
	}
	return m.set(k, v), nil
}
func (m *memoryKV) Update(_ context.Context, k string, v []byte, r uint64) (uint64, error) {
	if m.entries[k] == nil || m.entries[k].Revision != r {
		return 0, natsclient.ErrKVRevisionMismatch
	}
	return m.set(k, v), nil
}
func (m *memoryKV) set(k string, v []byte) uint64 {
	m.revision++
	m.entries[k] = &natsclient.KVEntry{Key: k, Value: append([]byte(nil), v...), Revision: m.revision}
	return m.revision
}
func (m *memoryKV) KeysByPrefix(_ context.Context, p string) ([]string, error) {
	var keys []string
	for k := range m.entries {
		if len(k) >= len(p) && k[:len(p)] == p {
			keys = append(keys, k)
		}
	}
	return keys, nil
}
func testJournal() (*KVJournal, *memoryKV) {
	kv := &memoryKV{entries: map[string]*natsclient.KVEntry{}}
	return &KVJournal{kv: kv, authority: entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}, namespace: "acme"}, kv
}
func testRecord() sourceintent.Record {
	return sourceintent.Record{Version: sourceintent.Version, Binding: sourceintent.Binding{Authority: entityid.Authority{Org: "acme", Platform: "test-a1b2c3"}, Namespace: "acme", Handle: "doc-source-manual", Factory: "doc-source", ConfigDigest: "digest", Generation: 1}, Operation: sourceintent.Remove, Phase: sourceintent.Prepared}
}
func TestRemovalJournalRetentionAndCAS(t *testing.T) {
	good := jetstream.StreamConfig{Storage: jetstream.FileStorage, Replicas: 1, MaxMsgs: -1, MaxBytes: -1, MaxAge: 0, MaxMsgsPerSubject: 1}
	if err := validateJournalPolicy(good, 1); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*jetstream.StreamConfig){"memory": func(c *jetstream.StreamConfig) { c.Storage = jetstream.MemoryStorage }, "replicas": func(c *jetstream.StreamConfig) { c.Replicas = 2 }, "eviction": func(c *jetstream.StreamConfig) { c.MaxMsgs = 20 }, "ttl": func(c *jetstream.StreamConfig) { c.MaxAge = 1 }, "bytes": func(c *jetstream.StreamConfig) { c.MaxBytes = 1 }} {
		t.Run(name, func(t *testing.T) {
			bad := good
			edit(&bad)
			if validateJournalPolicy(bad, 1) == nil {
				t.Fatal("accepted unsafe existing policy")
			}
		})
	}
	j, _ := testJournal()
	ctx := context.Background()
	record := testRecord()
	rev, err := j.Create(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	record.Phase = sourceintent.Pending
	if _, err = j.Update(ctx, record, rev); err != nil {
		t.Fatal(err)
	}
	record.Phase = sourceintent.Complete
	if _, err = j.Update(ctx, record, rev); !errors.Is(err, sourceintent.ErrConflict) {
		t.Fatalf("stale completion=%v", err)
	}
	got, _, err := j.Get(ctx, record.Binding.Handle)
	if err != nil || got.Phase != sourceintent.Pending {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	record.Binding.Authority.Org = "foreign"
	if _, err = j.Create(ctx, record); err == nil {
		t.Fatal("accepted foreign authority")
	}
}
func TestGateHonorsCancellation(t *testing.T) {
	var gate Gate
	release, err := gate.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gate.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire=%v", err)
	}
}
