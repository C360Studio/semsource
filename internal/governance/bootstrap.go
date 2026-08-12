// Package governance declares SemSource's local projection-contract intent and
// owns the static predicate vocabulary that intent covers. Contracts describe
// operation intent and expected graph shape; they do not grant ownership, and
// no registry, heartbeat, or claim bucket exists to bind them into.
package governance

import (
	"fmt"
	"log/slog"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/pkg/projection"
)

// Bootstrap records the locally declared, validated projection intent for the
// standalone SemSource service.
type Bootstrap struct {
	Contract projection.Contract
}

// BootstrapStandalone validates and records SemSource's projection-contract
// intent before graph-ingest starts. semsource runs as a standalone external
// service and always owns this declaration; it is local by design — nothing is
// registered, published, or heartbeated.
func BootstrapStandalone(logger *slog.Logger) (*Bootstrap, error) {
	if logger == nil {
		logger = slog.Default()
	}

	contract := semsourcegraph.SourceEntityContract()
	if err := contract.Validate(); err != nil {
		return nil, fmt.Errorf("validate SemSource projection contract: %w", err)
	}

	logger.Info("projection intent declared",
		"owner", semsourcegraph.OwnerID,
		"contract", contract.Name,
		"predicates", len(semsourcegraph.OwnedPredicates()),
	)
	return &Bootstrap{Contract: contract}, nil
}
