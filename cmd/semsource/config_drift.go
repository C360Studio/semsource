package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
)

// maxDriftNames bounds the names one drift warning lists per attribute; the
// count of all of them is logged beside each list.
const maxDriftNames = 10

// startConfigManager starts the config manager over the composition generated
// from the file, removes retained components the effective configuration
// lacks, and warns when boot arbitration left a retained configuration in
// effect that differs from the file. An equal or older file version loses to
// the retained store silently at the manager, so this is the only place an
// operator learns their edit was not applied.
func startConfigManager(ctx context.Context, generated *semconfig.Config, nc *natsclient.Client, logger *slog.Logger) (*semconfig.Manager, error) {
	// Defensive: at this pin the manager copies the config it is handed and
	// swaps the copy rather than writing to it, so the file's view would
	// survive Start without the clone too.
	fromFile := generated.Clone()
	configMgr, err := semconfig.NewConfigManager(generated, nc, logger)
	if err != nil {
		return nil, fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return nil, fmt.Errorf("start config manager: %w", err)
	}
	// A component the prune leaves behind stays in the bucket: this boot runs
	// the newer file without it, the next equal-version boot runs it again and
	// names it as retained-only, and the next bump clears it, so a failure here
	// does not stop the boot.
	if err := pruneComponents(ctx, configMgr, nc, fromFile.Platform, logger); err != nil {
		logger.Warn(pruneFailedMessage, "error", err)
	}
	warnRetainedConfigDrift(logger, fromFile, configMgr.GetConfig().Get())
	return configMgr, nil
}

// pruneRetainedComponents deletes every components.<name> key in the config
// bucket that the effective configuration lacks. When the bucket won
// arbitration, the manager loaded every such key, so none is absent. When the
// file was pushed, the absent ones are components an earlier file or a runtime
// add left behind, which the next equal-version boot would load back.
//
// shortcut: SemEngine's push overlays the bucket instead of replacing its key
// set, and only components have a public delete, so a model registry or
// service the file dropped stays retained. The prune runs after the push, not
// with it: a boot stopped between the two leaves the dropped component in the
// bucket until the next bump. It assumes one writer per config bucket; a
// second application booting on the same store at the same time can delete a
// component the other just added. Remove this when SemEngine's push replaces
// the key set (SemEngine issue not yet filed).
func pruneRetainedComponents(ctx context.Context, configMgr *semconfig.Manager, nc *natsclient.Client, declared semconfig.PlatformConfig, logger *slog.Logger) error {
	// The bucket is named by the declared pair, as the manager names it; the
	// effective platform id is the minted one.
	bucket, err := semconfig.BucketName(declared.Org, declared.ID)
	if err != nil {
		return fmt.Errorf("name the config bucket: %w", err)
	}
	reader, err := kvcatalog.OpenCatalogReader(ctx, nc, bucket)
	if err != nil {
		return fmt.Errorf("open config bucket %q to list components: %w", bucket, err)
	}
	keys, err := natsclient.FilteredKeys(ctx, reader, "components.*")
	if err != nil {
		return fmt.Errorf("list components in config bucket %q: %w", bucket, err)
	}
	// Start names every effective component as its key does, so the names
	// compare directly.
	effective := configMgr.GetConfig().Get().Components
	var removed []string
	for _, key := range keys {
		name := strings.TrimPrefix(key, "components.")
		if _, ok := effective[name]; ok {
			continue
		}
		if err := configMgr.DeleteComponentFromKV(ctx, name); err != nil {
			return fmt.Errorf("remove component %q, which the configuration file no longer defines, from config bucket %q: %w", name, bucket, err)
		}
		removed = append(removed, name)
	}
	if len(removed) > 0 {
		sort.Strings(removed)
		logger.Info(prunedComponentsMessage, "components", removed)
	}
	return nil
}

// pruneComponents is pruneRetainedComponents; integration tests replace it to
// fail the prune.
var pruneComponents = pruneRetainedComponents

// prunedComponentsMessage is the record pruneRetainedComponents logs.
const prunedComponentsMessage = "removed retained components the configuration file no longer defines"

// pruneFailedMessage is the warning startConfigManager logs when the prune fails.
const pruneFailedMessage = "could not remove retained components the configuration file no longer defines; " +
	"they remain in the config bucket, return on the next boot named as retained_only, and are removed by the next config_revision bump"

// retainedConfigDriftMessage is the warning warnRetainedConfigDrift logs.
const retainedConfigDriftMessage = "configuration file differs from the retained configuration; " +
	"the retained configuration is in effect and the file's values were not applied"

const (
	// retainedHint is the remedy every warning gives. It does not promise that
	// a newer file's values persist: a model registry or service the file no
	// longer defines comes back from the bucket. The retained revision can be
	// above the file's (a restored file, an older binary), so "+1" is not
	// always enough.
	retainedHint = "the retained configuration stays in effect until config_revision in the configuration file is higher than the retained one; " +
		"see \"Changing configuration on an existing deployment\" in the README"
	// retainedOnlyHint is added when a newer file would drop components, such
	// as a source added at runtime.
	retainedOnlyHint = "; a higher config_revision drops the retained_only components, so add them to the file first to keep them"
	// retainedDisabledHint is added when a newer file would re-enable
	// components, such as a file-defined source removed at runtime.
	retainedDisabledHint = "; a higher config_revision re-enables the retained_disabled components, so remove them from the file first to keep them off"
)

// warnRetainedConfigDrift logs one warning naming the entries that differ
// between the file's composition and the effective one, and nothing when they
// are equal. It names entries, never their values.
func warnRetainedConfigDrift(logger *slog.Logger, fromFile, effective *semconfig.Config) {
	drift := retainedConfigDrift(fromFile, effective)
	if len(drift.differing) == 0 && len(drift.retainedOnly) == 0 && len(drift.retainedDisabled) == 0 {
		return
	}
	hint := retainedHint
	if hasComponent(drift.retainedOnly) {
		hint += retainedOnlyHint
	}
	if len(drift.retainedDisabled) > 0 {
		hint += retainedDisabledHint
	}
	logger.Warn(retainedConfigDriftMessage,
		"entries", capNames(drift.differing),
		"differing", len(drift.differing),
		"retained_only", capNames(drift.retainedOnly),
		"retained_only_count", len(drift.retainedOnly),
		"retained_disabled", capNames(drift.retainedDisabled),
		"retained_disabled_count", len(drift.retainedDisabled),
		"file_version", fromFile.Version,
		"hint", hint)
}

// hasComponent reports whether names holds a component; a newer file deletes
// only retained components, not services.
func hasComponent(names []string) bool {
	for _, name := range names {
		if strings.HasPrefix(name, "components.") {
			return true
		}
	}
	return false
}

func capNames(names []string) []string {
	if len(names) > maxDriftNames {
		return names[:maxDriftNames]
	}
	return names
}

// configDrift names, sorted, what differs between the file's composition and
// the effective configuration. Services and components are named by their
// bucket keys ("services.<name>", "components.<name>").
type configDrift struct {
	// differing holds the services and components the file defines whose
	// effective value differs or is absent, and "model_registry" and "nats"
	// when those values differ, except those in retainedDisabled.
	differing []string
	// retainedOnly holds the services and components only the effective
	// configuration defines.
	retainedOnly []string
	// retainedDisabled holds the components the file enables and the
	// effective configuration holds disabled, as a runtime removal leaves them.
	retainedDisabled []string
}

// retainedConfigDrift compares the sections the config bucket retains and a
// boot reads back in place of the file's: services, components, the model
// registry, and the NATS settings. Values compare as decoded JSON, so a
// re-encoding through the bucket is not a difference.
func retainedConfigDrift(fromFile, effective *semconfig.Config) configDrift {
	var drift configDrift
	appendDrift(&drift, "services", fromFile.Services, effective.Services, nil)
	disabled := map[string]bool{}
	for name, want := range fromFile.Components {
		if got, ok := effective.Components[name]; ok && want.Enabled && !got.Enabled {
			disabled[name] = true
			drift.retainedDisabled = append(drift.retainedDisabled, "components."+name)
		}
	}
	appendDrift(&drift, "components", fromFile.Components, effective.Components, disabled)
	if !sameDecodedJSON(fromFile.ModelRegistry, effective.ModelRegistry) {
		drift.differing = append(drift.differing, "model_registry")
	}
	if !sameDecodedJSON(fromFile.NATS, effective.NATS) {
		drift.differing = append(drift.differing, "nats")
	}
	sort.Strings(drift.differing)
	sort.Strings(drift.retainedOnly)
	sort.Strings(drift.retainedDisabled)
	return drift
}

// appendDrift skips the file's entries named in skip.
func appendDrift[T any](drift *configDrift, section string, fromFile, effective map[string]T, skip map[string]bool) {
	for name, want := range fromFile {
		if skip[name] {
			continue
		}
		if got, ok := effective[name]; !ok || !sameDecodedJSON(want, got) {
			drift.differing = append(drift.differing, section+"."+name)
		}
	}
	for name := range effective {
		if _, ok := fromFile[name]; !ok {
			drift.retainedOnly = append(drift.retainedOnly, section+"."+name)
		}
	}
}

// sameDecodedJSON reports whether a and b encode to the same JSON value. A
// value that cannot be encoded compares as different: it is reported rather
// than hidden.
func sameDecodedJSON(a, b any) bool {
	da, okA := decodedJSON(a)
	db, okB := decodedJSON(b)
	return okA && okB && reflect.DeepEqual(da, db)
}

func decodedJSON(v any) (any, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}
