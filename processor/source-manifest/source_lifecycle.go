package sourcemanifest

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

func (c *Component) acquireDesired(ctx context.Context, cfg IngestHandlerConfig) (func(), error) {
	if cfg.Lifecycle != nil {
		return cfg.Lifecycle.Acquire(ctx)
	}
	return c.desiredMu.Acquire(ctx)
}

// RepairRemovalManifest exposes the existing manifest repair through the product
// coordinator without creating a second desired-state authority or mutex.
func RepairRemovalManifest(ctx context.Context, store sourcespawn.ConfigStore, opts sourcespawn.Options, handle string) error {
	return (&Component{}).persistDesiredManifest(ctx, IngestHandlerConfig{Store: store, Spawn: opts}, nil, handle)
}
func (c *Component) removeWithLifecycle(ctx context.Context, handle string, cfg IngestHandlerConfig) *RemoveReply {
	record, err := cfg.Lifecycle.PrepareRemoval(ctx, handle)
	reply := &RemoveReply{InstanceName: handle, Timestamp: time.Now()}
	if err != nil {
		reply.Error = mapSpawnError(err)
		return reply
	}
	reply.Generation = record.Binding.Generation
	reply.ProjectionPhase = string(record.Phase)
	err = cfg.Lifecycle.PersistRemoval(ctx, record)
	committed, readErr := cfg.Lifecycle.RemovalCommitted(ctx, record)
	err = errors.Join(err, readErr)
	if committed {
		reply.Removed = true
		reply.DesiredChanged = true
		reply.RestartRequired = true
	}
	if err != nil {
		reply.Error = &IngestError{Code: CodeKVWriteFailed, Message: err.Error()}
		return reply
	}
	reply.Removed = true
	reply.DesiredChanged = true
	reply.RestartRequired = true
	reply.ProjectionPhase = string(sourceintent.Pending)
	return reply
}

type lifecycleStatus struct {
	Binding   sourceintent.Binding        `json:"binding"`
	Operation sourceintent.Operation      `json:"operation"`
	Phase     sourceintent.Phase          `json:"phase"`
	Progress  sourceintent.Progress       `json:"progress"`
	Seed      *sourceintent.SeedSeal      `json:"seed,omitempty"`
	Effect    *sourceintent.EffectAttempt `json:"effect,omitempty"`
}

func (c *Component) handleLifecycleHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, ok := c.authorizedIngest(w, r)
	if !ok {
		return
	}
	if cfg.Lifecycle == nil {
		http.Error(w, "source lifecycle unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	records, err := cfg.Lifecycle.Status(ctx)
	if err != nil {
		http.Error(w, "source lifecycle status unavailable", http.StatusServiceUnavailable)
		return
	}
	for _, record := range records {
		if record.Binding.Handle == r.PathValue("id") {
			writeJSON(w, http.StatusOK, lifecycleStatus{Binding: record.Binding, Operation: record.Operation, Phase: record.Phase, Progress: record.Progress, Seed: record.Seed, Effect: record.Effect})
			return
		}
	}
	http.Error(w, "source lifecycle intent not found", http.StatusNotFound)
}
func (c *Component) startLifecycleReplay(ctx context.Context) {
	owner := c.ingestCfg.Lifecycle
	if owner == nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, cancel)
	c.mu.Unlock()
	c.workers.Go(func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			err := owner.ReconcileOnce(runCtx)
			statusCtx, stopStatus := context.WithTimeout(runCtx, 5*time.Second)
			records, statusErr := owner.Status(statusCtx)
			stopStatus()
			pending := 0
			for _, r := range records {
				if r.Phase != sourceintent.Complete && r.Phase != sourceintent.Superseded {
					pending++
				}
			}
			c.mu.Lock()
			previous := c.lifecycleError
			c.lifecyclePending = pending
			c.lifecycleError = ""
			if joined := errors.Join(err, statusErr); joined != nil {
				c.lifecycleError = joined.Error()
			}
			current := c.lifecycleError
			c.mu.Unlock()
			if previous == "" && current != "" {
				c.logger.Warn("source lifecycle replay pending", "error", current)
			} else if previous != "" && current == "" {
				c.logger.Info("source lifecycle replay recovered")
			}
			select {
			case <-runCtx.Done():
				return
			case <-tick.C:
			}
		}
	})
}
