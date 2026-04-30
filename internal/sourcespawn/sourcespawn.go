package sourcespawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/types"
)

// ErrorCode is a typed error code for source-spawn operations. Codes flow
// to remote callers (graph.ingest.add reply payloads) so they can branch on
// retryability without scraping log strings.
type ErrorCode string

const (
	// CodeValidationFailed indicates the SourceEntry did not pass type-specific
	// validation (missing required fields, invalid duration strings, etc.).
	CodeValidationFailed ErrorCode = "VALIDATION_FAILED"

	// CodeInstanceExists indicates a component with the deterministic instance
	// name already exists. The Add helper still overwrites; this code is
	// returned via Result.Created=false rather than as an error, but is
	// reserved here for explicit-conflict modes.
	CodeInstanceExists ErrorCode = "INSTANCE_EXISTS"

	// CodeKVWriteFailed indicates the underlying ConfigManager KV write
	// failed. Retryable.
	CodeKVWriteFailed ErrorCode = "KV_WRITE_FAILED"

	// CodeUnsupportedType indicates the SourceEntry.Type is not yet
	// implementable through this API (e.g., multi-branch repo). The type may
	// be a valid SourceEntry type but unspawnable in the current build.
	CodeUnsupportedType ErrorCode = "UNSUPPORTED_TYPE"
)

// Error wraps a typed error code with a message and optional cause.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// CodeOf extracts the ErrorCode from err if it is or wraps a *Error.
// Returns the empty string for unrelated errors.
func CodeOf(err error) ErrorCode {
	var serr *Error
	if errors.As(err, &serr) {
		return serr.Code
	}
	return ""
}

// Options carries deployment-wide settings the per-type config builders need.
// Mirrors the fields of config.Config that flow into source components.
type Options struct {
	// Org is the namespace ("c360", "noaa", etc.) used as the entity-ID org
	// segment and propagated into source components.
	Org string

	// WorkspaceDir is the base directory where git sources clone repos.
	WorkspaceDir string

	// GitToken is an optional auth token forwarded to git sources for
	// private-repo access.
	GitToken string

	// MediaStoreDir is the base directory where media (image/video/audio)
	// content is stored. When empty, media components fall back to their
	// component-level default.
	MediaStoreDir string
}

// Result describes one component spawn outcome from an Add call. A single
// flat source (git, ast, doc, cfgfile, url, media) yields one Result; a
// "repo" meta-source single-branch expansion yields four (git, ast, doc,
// cfgfile).
type Result struct {
	// InstanceName is the deterministic component instance name written to
	// the KV store. Callers use it to subscribe to per-instance status and
	// to call Remove later.
	InstanceName string

	// FactoryName is the registered component factory ("git-source",
	// "ast-source", etc.). Useful for telemetry and for building remove
	// calls that don't carry the original SourceEntry.
	FactoryName string

	// Created is true when the KV write replaced no prior config under the
	// same key. False when an entry under InstanceName already existed (the
	// write still succeeds — deterministic names make Add idempotent).
	Created bool

	// SourceType echoes the original src.Type for caller context. For repo
	// expansions every Result records the *expanded* type (git/ast/...),
	// not the original "repo".
	SourceType string
}

// ConfigStore is the minimal subset of *semconfig.Manager that sourcespawn
// needs. Tests can use a fake by implementing this interface.
type ConfigStore interface {
	PutComponentToKV(ctx context.Context, name string, cfg types.ComponentConfig) error
	DeleteComponentFromKV(ctx context.Context, name string) error
}

// ExistsChecker is an optional capability used to detect Result.Created.
// When nil, Created is always reported true.
type ExistsChecker interface {
	HasComponent(name string) bool
}

// Add validates src and writes the corresponding component config(s) into
// the ConfigManager KV store. ServiceManager picks up the change reactively
// and spawns the component(s).
//
// A flat source produces one Result. A "repo" meta-source single-branch
// expansion produces four Results (git, ast, doc, cfgfile). Multi-branch
// repos return CodeUnsupportedType — the BranchWatcher path is not yet
// KV-reactive.
//
// Add is idempotent: deterministic instance names mean re-submitting the
// same SourceEntry overwrites the existing KV entry. Result.Created
// distinguishes new vs. refresh when an ExistsChecker is supplied via
// AddWithChecker.
func Add(ctx context.Context, src config.SourceEntry, store ConfigStore, opts Options) ([]Result, error) {
	return AddWithChecker(ctx, src, store, nil, opts)
}

// AddWithChecker is Add but uses checker to populate Result.Created when
// non-nil.
func AddWithChecker(
	ctx context.Context,
	src config.SourceEntry,
	store ConfigStore,
	checker ExistsChecker,
	opts Options,
) ([]Result, error) {
	if err := src.Validate(); err != nil {
		return nil, &Error{Code: CodeValidationFailed, Message: err.Error()}
	}

	// "repo" is a meta-source. Multi-branch isn't supported via this API yet.
	if src.Type == "repo" && len(src.Branches) > 0 {
		return nil, &Error{
			Code:    CodeUnsupportedType,
			Message: "multi-branch repo adds are not supported via this API; submit individual git/ast/docs/config entries instead",
		}
	}

	specs, err := buildSpecs(src, opts)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(specs))
	for _, spec := range specs {
		raw, err := json.Marshal(spec.compCfg)
		if err != nil {
			return results, &Error{
				Code:    CodeValidationFailed,
				Message: fmt.Sprintf("marshal component config for %q", spec.instanceName),
				Cause:   err,
			}
		}

		created := true
		if checker != nil && checker.HasComponent(spec.instanceName) {
			created = false
		}

		cfg := types.ComponentConfig{
			Name:    spec.factoryName,
			Type:    types.ComponentTypeProcessor,
			Enabled: true,
			Config:  raw,
		}
		if err := store.PutComponentToKV(ctx, spec.instanceName, cfg); err != nil {
			return results, &Error{
				Code:    CodeKVWriteFailed,
				Message: fmt.Sprintf("put component %q to KV", spec.instanceName),
				Cause:   err,
			}
		}

		results = append(results, Result{
			InstanceName: spec.instanceName,
			FactoryName:  spec.factoryName,
			Created:      created,
			SourceType:   spec.sourceType,
		})
	}

	return results, nil
}

// Build is the marshal-only path: it validates src and produces the
// instance-name → ComponentConfig map without touching any KV store. Used
// by the startup loader (which collects all sources into a single config
// document before handing it to the ConfigManager) and the branch-watcher.
//
// Instance names are deterministic functions of the SourceEntry's
// identifying fields — no index parameter, no insertion-order dependency.
// Equivalent inputs always produce identical KV keys, which is what makes
// Add idempotent.
func Build(src config.SourceEntry, opts Options) (map[string]types.ComponentConfig, error) {
	if err := src.Validate(); err != nil {
		return nil, &Error{Code: CodeValidationFailed, Message: err.Error()}
	}

	specs, err := buildSpecs(src, opts)
	if err != nil {
		return nil, err
	}

	out := make(map[string]types.ComponentConfig, len(specs))
	for _, spec := range specs {
		raw, err := json.Marshal(spec.compCfg)
		if err != nil {
			return nil, &Error{
				Code:    CodeValidationFailed,
				Message: fmt.Sprintf("marshal component config for %q", spec.instanceName),
				Cause:   err,
			}
		}
		out[spec.instanceName] = types.ComponentConfig{
			Name:    spec.factoryName,
			Type:    types.ComponentTypeProcessor,
			Enabled: true,
			Config:  raw,
		}
	}
	return out, nil
}

// Remove deletes the component config from the ConfigManager KV store.
// ServiceManager tears down the component reactively.
func Remove(ctx context.Context, instanceName string, store ConfigStore) error {
	if instanceName == "" {
		return &Error{Code: CodeValidationFailed, Message: "instance_name is required"}
	}
	if err := store.DeleteComponentFromKV(ctx, instanceName); err != nil {
		return &Error{
			Code:    CodeKVWriteFailed,
			Message: fmt.Sprintf("delete component %q from KV", instanceName),
			Cause:   err,
		}
	}
	return nil
}

// componentSpec is the internal pre-marshal form of one component to write.
type componentSpec struct {
	instanceName string
	factoryName  string
	sourceType   string
	compCfg      map[string]any
}

// buildSpecs dispatches src to the right per-type builder(s). Instance names
// are deterministic functions of the SourceEntry's identifying fields — no
// insertion-order or index dependency.
func buildSpecs(src config.SourceEntry, opts Options) ([]componentSpec, error) {
	switch src.Type {
	case "ast":
		name, cfg, err := astComponentConfig(src, opts.Org)
		if err != nil {
			return nil, &Error{Code: CodeValidationFailed, Message: err.Error()}
		}
		return []componentSpec{{name, "ast-source", "ast", cfg}}, nil

	case "git":
		name, cfg, err := gitComponentConfig(src, opts.Org, opts)
		if err != nil {
			return nil, &Error{Code: CodeValidationFailed, Message: err.Error()}
		}
		return []componentSpec{{name, "git-source", "git", cfg}}, nil

	case "docs":
		name, cfg := docComponentConfig(src, opts.Org)
		return []componentSpec{{name, "doc-source", "docs", cfg}}, nil

	case "config":
		name, cfg := cfgfileComponentConfig(src, opts.Org)
		return []componentSpec{{name, "cfgfile-source", "config", cfg}}, nil

	case "url":
		name, cfg := urlComponentConfig(src, opts.Org)
		return []componentSpec{{name, "url-source", "url", cfg}}, nil

	case "image", "video", "audio":
		name, cfg := mediaComponentConfig(src, opts.Org, opts)
		return []componentSpec{{name, src.Type + "-source", src.Type, cfg}}, nil

	case "repo":
		return repoSpecs(src, opts)

	default:
		return nil, &Error{
			Code:    CodeUnsupportedType,
			Message: fmt.Sprintf("source type %q is not spawnable via sourcespawn", src.Type),
		}
	}
}

// repoSpecs expands a single-branch repo into git+ast+docs+config specs.
// Multi-branch (Branches set) is rejected earlier with CodeUnsupportedType.
func repoSpecs(src config.SourceEntry, opts Options) ([]componentSpec, error) {
	expanded, err := config.ExpandRepoSources(
		// ExpandRepoSources takes a context for branch discovery; with
		// Branches empty it never reads the network and ctx is unused.
		context.Background(),
		[]config.SourceEntry{src},
		opts.WorkspaceDir,
	)
	if err != nil {
		return nil, &Error{
			Code:    CodeValidationFailed,
			Message: "expand repo source",
			Cause:   err,
		}
	}

	var specs []componentSpec
	for _, entry := range expanded.Sources {
		sub, err := buildSpecs(entry, opts)
		if err != nil {
			return nil, err
		}
		specs = append(specs, sub...)
	}
	return specs, nil
}
