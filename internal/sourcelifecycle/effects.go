package sourcelifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semsource/internal/sourceintent"
	semtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/c360studio/semstreams/types"
)

func terminalEffect(outcome sourceintent.EffectOutcome) bool {
	return outcome == sourceintent.EffectVerified || outcome == sourceintent.EffectNotCommitted
}
func unresolvedEffect(r sourceintent.Record) bool {
	return r.Effect != nil && !terminalEffect(r.Effect.Outcome)
}
func sameAttempt(a, b sourceintent.EffectAttempt) bool {
	return a.ID == b.ID && a.Binding == b.Binding && a.EntityID == b.EntityID && a.StartedAt.Equal(b.StartedAt)
}
func effectBlocked() error {
	return blocker(sourceintent.CodeEffectUnresolved, "retained mutation attempt has no proven terminal outcome (semstreams#1446)")
}

// CheckAdmission prevents source generation changes before any desired write.
// Callers hold the shared desired gate throughout this check and their writes.
func (c *Coordinator) CheckAdmission(ctx context.Context) error {
	records, err := c.journal.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		if unresolvedEffect(r) {
			return effectBlocked()
		}
	}
	return nil
}
func (c *Coordinator) checkBootEffects(ctx context.Context, boot map[string]types.ComponentConfig) error {
	records, err := c.journal.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		if !unresolvedEffect(r) {
			continue
		}
		if cc, ok := boot[r.Binding.Handle]; ok && cc.Enabled {
			return effectBlocked()
		}
		if err := CheckExclusive(r.Scope, boot); err != nil {
			return errors.Join(effectBlocked(), err)
		}
	}
	return nil
}

type removalEffects struct {
	owner   *Coordinator
	binding sourceintent.Binding
}

func (c *Coordinator) effectFence(b sourceintent.Binding) sourceintent.EffectFence {
	return &removalEffects{owner: c, binding: b}
}
func (f *removalEffects) Begin(ctx context.Context, entityID string) (sourceintent.EffectAttempt, error) {
	var zero sourceintent.EffectAttempt
	if err := f.owner.CheckAdmission(ctx); err != nil {
		return zero, err
	}
	r, rev, err := f.owner.journal.Get(ctx, f.binding.Handle)
	if err != nil {
		return zero, err
	}
	if r.Binding != f.binding || r.Operation != sourceintent.Remove || r.Phase != sourceintent.Pending {
		return zero, sourceintent.ErrConflict
	}
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return zero, err
	}
	admitted := false
	for _, selector := range r.Scope.Selectors {
		admitted = admitted || strings.HasPrefix(entityID, selector.Prefix)
	}
	if !admitted {
		return zero, blocker(sourceintent.CodeOwnership, "effect entity outside captured source scope")
	}
	committed, err := f.owner.RemovalCommitted(ctx, r)
	if err != nil {
		return zero, err
	}
	if !committed {
		return zero, blocker(sourceintent.CodeDesiredAmbiguous, "effect no longer owns disabled desired source")
	}
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		return zero, err
	}
	attempt := sourceintent.EffectAttempt{ID: hex.EncodeToString(entropy), Binding: f.binding, EntityID: entityID, StartedAt: time.Now(), Outcome: sourceintent.EffectUnknown}
	r.Effect = &attempt
	r.Phase = sourceintent.Pending
	r.Progress.Blocker = asBlocker(effectBlocked())
	r.UpdatedAt = time.Now()
	if _, err := f.owner.journal.Update(ctx, r, rev); err != nil {
		return zero, err
	}
	return attempt, nil
}
func (f *removalEffects) Resolve(ctx context.Context, attempt sourceintent.EffectAttempt, outcome sourceintent.EffectOutcome) error {
	if attempt.Binding != f.binding {
		return sourceintent.ErrConflict
	}
	if !terminalEffect(outcome) {
		return effectBlocked()
	}
	r, rev, err := f.owner.journal.Get(ctx, f.binding.Handle)
	if err != nil {
		return err
	}
	if r.Binding != f.binding || r.Effect == nil || !sameAttempt(*r.Effect, attempt) {
		return sourceintent.ErrConflict
	}
	if terminalEffect(r.Effect.Outcome) {
		if r.Effect.Outcome != outcome {
			return sourceintent.ErrConflict
		}
		return nil
	}
	// Keep the terminal evidence instead of erasing the attempt. A lost CAS reply
	// can then be distinguished from an unknown remote mutation on the next read.
	resolved := *r.Effect
	resolved.Outcome = outcome
	r.Effect = &resolved
	r.UpdatedAt = time.Now()
	_, err = f.owner.journal.Update(ctx, r, rev)
	return err
}
func (j *KVJournal) validateEffect(r sourceintent.Record) error {
	e := r.Effect
	if e == nil {
		return nil
	}
	if err := j.validateBinding(e.Binding); err != nil {
		return err
	}
	if e.ID == "" || e.EntityID == "" || e.StartedAt.IsZero() || e.Binding.Handle != r.Binding.Handle || e.Binding.Generation > r.Binding.Generation {
		return fmt.Errorf("invalid effect attempt identity")
	}
	if err := semtypes.ValidateEntityIDAuthority(e.EntityID, e.Binding.Authority.Org, e.Binding.Authority.Platform, false); err != nil {
		return err
	}
	if e.Outcome != sourceintent.EffectUnknown && !terminalEffect(e.Outcome) {
		return errors.New("invalid effect outcome")
	}
	if !terminalEffect(e.Outcome) && e.Binding != r.Binding {
		return errors.New("unresolved effect belongs to different generation")
	}
	return nil
}
