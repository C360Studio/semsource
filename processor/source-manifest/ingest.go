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
	// APIToken, when non-empty, is the bearer token the HTTP façade requires on
	// its write/read endpoints (ADR-0007 §6 auth seam). Empty = permissive
	// (trusted-network) default. Unused by the NATS path.
	APIToken string
	// AllowedRoots is the filesystem-root allowlist enforced on path-based source
	// registration over HTTP (ADR-0007 §3). Empty rejects path-based HTTP adds.
	// Unused by the NATS path (in-mesh trusted).
	AllowedRoots []string
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
	// Publish the config to the HTTP façade (registered separately by the
	// ServiceManager, so its handlers read it here at request time). Copy so the
	// stored value can't be mutated by the caller after the fact.
	cfgCopy := cfg
	c.ingestCfg = &cfgCopy
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
	return marshalAddReply(c.addSource(ctx, req, cfg))
}

// addSource dispatches an AddRequest to sourcespawn and returns the AddReply. It
// is the single source-add code path shared by the NATS ingest handler and the
// HTTP façade (ADR-0007). Transport-level concerns (auth, path allowlisting)
// are the caller's responsibility and must run BEFORE this.
func (c *Component) addSource(ctx context.Context, req AddRequest, cfg IngestHandlerConfig) *AddReply {
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
		// Re-adding a previously removed instance makes its status reports
		// welcome again (see the removedSources guard in handleStatusReport).
		c.statusMu.Lock()
		delete(c.removedSources, r.InstanceName)
		c.statusMu.Unlock()
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

	return reply
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

// removeManifestSourceByInstance keeps the manifest in sync with the KV on
// removal, instance-scoped: for "repo" expansions (one manifest entry → many
// instances) the entry is dropped only when the removed instance was the LAST
// of its expansion still registered — removing just the git instance of an
// expanded repo previously erased the whole repo from the manifest while its
// sibling components kept ingesting (audit 2026-07-19).
func (c *Component) removeManifestSourceByInstance(instanceName string, opts sourcespawn.Options, store sourcespawn.ConfigStore) bool {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	for i, existing := range c.manifestSources {
		built, err := sourcespawn.Build(manifestSourceToSourceEntry(existing), opts)
		if err != nil {
			continue
		}
		if _, ok := built[instanceName]; !ok {
			continue
		}
		// This entry owns the removed instance. Keep the entry while any
		// sibling instance of the same expansion remains registered.
		if store != nil {
			if cfg := store.GetConfig().Get(); cfg != nil {
				for sibling := range built {
					if sibling == instanceName {
						continue
					}
					if _, alive := cfg.Components[sibling]; alive {
						return false
					}
				}
			}
		}
		c.manifestSources = append(c.manifestSources[:i], c.manifestSources[i+1:]...)
		return true
	}
	return false
}

// dropSourceStatus removes a deregistered source from the status aggregator
// and republishes the rebuilt status so the change is observable within one
// aggregation pass. The instance is also remembered as removed so an
// in-flight periodic report from the tearing-down component cannot resurrect
// a phantom status entry.
func (c *Component) dropSourceStatus(ctx context.Context, instanceName string) {
	c.statusMu.Lock()
	if c.removedSources == nil {
		c.removedSources = make(map[string]struct{})
	}
	c.removedSources[instanceName] = struct{}{}
	if c.aggregator == nil || !c.aggregator.remove(instanceName) {
		c.statusMu.Unlock()
		return
	}
	status := c.aggregator.buildStatus(c.config.Namespace)
	c.statusMu.Unlock()

	c.updateStatusData(status)
	if err := c.publishPayload(ctx, StatusType, status, statusSubject); err != nil {
		c.logger.Warn("failed to publish status after source removal", "error", err)
	}
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
	return marshalRemoveReply(c.removeSource(ctx, req.InstanceName, req.Provenance.Actor, cfg))
}

// removeSource deletes a component config from the KV store and returns the
// RemoveReply. Shared by the NATS ingest handler and the HTTP façade. Removal
// stops ingestion but does NOT retract entities — eager retraction is parked
// behind the fact-layer provenance model (ADR-0007 sequencing guardrail).
func (c *Component) removeSource(ctx context.Context, instanceName, actor string, cfg IngestHandlerConfig) *RemoveReply {
	if err := sourcespawn.Remove(ctx, instanceName, cfg.Store); err != nil {
		return &RemoveReply{
			InstanceName: instanceName,
			Error:        mapSpawnError(err),
			Timestamp:    time.Now(),
		}
	}

	c.logger.Info("source removed via ingest API",
		"namespace", cfg.Namespace,
		"instance_name", instanceName,
		"actor", actor)

	// Drop the removed source from status aggregation immediately — before
	// this, removed sources reported as phantom "watching" entries forever
	// (audit 2026-07-19, live-confirmed at a 20-minute horizon).
	c.dropSourceStatus(ctx, instanceName)

	if c.removeManifestSourceByInstance(instanceName, cfg.Spawn, cfg.Store) {
		if err := c.publishManifest(ctx); err != nil {
			c.logger.Warn("failed to republish manifest after remove", "error", err)
		}
	}

	return &RemoveReply{
		InstanceName: instanceName,
		Removed:      true,
		Timestamp:    time.Now(),
	}
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
	case sourcespawn.CodeNotFound:
		code = CodeNotFound
	}
	return &IngestError{Code: code, Message: serr.Message}
}

func marshalAddReply(reply *AddReply) ([]byte, error) {
	return json.Marshal(reply)
}

func marshalRemoveReply(reply *RemoveReply) ([]byte, error) {
	return json.Marshal(reply)
}
