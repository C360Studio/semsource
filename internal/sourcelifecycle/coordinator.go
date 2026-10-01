package sourcelifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/types"
)

// CoordinatorConfig supplies operation-specific dependencies, never runtime globals.
type CoordinatorConfig struct {
	Authority      entityid.Authority
	Namespace      string
	Journal        sourceintent.Journal
	Store          sourcespawn.ConfigStore
	Tail           sourceintent.TailObserver
	Projector      sourceintent.Projector
	Inputs         []sourceintent.StreamInput
	RepairManifest func(context.Context, string) error
	ReadDesired    func(context.Context, string) (types.ComponentConfig, bool, error)
}

// Coordinator serializes desired intent changes and bounded replay units. Receipt
// storage is deliberately independent so producer drain cannot deadlock this gate.
type Coordinator struct {
	gate        Gate
	authority   entityid.Authority
	namespace   string
	journal     sourceintent.Journal
	store       sourcespawn.ConfigStore
	tail        sourceintent.TailObserver
	projector   sourceintent.Projector
	inputs      []sourceintent.StreamInput
	repair      func(context.Context, string) error
	readDesired func(context.Context, string) (types.ComponentConfig, bool, error)
	boot        map[string]types.ComponentConfig
	bindings    map[string]sourceintent.Binding
}

// NewCoordinator creates no workers and retains no caller context.
func NewCoordinator(cfg CoordinatorConfig) (*Coordinator, error) {
	if err := cfg.Authority.Validate(); err != nil {
		return nil, err
	}
	if cfg.Namespace == "" || cfg.Journal == nil || cfg.Store == nil || cfg.Tail == nil || cfg.Projector == nil || cfg.RepairManifest == nil {
		return nil, errors.New("source lifecycle coordinator dependencies incomplete")
	}
	return &Coordinator{authority: cfg.Authority, namespace: cfg.Namespace, journal: cfg.Journal, store: cfg.Store, tail: cfg.Tail, projector: cfg.Projector, inputs: cfg.Inputs, repair: cfg.RepairManifest, readDesired: cfg.ReadDesired, boot: map[string]types.ComponentConfig{}}, nil
}

// Acquire serializes Add, Remove and one replay unit, honoring request cancellation.
func (c *Coordinator) Acquire(ctx context.Context) (func(), error) { return c.gate.Acquire(ctx) }
func (c *Coordinator) desired(ctx context.Context, handle string) (types.ComponentConfig, bool, error) {
	if c.readDesired != nil {
		return c.readDesired(ctx, handle)
	}
	cfg := c.store.GetConfig().Get()
	if cfg == nil {
		return types.ComponentConfig{}, false, errors.New("desired config unavailable")
	}
	cc, ok := cfg.Components[handle]
	return cc, ok, nil
}
func sameConfig(a, b types.ComponentConfig) bool {
	left, e1 := ConfigDigest(a)
	right, e2 := ConfigDigest(b)
	return e1 == nil && e2 == nil && left == right
}
func blocker(code sourceintent.ErrorCode, message string) *sourceintent.Blocker {
	return &sourceintent.Blocker{Code: code, Message: message, Retryable: true}
}
func asBlocker(err error) *sourceintent.Blocker {
	var b *sourceintent.Blocker
	if errors.As(err, &b) {
		return b
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return blocker(sourceintent.CodeCanceled, err.Error())
	}
	return blocker(sourceintent.CodeStorage, err.Error())
}

// PrepareRemoval captures the old admitted source facts before any desired write.
// Callers hold Acquire through PersistRemoval and their response construction.
func (c *Coordinator) PrepareRemoval(ctx context.Context, handle string) (sourceintent.Record, error) {
	if err := c.CheckAdmission(ctx); err != nil {
		return sourceintent.Record{}, err
	}
	previous, rev, err := c.journal.Get(ctx, handle)
	if err != nil && !errors.Is(err, sourceintent.ErrNotFound) {
		return previous, err
	}
	cc, exists, readErr := c.desired(ctx, handle)
	if readErr != nil {
		return previous, readErr
	}
	if !exists || !IsSourceFactory(cc.Name) {
		return previous, &sourcespawn.Error{Code: sourcespawn.CodeNotFound, Message: "no source component named " + handle}
	}
	if !cc.Enabled {
		if err == nil && previous.Operation == sourceintent.Remove && (previous.Phase == sourceintent.Prepared || previous.Phase == sourceintent.Pending) && sameConfig(cc, previous.RetiredConfig) {
			return previous, nil
		}
		return previous, &sourcespawn.Error{Code: sourcespawn.CodeNotFound, Message: "source is already disabled"}
	}
	if err == nil && previous.Operation == sourceintent.Remove && previous.Phase == sourceintent.Prepared && sameConfig(cc, previous.RetiredConfig) {
		return previous, nil
	}
	scope, err := ScopeFor(c.authority, handle, cc, c.inputs)
	if err != nil {
		return previous, err
	}
	evidence, tailErr := c.tail.Settled(ctx, scope)
	// Preparation may capture a busy old producer, but every stream/consumer
	// identity must already have been read successfully before acknowledging intent.
	if len(evidence.Inputs) != len(scope.Inputs) || len(evidence.Inputs) == 0 {
		if tailErr != nil {
			return previous, tailErr
		}
		return previous, blocker(sourceintent.CodeTailUnavailable, "graph input identities unavailable")
	}
	if tailErr != nil {
		b := asBlocker(tailErr)
		if b.Code != sourceintent.CodeTailBacklog && b.Code != sourceintent.ErrorCode("applied_tail_unproven") {
			return previous, tailErr
		}
	}
	generation := uint64(1)
	if rev > 0 {
		generation = previous.Binding.Generation + 1
	}
	now := time.Now()
	record := sourceintent.Record{Version: sourceintent.Version, Binding: sourceintent.Binding{Authority: c.authority, Namespace: c.namespace, Handle: handle, Generation: generation, Factory: cc.Name, ConfigDigest: scope.ConfigDigest}, Operation: sourceintent.Remove, Phase: sourceintent.Prepared, RetiredConfig: cc, Scope: scope, Tail: evidence, CreatedAt: now, UpdatedAt: now}
	if rev == 0 {
		_, err = c.journal.Create(ctx, record)
	} else {
		_, err = c.journal.Update(ctx, record, rev)
	}
	return record, err
}

// PersistRemoval writes the exact disabled envelope, then repairs the desired
// manifest and promotes the same journal generation. Errors retain prepared work.
func (c *Coordinator) PersistRemoval(ctx context.Context, record sourceintent.Record) error {
	if err := c.CheckAdmission(ctx); err != nil {
		return err
	}
	current, rev, err := c.journal.Get(ctx, record.Binding.Handle)
	if err != nil {
		return err
	}
	if current.Binding != record.Binding {
		return sourceintent.ErrConflict
	}
	cc, exists, err := c.desired(ctx, record.Binding.Handle)
	if err != nil {
		return err
	}
	if !exists || !sameConfig(cc, record.RetiredConfig) {
		return blocker(sourceintent.CodeDesiredAmbiguous, "desired source changed during removal")
	}
	cc.Enabled = false
	if err := c.store.PutComponentToKV(ctx, record.Binding.Handle, cc); err != nil {
		return fmt.Errorf("persist source disable (commit may require repair): %w", err)
	}
	if err := c.repair(ctx, record.Binding.Handle); err != nil {
		return err
	}
	current.Phase = sourceintent.Pending
	current.DesiredCommitted = true
	current.UpdatedAt = time.Now()
	current.Progress.Blocker = nil
	_, err = c.journal.Update(ctx, current, rev)
	return err
}

// RepairDesired resumes only committed exact disables. Prepared enabled sources
// are not silently removed; enabled replacements supersede older removal intent.
func (c *Coordinator) RepairDesired(ctx context.Context) error {
	records, err := c.journal.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		cc, exists, err := c.desired(ctx, r.Binding.Handle)
		if err != nil {
			return err
		}
		if !exists {
			return blocker(sourceintent.CodeDesiredAmbiguous, "journal source has no desired envelope")
		}
		// A same-handle Add refresh may commit before its journal CAS. Renew
		// that reactivation generation from enabled desired facts before boot
		// can reuse the prior configuration's epoch or publication evidence.
		if r.Operation == sourceintent.Reactivate && cc.Enabled && !sameDigest(cc, r.Binding.ConfigDigest) {
			if err := c.RecordReadd(ctx, r.Binding.Handle); err != nil {
				return err
			}
			continue
		}
		if r.Operation == sourceintent.Remove {
			if cc.Enabled {
				if r.Phase == sourceintent.Prepared && sameConfig(cc, r.RetiredConfig) {
					continue
				}
				if err := c.RecordReadd(ctx, r.Binding.Handle); err != nil {
					return err
				}
				continue
			}
			if !sameConfig(cc, r.RetiredConfig) {
				return blocker(sourceintent.CodeDesiredAmbiguous, "disabled source differs from retained removal")
			}
			if r.Phase == sourceintent.Prepared {
				if err := c.PersistRemoval(ctx, r); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RecordReadd runs AFTER the enabled desired envelope commits. Even if this CAS
// fails, the enabled desired source blocks every old removal replay attempt.
func (c *Coordinator) RecordReadd(ctx context.Context, handle string) error {
	if err := c.CheckAdmission(ctx); err != nil {
		return err
	}
	old, rev, err := c.journal.Get(ctx, handle)
	if errors.Is(err, sourceintent.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	cc, exists, err := c.desired(ctx, handle)
	if err != nil {
		return err
	}
	if !exists || !cc.Enabled {
		return blocker(sourceintent.CodeDesiredAmbiguous, "re-add envelope is not enabled")
	}
	digest, err := ConfigDigest(cc)
	if err != nil {
		return err
	}
	if old.Operation == sourceintent.Reactivate && old.Binding.ConfigDigest == digest {
		return nil
	}
	scope, err := ScopeFor(c.authority, handle, cc, c.inputs)
	if err != nil {
		return err
	}
	next := old
	next.Binding.Generation++
	next.Binding.BootEpoch = ""
	next.Binding.ConfigDigest = digest
	next.Binding.Factory = cc.Name
	next.Operation = sourceintent.Reactivate
	next.Phase = sourceintent.Pending
	next.Scope = scope
	next.Seed = nil
	next.Progress = sourceintent.Progress{}
	next.DesiredCommitted = true
	next.ReplacementGeneration = next.Binding.Generation
	next.UpdatedAt = time.Now()
	_, err = c.journal.Update(ctx, next, rev)
	return err
}

// BindBoot copies immutable admission facts and invalidates every prior-process
// reactivation receipt before factories or producer Start can run.
func (c *Coordinator) BindBoot(ctx context.Context) (map[string]sourceintent.Binding, error) {
	snapshot := c.store.GetConfig().Get()
	if snapshot == nil {
		return nil, errors.New("boot config unavailable")
	}
	boot := make(map[string]types.ComponentConfig, len(snapshot.Components))
	for handle, cc := range snapshot.Components {
		cc.Config = append([]byte(nil), cc.Config...)
		boot[handle] = cc
	}
	if err := c.checkBootEffects(ctx, boot); err != nil {
		return nil, err
	}
	c.boot = boot
	records, err := c.journal.List(ctx)
	if err != nil {
		return nil, err
	}
	bindings := map[string]sourceintent.Binding{}
	for _, r := range records {
		if r.Operation != sourceintent.Reactivate {
			continue
		}
		cc, ok := boot[r.Binding.Handle]
		if !ok || !cc.Enabled || !sameDigest(cc, r.Binding.ConfigDigest) {
			return nil, blocker(sourceintent.CodeDesiredAmbiguous, "reactivation does not match boot source")
		}
		entropy := make([]byte, 16)
		if _, err := rand.Read(entropy); err != nil {
			return nil, err
		}
		current, rev, err := c.journal.Get(ctx, r.Binding.Handle)
		if err != nil {
			return nil, err
		}
		if current.Binding != r.Binding {
			return nil, sourceintent.ErrConflict
		}
		r = current
		r.Binding.BootEpoch = hex.EncodeToString(entropy)
		r.Phase = sourceintent.Pending
		r.Seed = nil
		r.Progress = sourceintent.Progress{}
		r.UpdatedAt = time.Now()
		if _, err := c.journal.Update(ctx, r, rev); err != nil {
			return nil, err
		}
		bindings[r.Binding.Handle] = r.Binding
	}
	c.bindings = bindings
	return bindings, nil
}
func sameDigest(cc types.ComponentConfig, digest string) bool {
	got, err := ConfigDigest(cc)
	return err == nil && got == digest
}

// PublisherObserver is independent of replay Stop; the root retains the journal
// and transport until every producer's accepted publication work settles.
func (c *Coordinator) PublisherObserver() sourceintent.PublicationObserver { return c }

// Published records only immutable post-ack current source facts.
func (c *Coordinator) Published(ctx context.Context, p sourceintent.Publication) error {
	if p.Payload == nil {
		return errors.New("nil publication")
	}
	fp, err := sourceintent.PublicationFingerprint(p.Payload)
	if err != nil {
		return err
	}
	return c.journal.PutReceipt(ctx, sourceintent.Receipt{Binding: p.Binding, BatchID: p.BatchID, EntityID: p.Payload.ID, Fingerprint: fp, Acknowledged: true})
}

// SeedFinished persists explicit enumeration and settlement evidence.
func (c *Coordinator) SeedFinished(ctx context.Context, m sourceintent.SeedManifest) error {
	return c.journal.SealSeed(ctx, m)
}

// Status returns typed durable records. A storage failure remains an error rather
// than masquerading as an empty or healthy source lifecycle queue.
func (c *Coordinator) Status(ctx context.Context) ([]sourceintent.Record, error) {
	return c.journal.List(ctx)
}

// RemovalCommitted reads the authoritative retained desired envelope after a
// potentially partial write. Stale ConfigManager memory cannot negate a commit.
func (c *Coordinator) RemovalCommitted(ctx context.Context, record sourceintent.Record) (bool, error) {
	cc, exists, err := c.desired(ctx, record.Binding.Handle)
	if err != nil {
		return false, err
	}
	return exists && !cc.Enabled && sameConfig(cc, record.RetiredConfig), nil
}
