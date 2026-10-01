package sourcelifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// BucketName is product operational state, intentionally outside the graph catalog.
const BucketName = "SEMSOURCE_SOURCE_LIFECYCLE"

type journalKV interface {
	Get(context.Context, string) (*natsclient.KVEntry, error)
	Create(context.Context, string, []byte) (uint64, error)
	Update(context.Context, string, []byte, uint64) (uint64, error)
	KeysByPrefix(context.Context, string) ([]string, error)
}

// KVJournal validates binding on every operational read and write.
type KVJournal struct {
	kv        journalKV
	authority entityid.Authority
	namespace string
}

// OpenJournal provisions no-lifecycle retained storage and independently verifies
// authoritative backing-stream policy before allowing source-management writes.
func OpenJournal(ctx context.Context, client *natsclient.Client, authority entityid.Authority, namespace string, replicas int) (*KVJournal, error) {
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	if namespace == "" || replicas < 1 {
		return nil, errors.New("journal namespace and positive replicas required")
	}
	bucket, err := natsclient.EnsureFrameworkBucket(ctx, client, natsclient.BucketSpec{Name: BucketName, Owner: "semsource.source-manifest", Class: natsclient.ClassOperational, Retention: natsclient.RetentionPolicy{Kind: natsclient.RetentionNoLifecycleStrict}, Write: natsclient.WriteOwnerOnly, Posture: natsclient.PostureOwnerCreates, History: 1, Replicas: replicas})
	if err != nil {
		return nil, fmt.Errorf("acquire source journal: %w", err)
	}
	stream, err := client.GetStream(ctx, "KV_"+BucketName)
	if err != nil {
		return nil, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateJournalPolicy(info.Config, replicas); err != nil {
		return nil, err
	}
	return &KVJournal{kv: client.NewKVStore(bucket), authority: authority, namespace: namespace}, nil
}
func validateJournalPolicy(c jetstream.StreamConfig, replicas int) error {
	if c.Storage != jetstream.FileStorage || c.Replicas != replicas || c.MaxMsgs > 0 || c.MaxBytes > 0 || c.MaxAge != 0 || c.MaxMsgsPerSubject != 1 {
		return fmt.Errorf("source journal requires file storage, replicas=%d, history=1 and no global eviction", replicas)
	}
	return nil
}
func token(value string) (string, error) { return natsclient.EncodeKVOpaqueToken([]byte(value)) }
func (j *KVJournal) prefix(kind string) (string, error) {
	a, err := token(j.authority.Org + "." + j.authority.Platform)
	return kind + "." + a + ".", err
}
func (j *KVJournal) recordKey(handle string) (string, error) {
	p, err := j.prefix("source")
	if err != nil {
		return "", err
	}
	h, err := token(handle)
	return p + h, err
}
func (j *KVJournal) validateBinding(b sourceintent.Binding) error {
	if b.Authority != j.authority || b.Namespace != j.namespace || b.Handle == "" || b.Factory == "" || b.ConfigDigest == "" || b.Generation == 0 {
		return &sourceintent.Blocker{Code: sourceintent.CodeOwnership, Message: "invalid source journal binding"}
	}
	return nil
}
func (j *KVJournal) validateRecord(r sourceintent.Record) error {
	if r.Version != sourceintent.Version {
		return &sourceintent.Blocker{Code: sourceintent.CodeCorruptRecord, Message: "unsupported source journal version"}
	}
	if err := j.validateBinding(r.Binding); err != nil {
		return err
	}
	if r.Operation != sourceintent.Remove && r.Operation != sourceintent.Reactivate {
		return errors.New("unknown source operation")
	}
	switch r.Phase {
	case sourceintent.Prepared, sourceintent.Pending, sourceintent.Complete, sourceintent.Superseded:
		return j.validateEffect(r)
	default:
		return errors.New("unknown source phase")
	}
}
func mapKVError(err error) error {
	if errors.Is(err, natsclient.ErrKVKeyNotFound) {
		return sourceintent.ErrNotFound
	}
	if errors.Is(err, natsclient.ErrKVKeyExists) || errors.Is(err, natsclient.ErrKVRevisionMismatch) {
		return sourceintent.ErrConflict
	}
	return err
}

// Get returns a validated exact record and its compare-and-swap revision.
func (j *KVJournal) Get(ctx context.Context, handle string) (sourceintent.Record, uint64, error) {
	key, err := j.recordKey(handle)
	if err != nil {
		return sourceintent.Record{}, 0, err
	}
	entry, err := j.kv.Get(ctx, key)
	if err != nil {
		return sourceintent.Record{}, 0, mapKVError(err)
	}
	var r sourceintent.Record
	if err := json.Unmarshal(entry.Value, &r); err != nil {
		return r, entry.Revision, fmt.Errorf("decode journal: %w", err)
	}
	if err := j.validateRecord(r); err != nil {
		return r, entry.Revision, err
	}
	if r.Binding.Handle != handle {
		return r, entry.Revision, errors.New("journal key/value handle mismatch")
	}
	return r, entry.Revision, nil
}

// Create refuses an existing handle and an unbound or foreign record.
func (j *KVJournal) Create(ctx context.Context, r sourceintent.Record) (uint64, error) {
	return j.write(ctx, r, 0)
}

// Update compares the explicit observed revision; a stale completion cannot win.
func (j *KVJournal) Update(ctx context.Context, r sourceintent.Record, rev uint64) (uint64, error) {
	if rev == 0 {
		return 0, sourceintent.ErrConflict
	}
	current, actual, err := j.Get(ctx, r.Binding.Handle)
	if err != nil {
		return 0, err
	}
	if actual != rev || r.Binding.Generation < current.Binding.Generation || r.Binding.Generation > current.Binding.Generation+1 {
		return 0, sourceintent.ErrConflict
	}
	if unresolvedEffect(current) && (r.Effect == nil || !sameAttempt(*current.Effect, *r.Effect)) {
		return 0, sourceintent.ErrConflict
	}
	return j.write(ctx, r, rev)
}
func (j *KVJournal) write(ctx context.Context, r sourceintent.Record, rev uint64) (uint64, error) {
	if err := j.validateRecord(r); err != nil {
		return 0, err
	}
	key, err := j.recordKey(r.Binding.Handle)
	if err != nil {
		return 0, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}
	var next uint64
	if rev == 0 {
		next, err = j.kv.Create(ctx, key, raw)
	} else {
		next, err = j.kv.Update(ctx, key, raw, rev)
	}
	return next, mapKVError(err)
}

// List scans current records; a corrupt value fails the scan rather than disappearing.
func (j *KVJournal) List(ctx context.Context) ([]sourceintent.Record, error) {
	prefix, err := j.prefix("source")
	if err != nil {
		return nil, err
	}
	keys, err := j.kv.KeysByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	out := make([]sourceintent.Record, 0, len(keys))
	for _, key := range keys {
		encoded := strings.TrimPrefix(key, prefix)
		raw, err := natsclient.DecodeKVOpaqueToken(encoded)
		if err != nil {
			return nil, err
		}
		r, _, err := j.Get(ctx, string(raw))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func (j *KVJournal) epochPrefix(kind string, b sourceintent.Binding) (string, error) {
	if err := j.validateBinding(b); err != nil {
		return "", err
	}
	if b.BootEpoch == "" {
		return "", errors.New("receipt epoch required")
	}
	prefix, err := j.prefix(kind)
	if err != nil {
		return "", err
	}
	for _, part := range []string{b.Handle, strconv.FormatUint(b.Generation, 10), b.BootEpoch} {
		encoded, err := token(part)
		if err != nil {
			return "", err
		}
		prefix += encoded + "."
	}
	return prefix, nil
}
func (j *KVJournal) batchPrefix(kind string, b sourceintent.Binding, batch string) (string, error) {
	if batch == "" {
		return "", errors.New("receipt batch required")
	}
	prefix, err := j.epochPrefix(kind, b)
	if err != nil {
		return "", err
	}
	encoded, err := token(batch)
	return prefix + encoded + ".", err
}
func (j *KVJournal) currentEpoch(ctx context.Context, b sourceintent.Binding) error {
	if err := j.validateBinding(b); err != nil {
		return err
	}
	r, _, err := j.Get(ctx, b.Handle)
	if err != nil {
		return err
	}
	if r.Binding != b || r.Operation != sourceintent.Reactivate {
		return sourceintent.ErrSuperseded
	}
	return nil
}
func (j *KVJournal) immutable(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = j.kv.Create(ctx, key, raw)
	if errors.Is(err, natsclient.ErrKVKeyExists) {
		entry, getErr := j.kv.Get(ctx, key)
		if getErr != nil {
			return getErr
		}
		if bytes.Equal(entry.Value, raw) {
			return nil
		}
		return sourceintent.ErrConflict
	}
	return err
}

// PutReceipt never accepts an unacknowledged or superseded epoch's publication.
func (j *KVJournal) PutReceipt(ctx context.Context, r sourceintent.Receipt) error {
	if !r.Acknowledged || r.EntityID == "" || r.Fingerprint == "" {
		return errors.New("receipt lacks acknowledged source facts")
	}
	if err := j.currentEpoch(ctx, r.Binding); err != nil {
		return err
	}
	p, err := j.batchPrefix("receipt", r.Binding, r.BatchID)
	if err != nil {
		return err
	}
	id, err := token(r.EntityID)
	if err != nil {
		return err
	}
	return j.immutable(ctx, p+id, r)
}

// ListReceipts returns only the exact requested epoch and batch.
func (j *KVJournal) ListReceipts(ctx context.Context, b sourceintent.Binding, batch string) ([]sourceintent.Receipt, error) {
	p, err := j.batchPrefix("receipt", b, batch)
	if err != nil {
		return nil, err
	}
	keys, err := j.kv.KeysByPrefix(ctx, p)
	if err != nil {
		return nil, err
	}
	var out []sourceintent.Receipt
	for _, key := range keys {
		entry, err := j.kv.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		var r sourceintent.Receipt
		if err := json.Unmarshal(entry.Value, &r); err != nil {
			return nil, err
		}
		id, err := token(r.EntityID)
		if err != nil {
			return nil, err
		}
		if r.Binding != b || r.BatchID != batch || !r.Acknowledged || key != p+id {
			return nil, errors.New("receipt key/value binding mismatch")
		}
		out = append(out, r)
	}
	return out, nil
}
