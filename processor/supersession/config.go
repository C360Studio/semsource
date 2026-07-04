// Package supersession implements the versioned-source correspondence and
// supersession pass (ADR-0008 open item #2). It enumerates versioned code
// entities from the graph, groups "the same logical symbol across versions of
// one source", and emits directional supersession lineage edges relating them.
//
// The pass is deterministic (tier-0, no model), additive, and retention-safe:
// it only reads via graph.query.prefix and appends lineage triples via the
// entity-publish path — it never retracts, deletes, or overwrites.
package supersession

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Default subjects and bounds.
const (
	// DefaultTriggerSubject is the NATS request/reply subject that runs the pass
	// on demand and returns a run summary.
	DefaultTriggerSubject = "graph.supersession.run"

	// DefaultMaxEntities bounds a single enumeration (QueryPrefixAll requires a
	// positive cap; there is no unbounded mode). Sized well above a large graph.
	DefaultMaxEntities = 200000
)

// Config holds configuration for the supersession component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Prefix scopes enumeration to entity IDs sharing this dot-delimited prefix
	// (e.g. "acme.semsource"). Empty enumerates the whole graph; non-code and
	// version-less entities are filtered out in code (they carry no version
	// triple), so an empty prefix is safe, just less selective.
	Prefix string `json:"prefix,omitempty" schema:"type:string,description:Entity-ID prefix to enumerate (empty = whole graph),category:basic"`

	// MaxEntities bounds a single enumeration pass. Must be > 0.
	MaxEntities int `json:"max_entities,omitempty" schema:"type:int,description:Maximum entities to enumerate per pass,category:advanced"`

	// TriggerSubject is the NATS request/reply subject that runs the pass on
	// demand. A request (any/empty body) runs one pass and replies with a JSON
	// run summary.
	TriggerSubject string `json:"trigger_subject,omitempty" schema:"type:string,description:NATS request subject to run the pass on demand,category:basic"`

	// Interval optionally runs the pass periodically (a Go duration string, e.g.
	// "5m"). Empty or "0" disables periodic runs — the pass then runs only on the
	// TriggerSubject. The pass is idempotent, so periodic re-runs are safe no-ops.
	Interval string `json:"interval,omitempty" schema:"type:string,description:Periodic run interval (Go duration; empty/0 = on-demand only),category:basic"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.MaxEntities < 0 {
		return fmt.Errorf("max_entities must be >= 0 (got %d)", c.MaxEntities)
	}
	if d := c.Interval; d != "" && d != "0" {
		if _, err := time.ParseDuration(d); err != nil {
			return fmt.Errorf("invalid interval %q: %w", d, err)
		}
	}
	return nil
}

// intervalDuration returns the parsed periodic interval, or 0 (disabled) when
// unset. Assumes Validate has passed.
func (c *Config) intervalDuration() time.Duration {
	if c.Interval == "" || c.Interval == "0" {
		return 0
	}
	d, _ := time.ParseDuration(c.Interval)
	return d
}

// maxEntities returns the effective enumeration cap (default when unset).
func (c *Config) maxEntities() int {
	if c.MaxEntities <= 0 {
		return DefaultMaxEntities
	}
	return c.MaxEntities
}

// triggerSubject returns the effective trigger subject (default when unset).
func (c *Config) triggerSubject() string {
	if c.TriggerSubject == "" {
		return DefaultTriggerSubject
	}
	return c.TriggerSubject
}

// DefaultConfig returns the default configuration for the supersession component.
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Outputs: []component.PortDefinition{
				{
					Name:        "graph.ingest",
					Type:        "jetstream",
					Subject:     "graph.ingest.entity",
					StreamName:  "GRAPH",
					Required:    true,
					Description: "Supersession lineage edges appended to existing code entities",
				},
			},
		},
		MaxEntities:    DefaultMaxEntities,
		TriggerSubject: DefaultTriggerSubject,
	}
}
