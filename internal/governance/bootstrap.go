// Package governance wires SemSource's standalone ownership registry and the
// static predicate-schema projection that consumers read before querying the
// graph. It owns the bootstrap that registers the SemSource owner and seeds the
// authoritative predicate vocabulary.
package governance

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/pkg/projection"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// Bootstrap owns the standalone SemSource ownership registry and static
// heartbeater created before graph-ingest starts.
type Bootstrap struct {
	Registry    *ownership.Registry
	Heartbeater *ownership.Heartbeater
}

// BootstrapStandalone creates the governed graph ownership buckets and binds
// SemSource's source projection contract. semsource runs as a standalone
// external service and always owns this bootstrap.
func BootstrapStandalone(ctx context.Context, nc *natsclient.Client, logger *slog.Logger) (*Bootstrap, error) {
	if logger == nil {
		logger = slog.Default()
	}

	reg, err := ownership.EnsureBuckets(ctx, nc, logger, semvocab.InverseResolver)
	if err != nil {
		return nil, fmt.Errorf("bootstrap ownership buckets: %w", err)
	}

	hb := reg.NewHeartbeater(ownership.HeartbeatInterval)
	if _, err := projection.BindAndHeartbeat(ctx, reg, hb, OwnerID, SourceEntityContract()); err != nil {
		return nil, fmt.Errorf("bind SemSource projection ownership: %w", err)
	}

	logger.Info("governed graph ownership bootstrapped",
		"owner", OwnerID,
		"contract", SourceEntityContract().Name,
		"predicates", len(OwnedPredicates()),
	)
	return &Bootstrap{Registry: reg, Heartbeater: hb}, nil
}
