package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// LifecycleTriggerSubject is the NATS request/reply subject that runs the
// staleness lifecycle pass (entity-staleness spec, design D2/D4), served by
// processor/supersession. The pass never guesses scope: every caller states
// org+systems (and, when a filesystem check applies, a root path) explicitly
// in a LifecycleRunRequest.
const LifecycleTriggerSubject = "graph.lifecycle.run"

// Reason values for the entity.lifecycle.stale marker (source/vocabulary).
// Duplicated here as untyped string constants (rather than importing
// source/vocabulary) to avoid entangling this wire-contract package with the
// predicate registry; callers may use either.
const (
	LifecycleReasonFileDeleted   = "file_deleted"
	LifecycleReasonSourceRemoved = "source_removed"
	LifecycleReasonPathMissing   = "path_missing"
)

// lifecycleTriggerTimeout bounds one request/reply round trip to the
// lifecycle pass. Generous: callers fire this off a background goroutine,
// never a caller's synchronous request path, and a full pass may enumerate
// many entities.
const lifecycleTriggerTimeout = 30 * time.Second

// LifecycleRunRequest triggers one lifecycle pass scoped to Org+Systems
// (entity-ID org and system segments — see entityid.Build). RootPath, when
// set, anchors a filesystem liveness check: entities whose path predicate
// resolves to a now-missing file under RootPath are marked with Reason;
// entities previously marked whose file has reappeared are cleared. RootPath
// empty (the remove_source shape) skips the filesystem check entirely and
// marks every in-scope entity with Reason unconditionally — correct only
// when the source itself is gone, not merely one file.
type LifecycleRunRequest struct {
	Org      string   `json:"org"`
	Systems  []string `json:"systems"`
	RootPath string   `json:"root_path,omitempty"`
	Reason   string   `json:"reason"`
}

// LifecycleRunResponse summarizes one lifecycle pass.
type LifecycleRunResponse struct {
	Entities int `json:"entities"` // in-scope entities considered
	Paths    int `json:"paths"`    // distinct paths stat-checked (0 when RootPath is empty)
	Marked   int `json:"marked"`   // entities newly marked stale
	Cleared  int `json:"cleared"`  // entities whose marker was cleared
}

// PublishLifecycleTrigger sends req to the lifecycle pass over client and
// waits for the run summary. Callers that must not block their own event
// loop on a full graph pass (a watch-event handler, a periodic reindex tick)
// should invoke this from a background goroutine and treat a returned error
// as a soft miss — a missing or not-yet-ready lifecycle responder degrades
// staleness marking, it does not fail source ingestion.
func PublishLifecycleTrigger(ctx context.Context, client *natsclient.Client, req LifecycleRunRequest) (*LifecycleRunResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("publish lifecycle trigger: nil NATS client")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal lifecycle trigger: %w", err)
	}
	reply, err := client.RequestClassified(ctx, LifecycleTriggerSubject, data, lifecycleTriggerTimeout)
	if err != nil {
		return nil, err
	}
	var resp LifecycleRunResponse
	if err := json.Unmarshal(reply, &resp); err != nil {
		return nil, fmt.Errorf("decode lifecycle trigger response: %w", err)
	}
	return &resp, nil
}
