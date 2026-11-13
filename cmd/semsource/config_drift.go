package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sort"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/natsclient"
)

// maxDriftNames bounds the entry names one drift warning lists; the count of
// all of them is logged beside it.
const maxDriftNames = 10

// startConfigManager starts the config manager over the composition generated
// from the file and warns when boot arbitration left a retained configuration
// in effect that differs from it. An equal or older file version loses to the
// retained store silently at the manager, so this is the only place an
// operator learns their edit was not applied.
func startConfigManager(ctx context.Context, generated *semconfig.Config, nc *natsclient.Client, logger *slog.Logger) (*semconfig.Manager, error) {
	// The manager owns and mutates the config it is handed, so keep the file's
	// view before it does.
	fromFile := generated.Clone()
	configMgr, err := semconfig.NewConfigManager(generated, nc, logger)
	if err != nil {
		return nil, fmt.Errorf("create config manager: %w", err)
	}
	if err := configMgr.Start(ctx); err != nil {
		return nil, fmt.Errorf("start config manager: %w", err)
	}
	warnRetainedConfigDrift(logger, fromFile, configMgr.GetConfig().Get())
	return configMgr, nil
}

// retainedConfigDriftMessage is the warning warnRetainedConfigDrift logs.
const retainedConfigDriftMessage = "configuration file differs from the retained configuration; " +
	"the retained configuration is in effect and the file's values were not applied"

// warnRetainedConfigDrift logs one warning naming the entries that differ
// between the file's composition and the effective one, and nothing when they
// are equal.
func warnRetainedConfigDrift(logger *slog.Logger, fromFile, effective *semconfig.Config) {
	drift := retainedConfigDrift(fromFile, effective)
	if len(drift) == 0 {
		return
	}
	shown := drift
	if len(shown) > maxDriftNames {
		shown = shown[:maxDriftNames]
	}
	logger.Warn(retainedConfigDriftMessage,
		"entries", shown,
		"differing", len(drift),
		"file_version", fromFile.Version,
		"hint", "bump config_revision in the configuration file for it to apply on the next boot")
}

// retainedConfigDrift returns, sorted, the bucket keys ("services.<name>",
// "components.<name>") of every service and component whose effective
// configuration differs from the file's or exists on one side only. Entries
// compare as decoded JSON, so a re-encoding through the bucket is not a
// difference.
func retainedConfigDrift(fromFile, effective *semconfig.Config) []string {
	drift := appendDrift(nil, "services", fromFile.Services, effective.Services)
	drift = appendDrift(drift, "components", fromFile.Components, effective.Components)
	sort.Strings(drift)
	return drift
}

func appendDrift[T any](drift []string, section string, fromFile, effective map[string]T) []string {
	for name, want := range fromFile {
		if got, ok := effective[name]; !ok || !sameDecodedJSON(want, got) {
			drift = append(drift, section+"."+name)
		}
	}
	for name := range effective {
		if _, ok := fromFile[name]; !ok {
			drift = append(drift, section+"."+name)
		}
	}
	return drift
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
