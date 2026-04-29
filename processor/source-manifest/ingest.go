package sourcemanifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	if err != nil {
		return marshalAddReply(&AddReply{
			Error:     mapSpawnError(err),
			Timestamp: time.Now(),
		})
	}

	components := make([]AddedComponent, 0, len(results))
	for _, r := range results {
		components = append(components, AddedComponent{
			InstanceName: r.InstanceName,
			FactoryName:  r.FactoryName,
			SourceType:   r.SourceType,
			Created:      r.Created,
		})
	}

	c.logger.Info("source added via ingest API",
		"namespace", cfg.Namespace,
		"source_type", req.Source.Type,
		"components", len(components),
		"actor", req.Provenance.Actor)

	return marshalAddReply(&AddReply{
		Components:    components,
		StatusSubject: statusSubject,
		ReadyWhen:     ingestReadyWhen,
		Timestamp:     time.Now(),
	})
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

	return marshalRemoveReply(&RemoveReply{
		InstanceName: req.InstanceName,
		Removed:      true,
		Timestamp:    time.Now(),
	})
}

// mapSpawnError maps a sourcespawn.Error code onto the wire IngestErrorCode.
// Returns a generic VALIDATION_FAILED envelope for unknown errors.
func mapSpawnError(err error) *IngestError {
	var serr *sourcespawn.Error
	if !errors.As(err, &serr) {
		return &IngestError{Code: CodeValidationFailed, Message: err.Error()}
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
