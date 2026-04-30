package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semstreams/natsclient"
)

const (
	// ingestAddSubjectPrefix is the per-namespace prefix for source-add
	// requests: graph.ingest.add.{namespace}.
	ingestAddSubjectPrefix = "graph.ingest.add"

	// ingestRemoveSubjectPrefix is the per-namespace prefix for
	// source-remove requests: graph.ingest.remove.{namespace}.
	ingestRemoveSubjectPrefix = "graph.ingest.remove"

	// ingestReadyWhen is the canonical readiness condition returned in
	// AddReply.ReadyWhen. Callers wait until the matching SourceStatus on
	// graph.ingest.status reports a phase in this set.
	ingestReadyWhen = "source_status.phase in ['watching', 'idle']"
)

// IngestHandlerConfig wires the ingest add/remove subscriptions on
// source-manifest. Callers must supply both Store (KV writes) and Spawn
// (per-source defaults). Namespace is the per-namespace subject suffix.
type IngestHandlerConfig struct {
	Namespace string
	Store     sourcespawn.ConfigStore
	Spawn     sourcespawn.Options
	// Checker is optional. When non-nil, AddReply distinguishes new vs.
	// refresh via the per-component Created flag.
	Checker sourcespawn.ExistsChecker
}

// RegisterIngestHandlers subscribes the component to graph.ingest.add and
// graph.ingest.remove for the configured namespace. Subscriptions are
// torn down by Stop along with the existing manifest/status subs.
//
// This is wired by the host program (cmd/semsource/run.go) after the
// ConfigManager is constructed and the component has Started, since the
// component itself does not own a reference to the ConfigManager.
//
// The component must be running before this is called — Stop short-circuits
// when !running, so subs registered against a non-running component would
// leak. The check below races with Stop, but since Stop also takes c.mu and
// clears c.running, the worst interleaving is a registration that fails
// cleanly mid-shutdown rather than leaking.
func (c *Component) RegisterIngestHandlers(ctx context.Context, cfg IngestHandlerConfig) error {
	if cfg.Namespace == "" {
		return errors.New("ingest handler: namespace required")
	}
	if cfg.Store == nil {
		return errors.New("ingest handler: store required")
	}

	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()
	if !running {
		return errors.New("ingest handler: component not started")
	}

	addSubject := ingestAddSubjectPrefix + "." + cfg.Namespace
	removeSubject := ingestRemoveSubjectPrefix + "." + cfg.Namespace

	// Append addSub to c.ingestSubs immediately under lock so a concurrent
	// Stop drains it correctly, even if removeSub never gets registered.
	addSub, err := c.client.SubscribeForRequests(ctx, addSubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
		return c.handleAddRequest(reqCtx, data, cfg)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", addSubject, err)
	}
	c.mu.Lock()
	c.ingestSubs = append(c.ingestSubs, addSub)
	c.mu.Unlock()

	removeSub, err := c.client.SubscribeForRequests(ctx, removeSubject, func(reqCtx context.Context, data []byte) ([]byte, error) {
		return c.handleRemoveRequest(reqCtx, data, cfg)
	})
	if err != nil {
		// Roll back addSub: remove from c.ingestSubs and unsubscribe.
		c.mu.Lock()
		c.ingestSubs = removeSub2(c.ingestSubs, addSub)
		c.mu.Unlock()
		_ = addSub.Unsubscribe()
		return fmt.Errorf("subscribe %s: %w", removeSubject, err)
	}
	c.mu.Lock()
	c.ingestSubs = append(c.ingestSubs, removeSub)
	c.mu.Unlock()

	c.logger.Info("listening for ingest requests",
		"add_subject", addSubject,
		"remove_subject", removeSubject)
	return nil
}

// removeSub2 returns subs with target removed (first match). Caller holds c.mu.
func removeSub2(subs []*natsclient.Subscription, target *natsclient.Subscription) []*natsclient.Subscription {
	for i, s := range subs {
		if s == target {
			return append(subs[:i], subs[i+1:]...)
		}
	}
	return subs
}

// handleAddRequest validates an AddRequest, dispatches to sourcespawn, and
// returns a marshaled AddReply. Errors in the reply envelope rather than
// returning Go errors so callers always get a structured response.
func (c *Component) handleAddRequest(ctx context.Context, data []byte, cfg IngestHandlerConfig) ([]byte, error) {
	var req AddRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return marshalAddReply(&AddReply{
			Error: &IngestError{
				Code:    CodeValidationFailed,
				Message: fmt.Sprintf("decode request: %v", err),
			},
			Timestamp: time.Now(),
		})
	}

	results, err := sourcespawn.AddWithChecker(ctx, req.Source, cfg.Store, cfg.Checker, cfg.Spawn)

	// AddWithChecker may return partial results alongside an error when a
	// repo-expansion mid-loop write fails. Always surface what landed so
	// the caller can distinguish "all 4 failed" from "git+ast committed,
	// docs failed" and retry safely (deterministic instance names make
	// re-add idempotent).
	components := make([]AddedComponent, 0, len(results))
	for _, r := range results {
		components = append(components, AddedComponent{
			InstanceName: r.InstanceName,
			FactoryName:  r.FactoryName,
			SourceType:   r.SourceType,
			Created:      r.Created,
		})
	}

	reply := &AddReply{
		Components:    components,
		StatusSubject: statusSubject,
		ReadyWhen:     ingestReadyWhen,
		Timestamp:     time.Now(),
	}
	if err != nil {
		reply.Error = mapSpawnError(err)
	}

	c.logger.Info("source add request handled",
		"namespace", cfg.Namespace,
		"source_type", req.Source.Type,
		"components", len(components),
		"error", err,
		"actor", req.Provenance.Actor)

	// Refresh the manifest only when at least one component landed. Partial
	// success is enough — the refreshed manifest will reflect what's actually
	// in KV, not what the caller intended.
	if len(components) > 0 {
		c.appendManifestSources(req.Source)
		if err := c.publishManifest(ctx); err != nil {
			c.logger.Warn("failed to republish manifest after add", "error", err)
		}
	}

	return marshalAddReply(reply)
}

// appendManifestSources adds entries reflecting src to c.manifestSources. A
// "repo" entry is recorded as itself (not its expanded children) so the
// manifest preserves the caller's intent. Idempotent on the manifest level:
// duplicate adds (same Type+identifier) are skipped.
func (c *Component) appendManifestSources(src config.SourceEntry) {
	entry := sourceEntryToManifestSource(src)
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	for _, existing := range c.manifestSources {
		if manifestSourcesEqual(existing, entry) {
			return
		}
	}
	c.manifestSources = append(c.manifestSources, entry)
}

// removeManifestSourceByInstance drops the first manifest entry whose
// expanded instance name matches target. Used by the Remove path so the
// manifest stays in sync with the KV. The mapping is not always perfect for
// "repo" expansions (one manifest entry → many instances); a remove by
// instance name only clears the manifest when the *last* expanded instance
// for that source is gone — left as a follow-on, since v1 manifest semantics
// are "what was registered," not "what is alive."
func (c *Component) removeManifestSourceByInstance(instanceName string, opts sourcespawn.Options) bool {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	for i, existing := range c.manifestSources {
		built, err := sourcespawn.Build(manifestSourceToSourceEntry(existing), opts)
		if err != nil {
			continue
		}
		if _, ok := built[instanceName]; ok {
			c.manifestSources = append(c.manifestSources[:i], c.manifestSources[i+1:]...)
			return true
		}
	}
	return false
}

func sourceEntryToManifestSource(src config.SourceEntry) ManifestSource {
	return ManifestSource{
		Type:          src.Type,
		Path:          src.Path,
		Paths:         src.Paths,
		URL:           src.URL,
		URLs:          src.URLs,
		Language:      src.Language,
		Branch:        src.Branch,
		Watch:         src.Watch,
		PollInterval:  src.PollInterval,
		IndexInterval: src.IndexInterval,
	}
}

func manifestSourceToSourceEntry(m ManifestSource) config.SourceEntry {
	return config.SourceEntry{
		Type:          m.Type,
		Path:          m.Path,
		Paths:         m.Paths,
		URL:           m.URL,
		URLs:          m.URLs,
		Language:      m.Language,
		Branch:        m.Branch,
		Watch:         m.Watch,
		PollInterval:  m.PollInterval,
		IndexInterval: m.IndexInterval,
	}
}

func manifestSourcesEqual(a, b ManifestSource) bool {
	if a.Type != b.Type || a.Path != b.Path || a.URL != b.URL || a.Branch != b.Branch {
		return false
	}
	if !stringSliceEqual(a.Paths, b.Paths) || !stringSliceEqual(a.URLs, b.URLs) {
		return false
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// handleRemoveRequest deletes a component config from the KV store.
func (c *Component) handleRemoveRequest(ctx context.Context, data []byte, cfg IngestHandlerConfig) ([]byte, error) {
	var req RemoveRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return marshalRemoveReply(&RemoveReply{
			Error: &IngestError{
				Code:    CodeValidationFailed,
				Message: fmt.Sprintf("decode request: %v", err),
			},
			Timestamp: time.Now(),
		})
	}

	if err := sourcespawn.Remove(ctx, req.InstanceName, cfg.Store); err != nil {
		return marshalRemoveReply(&RemoveReply{
			InstanceName: req.InstanceName,
			Error:        mapSpawnError(err),
			Timestamp:    time.Now(),
		})
	}

	c.logger.Info("source removed via ingest API",
		"namespace", cfg.Namespace,
		"instance_name", req.InstanceName,
		"actor", req.Provenance.Actor)

	if c.removeManifestSourceByInstance(req.InstanceName, cfg.Spawn) {
		if err := c.publishManifest(ctx); err != nil {
			c.logger.Warn("failed to republish manifest after remove", "error", err)
		}
	}

	return marshalRemoveReply(&RemoveReply{
		InstanceName: req.InstanceName,
		Removed:      true,
		Timestamp:    time.Now(),
	})
}

// mapSpawnError maps a sourcespawn.Error code onto the wire IngestErrorCode.
// Non-typed errors (json decode failures, anything that bypasses
// sourcespawn.Error wrapping) become INTERNAL_ERROR — retryable, distinct
// from VALIDATION_FAILED. The decode-failure path is technically a caller
// error, but we cannot distinguish it from genuine internal failures here;
// callers should see INTERNAL_ERROR and inspect Message to decide.
func mapSpawnError(err error) *IngestError {
	var serr *sourcespawn.Error
	if !errors.As(err, &serr) {
		return &IngestError{Code: CodeInternalError, Message: err.Error()}
	}
	code := CodeValidationFailed
	switch serr.Code {
	case sourcespawn.CodeValidationFailed:
		code = CodeValidationFailed
	case sourcespawn.CodeInstanceExists:
		code = CodeInstanceExists
	case sourcespawn.CodeKVWriteFailed:
		code = CodeKVWriteFailed
	case sourcespawn.CodeUnsupportedType:
		code = CodeUnsupportedType
	}
	return &IngestError{Code: code, Message: serr.Message}
}

func marshalAddReply(reply *AddReply) ([]byte, error) {
	return json.Marshal(reply)
}

func marshalRemoveReply(reply *RemoveReply) ([]byte, error) {
	return json.Marshal(reply)
}
