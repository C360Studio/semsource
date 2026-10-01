package main

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semstreams/component"
)

type projectionRegistry struct {
	registry  *component.Registry
	projector *sourcelifecycle.LocalProjector
}

// RegisterWithConfig preserves the normal declaration and pre-Start construction
// while binding the single local effect owner without a mutation-capable RPC.
func (r *projectionRegistry) RegisterWithConfig(registration component.RegistrationConfig) error {
	original := registration.Factory
	if registration.Name != "supersession" || original == nil || r.projector == nil {
		return fmt.Errorf("invalid local projection registration")
	}
	registration.Factory = func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
		instance, err := original(raw, deps)
		if err != nil {
			return nil, err
		}
		owner, ok := instance.(sourceintent.Projector)
		if !ok {
			return nil, fmt.Errorf("supersession factory lacks local projection contract")
		}
		if err := r.projector.Bind(owner); err != nil {
			return nil, err
		}
		return instance, nil
	}
	return r.registry.RegisterWithConfig(registration)
}
