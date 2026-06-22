package handler

import (
	"fmt"
	"strings"

	"github.com/c360studio/semsource/entityid"
)

// ValidateSelfSubjectState checks that a typed entity state only mutates its
// own subject. Relationship triples are still represented by Object IDs.
func ValidateSelfSubjectState(state *EntityState) error {
	if state == nil {
		return fmt.Errorf("entity state is nil")
	}
	if state.ID == "" {
		return fmt.Errorf("entity state ID is required")
	}
	for i, triple := range state.Triples {
		if triple.Subject == "" {
			return fmt.Errorf("triple %d has empty subject for entity %q", i, state.ID)
		}
		if triple.Subject != state.ID {
			return fmt.Errorf("triple %d subject %q does not match entity %q", i, triple.Subject, state.ID)
		}
	}
	return nil
}

// ValidateSelfSubjectStates applies ValidateSelfSubjectState to a batch of
// entity states and preserves the failing index in the error.
func ValidateSelfSubjectStates(states []*EntityState) error {
	for i, state := range states {
		if err := ValidateSelfSubjectState(state); err != nil {
			return fmt.Errorf("state %d: %w", i, err)
		}
	}
	return nil
}

// ValidateEntityStateID checks that a state ID is a canonical six-part
// SemSource graph ID and a legal NATS KV key.
func ValidateEntityStateID(state *EntityState) error {
	if state == nil {
		return fmt.Errorf("entity state is nil")
	}
	if err := entityid.ValidateNATSKVKey(state.ID); err != nil {
		return err
	}
	parts := strings.Split(state.ID, ".")
	if len(parts) != 6 {
		return fmt.Errorf("entity state ID %q has %d parts, want 6", state.ID, len(parts))
	}
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("entity state ID %q has empty part %d", state.ID, i)
		}
	}
	return nil
}

// ValidateEntityStateIDs applies ValidateEntityStateID to a batch of entity
// states and preserves the failing index in the error.
func ValidateEntityStateIDs(states []*EntityState) error {
	for i, state := range states {
		if err := ValidateEntityStateID(state); err != nil {
			return fmt.Errorf("state %d: %w", i, err)
		}
	}
	return nil
}
