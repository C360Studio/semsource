package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourcespawn"
	semconfig "github.com/c360studio/semstreams/config"
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
	ingestReadyWhen = "after application restart: source_status.phase in ['watching', 'idle']"

	// lifecycleTriggerTimeout bounds the background NATS round trip
	// triggering a staleness lifecycle pass after remove_source. Fire-and-
	// forget from the caller's perspective — a missing or slow responder
	// degrades staleness marking, it never blocks or fails removal.
	lifecycleTriggerTimeout = 30 * time.Second
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
		// Retain addSub so manager-owned failed-Start Stop drains active callbacks.
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
	c.desiredMu.Lock()
	defer c.desiredMu.Unlock()
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
		Components:      components,
		DesiredChanged:  len(components) > 0,
		RestartRequired: len(components) > 0,
		StatusSubject:   statusSubject,
		ReadyWhen:       ingestReadyWhen,
		Timestamp:       time.Now(),
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

	// ADRs 094/100 keep the running component set immutable. Persist the desired
	// manifest for next boot; do not change live status or promise activation.
	if len(components) > 0 {
		if persistErr := c.persistDesiredManifest(ctx, cfg, &req.Source, ""); persistErr != nil {
			if err != nil {
				persistErr = errors.Join(err, persistErr)
			}
			reply.Error = &IngestError{Code: CodeKVWriteFailed, Message: persistErr.Error()}
		}
	}

	return reply
}

// appendManifestSources adds entries reflecting src to c.manifestSources. A
// "repo" entry is recorded as itself (not its expanded children) so the
// manifest preserves the caller's intent. Idempotent on the manifest level:
// duplicate adds (same Type+identifier) are skipped.
func (c *Component) appendManifestSources(src config.SourceEntry, options ...sourcespawn.Options) {
	opts := sourcespawn.Options{}
	if len(options) > 0 {
		opts = options[0]
	}
	entry := sourceEntryToManifestSource(src)
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	kept := c.manifestSources[:0]
	for _, existing := range c.manifestSources {
		if !manifestSourcesEqual(existing, entry, opts) {
			kept = append(kept, existing)
		}
	}
	c.manifestSources = append(kept, entry)
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
	kept := c.manifestSources[:0]
	removed := false
	for _, existing := range c.manifestSources {
		built, err := sourcespawn.InstanceNames(manifestSourceToSourceEntry(existing), opts)
		_, owns := built[instanceName]
		keep := err != nil || !owns
		if !keep && store != nil {
			if cfg := store.GetConfig().Get(); cfg != nil {
				for sibling := range built {
					if sibling != instanceName {
						if siblingConfig, exists := cfg.Components[sibling]; exists && siblingConfig.Enabled {
							keep = true
							break
						}
					}
				}
			}
		}
		if keep {
			kept = append(kept, existing)
		} else {
			removed = true
		}
	}
	c.manifestSources = kept
	return removed
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

// ManifestSourceFromEntry preserves identity fields needed for desired-state handle matching.
func ManifestSourceFromEntry(src config.SourceEntry) ManifestSource {
	return sourceEntryToManifestSource(src)
}

func sourceEntryToManifestSource(src config.SourceEntry) ManifestSource {
	return ManifestSource{
		Type:           src.Type,
		Path:           src.Path,
		Paths:          src.Paths,
		URL:            src.URL,
		URLs:           src.URLs,
		Language:       src.Language,
		Languages:      src.Languages,
		Project:        src.Project,
		Version:        src.Version,
		BranchSlug:     src.BranchSlug,
		InstanceSuffix: src.InstanceSuffix,
		Bucket:         src.Bucket,
		Prefix:         src.Prefix,
		Endpoint:       src.Endpoint,
		Region:         src.Region,
		PathStyle:      src.PathStyle,
		Branch:         src.Branch,
		Watch:          src.Watch,
		PollInterval:   src.PollInterval,
		IndexInterval:  src.IndexInterval,
	}
}

func manifestSourceToSourceEntry(m ManifestSource) config.SourceEntry {
	return config.SourceEntry{
		Type:           m.Type,
		Path:           m.Path,
		Paths:          m.Paths,
		URL:            m.URL,
		URLs:           m.URLs,
		Language:       m.Language,
		Languages:      m.Languages,
		Project:        m.Project,
		Version:        m.Version,
		BranchSlug:     m.BranchSlug,
		InstanceSuffix: m.InstanceSuffix,
		Bucket:         m.Bucket,
		Prefix:         m.Prefix,
		Endpoint:       m.Endpoint,
		Region:         m.Region,
		PathStyle:      m.PathStyle,
		Branch:         m.Branch,
		Watch:          m.Watch,
		PollInterval:   m.PollInterval,
		IndexInterval:  m.IndexInterval,
	}
}

func manifestSourcesEqual(a, b ManifestSource, opts sourcespawn.Options) bool {
	if a.Type != b.Type {
		return false
	}
	left, err := sourcespawn.InstanceNames(manifestSourceToSourceEntry(a), opts)
	if err != nil {
		return false
	}
	right, err := sourcespawn.InstanceNames(manifestSourceToSourceEntry(b), opts)
	if err != nil || len(left) != len(right) {
		return false
	}
	for name := range left {
		if _, ok := right[name]; !ok {
			return false
		}
	}
	return true
}

// handleRemoveRequest persists a source disable for the next application boot.
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

// removeSource persists a disabled component config in KV and returns the
// RemoveReply. Shared by the NATS ingest handler and the HTTP façade. Removal
// changes the next boot's composition; current ingestion continues until restart.
// Entity history is retained. Durable source_removed replay remains a migration
// merge blocker (docs/testing/setup-03a/compatibility.md).
func (c *Component) removeSource(ctx context.Context, instanceName, actor string, cfg IngestHandlerConfig) *RemoveReply {
	c.desiredMu.Lock()
	defer c.desiredMu.Unlock()
	if err := sourcespawn.Remove(ctx, instanceName, cfg.Store); err != nil {
		var spawnErr *sourcespawn.Error
		// A prior disable can commit before the manifest write fails. Only a known
		// stale desired manifest entry authorizes retry repair; typos stay NOT_FOUND.
		if !errors.As(err, &spawnErr) || spawnErr.Code != sourcespawn.CodeNotFound || !desiredManifestNeedsRemovalRepair(instanceName, cfg) {
			return &RemoveReply{InstanceName: instanceName, Error: mapSpawnError(err), Timestamp: time.Now()}
		}
	}
	reply := &RemoveReply{InstanceName: instanceName, Removed: true, DesiredChanged: true, RestartRequired: true, Timestamp: time.Now()}
	if err := c.persistDesiredManifest(ctx, cfg, nil, instanceName); err != nil {
		reply.Error = &IngestError{Code: CodeKVWriteFailed, Message: err.Error()}
	}
	c.logger.Info("source removal persisted for next application boot", "namespace", cfg.Namespace, "instance_name", instanceName, "actor", actor)
	return reply
}

// persistDesiredManifest is a second KV write, not a transaction with the
// component writes. Callers expose committed components on partial failure so
// deterministic retries can repair it. Live manifest and status stay unchanged.
func (c *Component) persistDesiredManifest(ctx context.Context, cfg IngestHandlerConfig, added *config.SourceEntry, removed string) error {
	snapshot := cfg.Store.GetConfig().Get()
	envelope, ok := snapshot.Components["source-manifest"]
	if !ok {
		return errors.New("source components committed but next-boot source-manifest config is absent")
	}
	desired := DefaultConfig()
	if err := json.Unmarshal(envelope.Config, &desired); err != nil {
		return fmt.Errorf("decode desired source-manifest: %w", err)
	}
	view := &Component{manifestSources: append([]ManifestSource(nil), desired.Sources...)}
	if added != nil {
		view.appendManifestSources(*added, cfg.Spawn)
	} else {
		view.removeManifestSourceByInstance(removed, cfg.Spawn, cfg.Store)
	}
	desired.Sources = view.manifestSources
	desired.ExpectedSourceCount = enabledSourceComponentCount(snapshot.Components)

	raw, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("encode desired source-manifest: %w", err)
	}
	envelope.Config = raw
	if err := cfg.Store.PutComponentToKV(ctx, "source-manifest", envelope); err != nil {
		return fmt.Errorf("source components committed but next-boot manifest write failed: %w", err)
	}
	return nil
}

// triggerRemovalLifecycleRun announces the removed source's scope to the
// staleness lifecycle pass (processor/supersession), fired in the background
// so remove_source's caller is never blocked on a full graph pass.
func (c *Component) triggerRemovalLifecycleRun(ctx context.Context, org string, systems []string) {
	req := graph.LifecycleRunRequest{
		Org:     org,
		Systems: systems,
		Reason:  graph.LifecycleReasonSourceRemoved,
	}
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, lifecycleTriggerTimeout)
		defer cancel()
		if _, err := graph.PublishLifecycleTrigger(runCtx, c.client, req); err != nil {
			c.logger.Debug("lifecycle trigger failed (staleness marking degraded, not fatal)",
				"org", org, "systems", systems, "error", err)
		}
	}()
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

func desiredManifestNeedsRemovalRepair(instance string, cfg IngestHandlerConfig) bool {
	snapshot := cfg.Store.GetConfig().Get()
	if snapshot == nil {
		return false
	}
	envelope, ok := snapshot.Components["source-manifest"]
	if !ok {
		return false
	}
	var desired Config
	if json.Unmarshal(envelope.Config, &desired) != nil {
		return false
	}
	for _, src := range desired.Sources {
		built, err := sourcespawn.InstanceNames(manifestSourceToSourceEntry(src), cfg.Spawn)
		if err == nil {
			if _, ok := built[instance]; ok {
				view := &Component{manifestSources: append([]ManifestSource(nil), desired.Sources...)}
				return view.removeManifestSourceByInstance(instance, cfg.Spawn, cfg.Store) ||
					desired.ExpectedSourceCount != enabledSourceComponentCount(snapshot.Components)
			}
		}
	}
	return false
}

// Disabled envelopes preserve removal intent without counting as admitted sources.
func enabledSourceComponentCount(components semconfig.ComponentConfigs) int {
	count := 0
	for _, component := range components {
		if component.Enabled {
			switch component.Name {
			case "ast-source", "git-source", "doc-source", "cfgfile-source", "url-source", "image-source", "audio-source", "video-source", "objectstore-source":
				count++
			}
		}
	}
	return count
}
