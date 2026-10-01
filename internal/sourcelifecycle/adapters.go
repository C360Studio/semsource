package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semsource/internal/graphstatus"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"
)

// GraphInputs derives the pin's durable consumer identity from the admitted port
// declarations. Only explicit local entity inputs are admitted by this adapter.
func GraphInputs(components semconfig.ComponentConfigs) ([]sourceintent.StreamInput, error) {
	cc, ok := components["graph-ingest"]
	if !ok || !cc.Enabled {
		return nil, errors.New("graph-ingest is not admitted")
	}
	var cfg struct {
		Ports component.PortConfig `json:"ports"`
	}
	if err := json.Unmarshal(cc.Config, &cfg); err != nil {
		return nil, err
	}
	var out []sourceintent.StreamInput
	for _, port := range cfg.Ports.Inputs {
		stream, ok := port.Config.(component.JetStreamPort)
		if !ok {
			continue
		}
		if stream.StreamName == "" || len(stream.Subjects) == 0 {
			return nil, errors.New("graph entity input lacks stream/subjects")
		}
		for _, subject := range stream.Subjects {
			if strings.ContainsAny(subject, "*>") {
				return nil, errors.New("lifecycle tail requires explicit entity input subjects")
			}
			consumer := "graph-ingest-" + strings.ReplaceAll(subject, ".", "-")
			out = append(out, sourceintent.StreamInput{Stream: stream.StreamName, Consumer: consumer, Filters: []string{subject}})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no declared graph entity input")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stream+out[i].Consumer < out[j].Stream+out[j].Consumer })
	return out, nil
}

// NATSTail observes the graph owner's current transport state without acquiring
// consumer lifecycle authority. This pin proves current-retained settlement only.
type NATSTail struct{ Client *natsclient.Client }

// Settled checks current identities, exact acknowledged-delivery policy and backlog.
func (t NATSTail) Settled(ctx context.Context, scope sourceintent.SourceScope) (sourceintent.TailEvidence, error) {
	result := sourceintent.TailEvidence{Proof: sourceintent.TailProofCurrentRetained, ObservedAt: time.Now()}
	var pending bool
	for _, input := range scope.Inputs {
		stream, err := t.Client.GetStream(ctx, input.Stream)
		if err != nil {
			return result, blocker(sourceintent.CodeTailUnavailable, err.Error())
		}
		si, err := stream.Info(ctx)
		if err != nil {
			return result, err
		}
		consumer, err := stream.Consumer(ctx, input.Consumer)
		if err != nil {
			return result, blocker(sourceintent.CodeTailUnavailable, err.Error())
		}
		ci, err := consumer.Info(ctx)
		if err != nil {
			return result, err
		}
		filters := append([]string(nil), ci.Config.FilterSubjects...)
		if ci.Config.FilterSubject != "" {
			filters = append(filters, ci.Config.FilterSubject)
		}
		sort.Strings(filters)
		expected := append([]string(nil), input.Filters...)
		sort.Strings(expected)
		if ci.Config.AckPolicy != jetstream.AckExplicitPolicy || ci.Config.DeliverPolicy != jetstream.DeliverAllPolicy || !reflect.DeepEqual(filters, expected) {
			return result, blocker(sourceintent.CodeTailPolicy, "consumer requires exact filters, AckExplicit and DeliverAll")
		}
		if ci.NumAckPending < 0 || ci.Created.IsZero() || si.Created.IsZero() {
			return result, blocker(sourceintent.CodeTailUnavailable, "consumer observation lacks valid identity/counters")
		}
		result.Inputs = append(result.Inputs, sourceintent.InputEvidence{Input: input, StreamCreated: si.Created, ConsumerCreated: ci.Created, Storage: si.Config.Storage.String(), Replicas: si.Config.Replicas, AckPolicy: ci.Config.AckPolicy.String(), DeliverPolicy: ci.Config.DeliverPolicy.String(), Pending: ci.NumPending, AckPending: ci.NumAckPending})
		pending = pending || ci.NumPending > 0 || ci.NumAckPending > 0
	}
	if len(result.Inputs) == 0 {
		return result, blocker(sourceintent.CodeTailUnavailable, "no graph consumer observation")
	}
	raw, err := graphstatus.New(t.Client).Raw(ctx, graphstatus.KeyGraphIngest)
	if err != nil {
		return result, blocker(sourceintent.CodeTailUnavailable, err.Error())
	}
	var status semgraph.IndexStatusResponse
	if err := json.Unmarshal(raw, &status); err != nil {
		return result, err
	}
	if status.State == semgraph.IndexStateDegraded || status.State == "reset_required" || status.FailedCount > 0 {
		result.Degraded = true
		return result, blocker(sourceintent.CodeTailDegraded, "graph-ingest reports unresolved effects")
	}
	if pending {
		return result, blocker(sourceintent.CodeTailBacklog, "graph-ingest still has pending or unacknowledged input")
	}
	return result, nil
}

// ConfigReader uses the catalog reader over the declared config bucket stem. It
// distinguishes durable desired state from ConfigManager's mutable memory view.
func ConfigReader(ctx context.Context, client *natsclient.Client, org, stem string) (func(context.Context, string) (types.ComponentConfig, bool, error), error) {
	bucket, err := semconfig.BucketName(org, stem)
	if err != nil {
		return nil, err
	}
	reader, err := semgraph.OpenCatalogReader(ctx, client, bucket)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, handle string) (types.ComponentConfig, bool, error) {
		var cc types.ComponentConfig
		entry, err := reader.Get(ctx, "components."+strings.ReplaceAll(handle, " ", "_"))
		if err != nil {
			if natsclient.IsKVNotFoundError(err) {
				return cc, false, nil
			}
			return cc, false, err
		}
		if err := json.Unmarshal(entry.Value(), &cc); err != nil {
			return cc, false, err
		}
		return cc, true, nil
	}, nil
}

// ProjectionSubject is the product's exact, typed lifecycle projection request.
const ProjectionSubject = "graph.lifecycle.source"

// ProjectionRequest separates journal-authorized effects from legacy path sweeps.
type ProjectionRequest struct {
	Removal      *sourceintent.RemovalProjection      `json:"removal,omitempty"`
	Reactivation *sourceintent.ReactivationProjection `json:"reactivation,omitempty"`
}

// ProjectionReply retains partial counts even when one page or mutation fails.
type ProjectionReply struct {
	Result sourceintent.ProjectionResult `json:"result"`
	Error  *sourceintent.Blocker         `json:"error,omitempty"`
}

// NATSProjector routes every product lifecycle write through the supersession owner.
type NATSProjector struct{ Client *natsclient.Client }

func (p NATSProjector) request(ctx context.Context, request ProjectionRequest) (sourceintent.ProjectionResult, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	reply, err := p.Client.RequestClassified(ctx, ProjectionSubject, raw, 15*time.Second)
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	var result ProjectionReply
	if err := json.Unmarshal(reply, &result); err != nil {
		return result.Result, err
	}
	if result.Error != nil {
		return result.Result, result.Error
	}
	return result.Result, nil
}

// ApplyRemoval marks only the request's exact retained source selectors.
func (p NATSProjector) ApplyRemoval(ctx context.Context, r sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	return p.request(ctx, ProjectionRequest{Removal: &r})
}

// ApplyReactivation carries the exact current seed proof to the mutation owner.
func (p NATSProjector) ApplyReactivation(ctx context.Context, r sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	return p.request(ctx, ProjectionRequest{Reactivation: &r})
}

// ProjectionError preserves typed failures at the transport boundary.
func ProjectionError(err error) *sourceintent.Blocker {
	if err == nil {
		return nil
	}
	return asBlocker(fmt.Errorf("source lifecycle projection: %w", err))
}
