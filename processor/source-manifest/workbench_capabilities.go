package sourcemanifest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	workbenchCapabilityContractVersion = 1
	workbenchReadinessTimeout          = 500 * time.Millisecond

	capabilityReady       = "ready"
	capabilityPartial     = "partial"
	capabilityNotReady    = "not_ready"
	capabilityUnsupported = "unsupported"
	readinessUnknown      = "unknown"
)

const (
	structuralStatusSubject = "graph.index.query.status"
	semanticStatusSubject   = "graph.embedding.query.status"
)

type readinessRequestFunc func(context.Context, string) ([]byte, error)

type workbenchCapabilitiesResponse struct {
	ContractVersion int                            `json:"contract_version"`
	Product         workbenchProduct               `json:"product"`
	Project         workbenchProject               `json:"project"`
	Readiness       workbenchReadiness             `json:"readiness"`
	Queries         map[string]workbenchCapability `json:"queries"`
	Actions         map[string]workbenchCapability `json:"actions"`
	ProjectViews    workbenchCapability            `json:"project_views"`
	Contracts       map[string]string              `json:"contracts"`
}

type workbenchProduct struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type workbenchProject struct {
	Key          string `json:"key"`
	IdentityKind string `json:"identity_kind"`
}

type workbenchReadiness struct {
	Overall         string                   `json:"overall"`
	Source          workbenchSourceReadiness `json:"source"`
	StructuralIndex workbenchIndexReadiness  `json:"structural_index"`
	SemanticIndex   workbenchIndexReadiness  `json:"semantic_index"`
}

type workbenchSourceReadiness struct {
	Available     bool                       `json:"available"`
	Ready         bool                       `json:"ready"`
	State         string                     `json:"state"`
	SourceCount   *int                       `json:"source_count,omitempty"`
	TotalEntities *int64                     `json:"total_entities,omitempty"`
	Timestamp     *time.Time                 `json:"timestamp,omitempty"`
	Reason        *workbenchCapabilityReason `json:"reason,omitempty"`
}

type workbenchIndexReadiness struct {
	Available       bool                       `json:"available"`
	Ready           bool                       `json:"ready"`
	State           string                     `json:"state"`
	IndexedRevision *uint64                    `json:"indexed_revision,omitempty"`
	TargetRevision  *uint64                    `json:"target_revision,omitempty"`
	Lag             *uint64                    `json:"lag,omitempty"`
	Revision        string                     `json:"revision,omitempty"`
	LastSynced      string                     `json:"last_synced,omitempty"`
	Reason          *workbenchCapabilityReason `json:"reason,omitempty"`
}

type workbenchCapability struct {
	Availability string                     `json:"availability"`
	Method       string                     `json:"method,omitempty"`
	Href         string                     `json:"href,omitempty"`
	Readiness    []string                   `json:"readiness,omitempty"`
	Reason       *workbenchCapabilityReason `json:"reason,omitempty"`
}

type workbenchCapabilityReason struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type namedReadinessSignal struct {
	name   string
	signal workbenchIndexReadiness
}

// workbenchIndexStatusWire mirrors the optional graph.IndexStatusResponse
// fields with pointers so an omitted upstream value remains absent instead of
// being manufactured as numeric zero in the browser contract.
type workbenchIndexStatusWire struct {
	Ready           bool    `json:"ready"`
	State           string  `json:"state"`
	IndexedRevision *uint64 `json:"indexed_revision,omitempty"`
	TargetRevision  *uint64 `json:"target_revision,omitempty"`
	Lag             *uint64 `json:"lag,omitempty"`
	Revision        string  `json:"revision,omitempty"`
	LastSynced      string  `json:"last_synced,omitempty"`
}

// handleWorkbenchCapabilities serves the product-owned browser bootstrap
// document. Readiness failures degrade individual capabilities rather than the
// document itself; callers can therefore render honest partial state.
func (c *Component) handleWorkbenchCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if !c.requireRunning(w) {
		return
	}

	structural, semantic := c.collectIndexReadiness(r.Context())
	if r.Context().Err() != nil {
		return
	}
	payload := c.buildWorkbenchCapabilities(structural, semantic)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		c.logger.Warn("write workbench capabilities failed", "route", r.URL.Path)
	}
}

func (c *Component) buildWorkbenchCapabilities(structural, semantic workbenchIndexReadiness) workbenchCapabilitiesResponse {
	source := c.sourceReadiness()
	overall := capabilityPartial
	if source.Ready && structural.Ready {
		overall = capabilityReady
	}

	queries := map[string]workbenchCapability{
		"source_inventory": capabilityForSignal(http.MethodGet, "/source-manifest/sources", "source",
			source.Ready, source.Reason),
		"source_status": capabilityForSignal(http.MethodGet, "/source-manifest/status", "source",
			source.Ready, source.Reason),
		"project_summary": capabilityForSignal(http.MethodGet, "/source-manifest/summary", "source",
			source.Ready, source.Reason),
		"predicate_schema": capabilityForSignal(http.MethodGet, "/source-manifest/predicates", "source",
			source.Ready, source.Reason),
		"code_context": capabilityForSignal(http.MethodPost, "/code-context/context",
			"structural_index", structural.Ready, structural.Reason),
		"code_impact": capabilityForSignal(http.MethodPost, "/code-context/impact",
			"structural_index", structural.Ready, structural.Reason),
		"code_search": capabilityForSignal(http.MethodPost, "/code-context/search",
			"semantic_index", semantic.Ready, semantic.Reason),
		"doc_context": capabilityForSignal(http.MethodPost, "/doc-context/context",
			"structural_index", structural.Ready, structural.Reason),
		"graph_projection": unsupportedCapability(
			"upstream_contract_pending", "The governed fusion graph projection is not available"),
	}

	c.mu.RLock()
	ingestReady := c.ingestCfg != nil
	c.mu.RUnlock()
	actions := map[string]workbenchCapability{
		"source_add":    ingestCapability(ingestReady, http.MethodPost, "/source-manifest/sources"),
		"source_remove": ingestCapability(ingestReady, http.MethodDelete, "/source-manifest/sources/{id}"),
		"okf_import":    unsupportedCapability("not_implemented", "OKF import is not available"),
		"okf_export":    unsupportedCapability("not_implemented", "OKF export is not available"),
	}

	return workbenchCapabilitiesResponse{
		ContractVersion: workbenchCapabilityContractVersion,
		Product:         workbenchProduct{Key: "semsource", Name: "SemSource"},
		Project: workbenchProject{
			Key:          c.config.Namespace,
			IdentityKind: "deployment_namespace",
		},
		Readiness: workbenchReadiness{
			Overall:         overall,
			Source:          source,
			StructuralIndex: structural,
			SemanticIndex:   semantic,
		},
		Queries:      queries,
		Actions:      actions,
		ProjectViews: unsupportedCapability("not_implemented", "Project views are not available"),
		Contracts:    map[string]string{"fusion_http_error": "1"},
	}
}

func (c *Component) sourceReadiness() workbenchSourceReadiness {
	status, ok := c.currentStatusPayload()
	if !ok {
		return workbenchSourceReadiness{
			State: readinessUnknown,
			Reason: &workbenchCapabilityReason{
				Code:      "status_unavailable",
				Message:   "Source readiness is unavailable",
				Retryable: true,
			},
		}
	}
	ready := status.Phase == PhaseReady
	state := status.Phase
	var reason *workbenchCapabilityReason
	if status.Phase == PhaseDegraded {
		reason = &workbenchCapabilityReason{
			Code:      "source_degraded",
			Message:   "Source ingestion is degraded",
			Retryable: true,
		}
	}
	for _, source := range status.Sources {
		switch {
		case source.Phase == SourcePhaseErrored || source.ErrorCount > 0 || source.LastError != nil:
			ready = false
			state = PhaseDegraded
			reason = &workbenchCapabilityReason{
				Code:      "source_errors_present",
				Message:   "One or more sources reported errors",
				Retryable: true,
			}
		case source.Phase == SourcePhaseIngesting && state != PhaseDegraded:
			ready = false
			state = PhaseSeeding
			if reason == nil {
				reason = &workbenchCapabilityReason{
					Code:      "source_not_ready",
					Message:   "Source ingestion is still in progress",
					Retryable: true,
				}
			}
		}
	}
	sourceCount := len(status.Sources)
	totalEntities := status.TotalEntities
	timestamp := status.Timestamp
	return workbenchSourceReadiness{
		Available:     true,
		Ready:         ready,
		State:         state,
		SourceCount:   &sourceCount,
		TotalEntities: &totalEntities,
		Timestamp:     &timestamp,
		Reason:        reason,
	}
}

func (c *Component) collectIndexReadiness(parent context.Context) (workbenchIndexReadiness, workbenchIndexReadiness) {
	ctx, cancel := context.WithTimeout(parent, c.workbenchReadinessTimeout())
	defer cancel()

	results := make(chan namedReadinessSignal, 2)
	for _, query := range []struct {
		name    string
		subject string
		label   string
	}{
		{name: "structural", subject: structuralStatusSubject, label: "Structural index"},
		{name: "semantic", subject: semanticStatusSubject, label: "Semantic index"},
	} {
		query := query
		go func() {
			results <- namedReadinessSignal{
				name:   query.name,
				signal: c.fetchIndexReadiness(ctx, query.subject, query.label),
			}
		}()
	}

	structural := unavailableReadiness("Structural index readiness is unavailable")
	semantic := unavailableReadiness("Semantic index readiness is unavailable")
	for range 2 {
		select {
		case result := <-results:
			switch result.name {
			case "structural":
				structural = result.signal
			case "semantic":
				semantic = result.signal
			}
		case <-ctx.Done():
			return structural, semantic
		}
	}
	return structural, semantic
}

func (c *Component) fetchIndexReadiness(ctx context.Context, subject, label string) workbenchIndexReadiness {
	raw, err := c.requestReadiness(ctx, subject)
	if err != nil {
		return unavailableReadiness(label + " readiness is unavailable")
	}
	var status workbenchIndexStatusWire
	if err := json.Unmarshal(raw, &status); err != nil {
		return unavailableReadiness(label + " readiness is unavailable")
	}
	state := status.State
	if state == "" {
		state = readinessUnknown
	}
	return workbenchIndexReadiness{
		Available:       true,
		Ready:           status.Ready,
		State:           state,
		IndexedRevision: status.IndexedRevision,
		TargetRevision:  status.TargetRevision,
		Lag:             status.Lag,
		Revision:        status.Revision,
		LastSynced:      status.LastSynced,
	}
}

func (c *Component) requestReadiness(ctx context.Context, subject string) ([]byte, error) {
	if c.readinessRequest != nil {
		raw, err := c.readinessRequest(ctx, subject)
		if err != nil {
			return nil, fmt.Errorf("request readiness %s: %w", subject, err)
		}
		return raw, nil
	}
	if c.client == nil {
		return nil, fmt.Errorf("request readiness %s: NATS client is unavailable", subject)
	}
	timeout := c.workbenchReadinessTimeout()
	raw, err := c.client.RequestReady(ctx, subject, nil, timeout, timeout)
	if err != nil {
		return nil, fmt.Errorf("request readiness %s: %w", subject, err)
	}
	return raw, nil
}

func (c *Component) workbenchReadinessTimeout() time.Duration {
	if c.readinessTimeout > 0 {
		return c.readinessTimeout
	}
	return workbenchReadinessTimeout
}

func capabilityForSignal(method, href, readiness string, ready bool,
	reason *workbenchCapabilityReason,
) workbenchCapability {
	capability := workbenchCapability{
		Availability: capabilityReady,
		Method:       method,
		Href:         href,
		Readiness:    []string{readiness},
	}
	if ready {
		return capability
	}
	capability.Availability = capabilityNotReady
	if reason != nil {
		capability.Reason = reason
		return capability
	}
	capability.Reason = &workbenchCapabilityReason{
		Code:      "dependency_not_ready",
		Message:   "The required readiness dependency is not ready",
		Retryable: true,
	}
	return capability
}

func ingestCapability(ready bool, method, href string) workbenchCapability {
	if ready {
		return workbenchCapability{Availability: capabilityReady, Method: method, Href: href}
	}
	return workbenchCapability{
		Availability: capabilityNotReady,
		Method:       method,
		Href:         href,
		Reason: &workbenchCapabilityReason{
			Code:      "ingest_not_ready",
			Message:   "Source management is not ready",
			Retryable: true,
		},
	}
}

func unsupportedCapability(code, message string) workbenchCapability {
	return workbenchCapability{
		Availability: capabilityUnsupported,
		Reason: &workbenchCapabilityReason{
			Code:      code,
			Message:   message,
			Retryable: false,
		},
	}
}

func unavailableReadiness(message string) workbenchIndexReadiness {
	return workbenchIndexReadiness{
		State: readinessUnknown,
		Reason: &workbenchCapabilityReason{
			Code:      "status_unavailable",
			Message:   message,
			Retryable: true,
		},
	}
}
