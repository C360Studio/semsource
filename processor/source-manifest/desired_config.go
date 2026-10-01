package sourcemanifest

import (
	"context"

	"github.com/c360studio/semsource/internal/sourcespawn"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

// desiredConfigView is a request-local view of ConfigManager's existing facts.
// It never repairs memory or starts a worker; ambiguous writes remain errors.
type desiredConfigView struct {
	sourcespawn.ConfigStore
	snapshot *semconfig.Config
	read     func(context.Context, string) (types.ComponentConfig, bool, error)
}

func (v *desiredConfigView) GetConfig() *semconfig.SafeConfig {
	return semconfig.NewSafeConfig(v.snapshot)
}

func (v *desiredConfigView) ReadComponentConfig(ctx context.Context, name string) (types.ComponentConfig, bool, error) {
	cc, exists, err := v.read(ctx, name)
	if err == nil {
		if exists {
			v.snapshot.Components[name] = cc
		} else {
			delete(v.snapshot.Components, name)
		}
	}
	return cc, exists, err
}

func (v *desiredConfigView) PutComponentToKV(ctx context.Context, name string, cc types.ComponentConfig) error {
	if err := v.ConfigStore.PutComponentToKV(ctx, name, cc); err != nil {
		return err
	}
	v.snapshot.Components[name] = cc
	return nil
}

func desiredRequestConfig(ctx context.Context, cfg IngestHandlerConfig, handle string) (IngestHandlerConfig, error) {
	if cfg.ReadDesired == nil {
		return cfg, nil
	}
	original := cfg.Store.GetConfig().Get()
	snapshot := *original
	snapshot.Components = make(semconfig.ComponentConfigs, len(original.Components))
	for name, cc := range original.Components {
		snapshot.Components[name] = cc
	}
	view := &desiredConfigView{ConfigStore: cfg.Store, snapshot: &snapshot, read: cfg.ReadDesired}
	// Read only source entries and their next-boot manifest; unrelated component
	// configuration and graph readiness do not participate in source management.
	for name, cc := range original.Components {
		if name == "source-manifest" || isSourceComponent(cc.Name) {
			if _, _, err := view.ReadComponentConfig(ctx, name); err != nil {
				return cfg, err
			}
		}
	}
	if handle != "" {
		if _, _, err := view.ReadComponentConfig(ctx, handle); err != nil {
			return cfg, err
		}
	}
	cfg.Store = view
	return cfg, nil
}
