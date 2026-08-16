package entitypub

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semstreams/metric"
)

// Seed-progress metrics (ingest-observability, async-source-seed 5.7 residual).
// The publish-boundary counters above prove delivery is alive, but a seed does
// real work BEFORE anything publishes — parsing a watch path, resolving type
// references, offloading verbatim bodies. On a large corpus those windows run
// for minutes with the publish count frozen, which is indistinguishable from a
// hang. These counters advance during that pre-publish work.
//
//	publish flat, files_parsed rising    -> parsing, healthy
//	publish flat, bodies_offloaded rising -> offloading bodies, healthy
//	publish flat, both flat              -> genuinely wedged
//
// Series are per source instance for the same reason as the publish counters:
// an aggregate would hide one stalled source behind its siblings.

// SeedMetrics holds the pre-publish seed-progress counters for one source
// instance. A nil *SeedMetrics is valid and does nothing, so callers need no
// registry-present branch.
type SeedMetrics struct {
	filesParsed     prometheus.Counter
	bodiesOffloaded prometheus.Counter
}

// NewSeedMetrics registers the seed-progress counters for a source instance.
// Returns nil when no registry is configured (tests, standalone) so callers
// stay branch-free. Registration is idempotent in the platform registry.
func NewSeedMetrics(registry *metric.MetricsRegistry, instanceName string) *SeedMetrics {
	subsystem := metricSubsystem(instanceName)
	if registry == nil || subsystem == "" {
		return nil
	}
	counter := func(name, help string) prometheus.Counter {
		return prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semsource",
			Subsystem: subsystem,
			Name:      name,
			Help:      help,
		})
	}
	m := &SeedMetrics{
		filesParsed: counter("files_parsed_total",
			"Source files successfully parsed (advances during the pre-publish parse phase)"),
		bodiesOffloaded: counter("bodies_offloaded_total",
			"Entity bodies resolved to a content-store blob, fresh or deduplicated"),
	}
	// Non-fatal and idempotent, same contract as the publish counters: losing
	// telemetry must never stop ingest.
	_ = registry.RegisterCounter(subsystem, "files_parsed_total", m.filesParsed)
	_ = registry.RegisterCounter(subsystem, "bodies_offloaded_total", m.bodiesOffloaded)
	return m
}

// IncFilesParsed records one successfully parsed source file.
func (m *SeedMetrics) IncFilesParsed() {
	if m != nil {
		m.filesParsed.Inc()
	}
}

// IncBodiesOffloaded records one body resolved to a content-store blob.
func (m *SeedMetrics) IncBodiesOffloaded() {
	if m != nil {
		m.bodiesOffloaded.Inc()
	}
}
