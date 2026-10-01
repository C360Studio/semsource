package main

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

type lifecycleRegistry struct {
	registry       *component.Registry
	configurations map[string]types.ComponentConfig
	bindings       map[string]sourceintent.Binding
	observer       sourceintent.PublicationObserver
}

// newLifecycleRegistry preflights identity from the immutable effective config
// map before the framework can call factories with only their raw config.
func newLifecycleRegistry(registry *component.Registry, configurations semconfig.ComponentConfigs, bindings map[string]sourceintent.Binding, observer sourceintent.PublicationObserver) (*lifecycleRegistry, error) {
	r := &lifecycleRegistry{registry: registry, configurations: make(map[string]types.ComponentConfig), bindings: make(map[string]sourceintent.Binding), observer: observer}
	for handle, cfg := range configurations {
		if !cfg.Enabled || !isPublicationSource(cfg.Name) {
			continue
		}
		var identity struct {
			InstanceName string `json:"instance_name"`
		}
		if err := json.Unmarshal(cfg.Config, &identity); err != nil {
			return nil, fmt.Errorf("source %s config: %w", handle, err)
		}
		if identity.InstanceName == "" || identity.InstanceName != handle {
			return nil, fmt.Errorf("source %s instance_name does not match admitted handle", handle)
		}
		cfg.Config = append(json.RawMessage(nil), cfg.Config...)
		r.configurations[handle] = cfg
	}
	for handle, binding := range bindings {
		cfg, ok := r.configurations[handle]
		if !ok {
			return nil, fmt.Errorf("source receipt binding %s has no enabled producer", handle)
		}
		digest, err := sourcelifecycle.ConfigDigest(cfg)
		if err != nil {
			return nil, err
		}
		if binding.Handle != handle || binding.Factory != cfg.Name || binding.ConfigDigest != digest || binding.Generation == 0 || binding.BootEpoch == "" || binding.Namespace == "" {
			return nil, fmt.Errorf("source %s receipt binding mismatches admitted envelope", handle)
		}
		if err := binding.Authority.Validate(); err != nil {
			return nil, err
		}
		if observer == nil {
			return nil, fmt.Errorf("source %s receipt observer unavailable", handle)
		}
		r.bindings[handle] = binding
	}
	return r, nil
}

func isPublicationSource(factory string) bool {
	switch factory {
	case "ast-source", "doc-source", "cfgfile-source", "git-source", "url-source", "objectstore-source", "image-source", "audio-source", "video-source":
		return true
	default:
		return false
	}
}

// RegisterWithConfig preserves the registration's declarer, schema, dependencies
// and metadata, replacing only construction with a verified pre-Start binding.
func (r *lifecycleRegistry) RegisterWithConfig(registration component.RegistrationConfig) error {
	original := registration.Factory
	if original == nil {
		return fmt.Errorf("source factory is nil")
	}
	registration.Factory = func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
		var identity struct {
			InstanceName string `json:"instance_name"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return nil, fmt.Errorf("source factory config: %w", err)
		}
		cfg, ok := r.configurations[identity.InstanceName]
		if !ok || cfg.Name != registration.Name {
			return nil, fmt.Errorf("source %s factory/config was not admitted", identity.InstanceName)
		}
		candidate := cfg
		candidate.Config = raw
		actual, err := sourcelifecycle.ConfigDigest(candidate)
		if err != nil {
			return nil, err
		}
		expected, err := sourcelifecycle.ConfigDigest(cfg)
		if err != nil {
			return nil, err
		}
		if actual != expected {
			return nil, fmt.Errorf("source %s config changed after admission", identity.InstanceName)
		}
		source, err := original(raw, deps)
		if err != nil {
			return nil, err
		}
		if binding, bound := r.bindings[identity.InstanceName]; bound {
			if deps.Platform.Org != binding.Authority.Org || deps.Platform.Platform != binding.Authority.Platform {
				return nil, fmt.Errorf("source %s publication authority mismatches runtime", identity.InstanceName)
			}
			publisher, ok := source.(sourceintent.BindablePublisher)
			if !ok {
				return nil, fmt.Errorf("source %s cannot establish publication proof", identity.InstanceName)
			}
			if err := publisher.BindSourceLifecycle(binding, r.observer); err != nil {
				return nil, fmt.Errorf("bind source %s publication proof: %w", identity.InstanceName, err)
			}
		}
		return source, nil
	}
	return r.registry.RegisterWithConfig(registration)
}
