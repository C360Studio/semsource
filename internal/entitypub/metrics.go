package entitypub

import (
	"strings"
	"unicode"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semstreams/metric"
)

// Publish-boundary metrics (runtime-telemetry). The publisher already kept
// published/failed/dropped/retries as atomics, but they were readable only via
// accessors that one caller invoked at Stop() — so during the window that
// matters, a long seed, they were unobservable from outside the process.
//
// `retries` is exported as its OWN series rather than folded into failures.
// That separation is the whole diagnostic value: a publisher retrying every
// entity reports zero failures and zero drops while being functionally stalled,
// which is exactly the state nothing could express before.
//
//	retries rising, published rising  -> slow but healthy, self-correcting
//	retries rising, published FLAT    -> stalled
//	failed/dropped rising             -> losing data
//
// Series are per source instance (the subsystem), because an aggregate counter
// moves whenever ANY source progresses and would hide one stalled source behind
// its healthy siblings — reproducing the original failure in a new place.

// pubMetrics holds the publish-boundary counters for one source instance.
// A nil *pubMetrics is valid and does nothing, so the publisher needs no
// registry-present branch at every call site.
type pubMetrics struct {
	published prometheus.Counter
	failed    prometheus.Counter
	dropped   prometheus.Counter
	retries   prometheus.Counter
	// backpressure is 1 while the publisher is in sustained backpressure, so an
	// operator can alert on the CONDITION rather than differentiate a counter.
	backpressure prometheus.Gauge
}

// metricSubsystem converts a component instance name into a valid Prometheus
// subsystem. Instance names are kebab-case ("ast-source-workspace") but
// Prometheus only accepts [a-zA-Z_][a-zA-Z0-9_]*, so an unsanitised name would
// make registration fail silently and the series would simply never appear.
func metricSubsystem(instanceName string) string {
	var b strings.Builder
	for _, r := range instanceName {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return ""
	}
	if unicode.IsDigit(rune(out[0])) {
		return "_" + out
	}
	return out
}

// newPubMetrics registers the publish counters for a source instance. Returns
// nil when no registry is configured (tests, standalone) so callers stay
// branch-free. Registration is idempotent in the platform registry.
func newPubMetrics(registry *metric.MetricsRegistry, subsystem string) *pubMetrics {
	subsystem = metricSubsystem(subsystem)
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
	m := &pubMetrics{
		published: counter("entities_published_total",
			"Entities confirmed handed off to the graph ingest transport"),
		failed: counter("entities_failed_total",
			"Entities that exhausted publish retries and were not delivered"),
		dropped: counter("entities_dropped_total",
			"Entities dropped after bounded backpressure (buffer full)"),
		retries: counter("publish_retries_total",
			"Publish attempts retried because the transport applied backpressure"),
		backpressure: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "semsource",
			Subsystem: subsystem,
			Name:      "publish_backpressure",
			Help:      "1 while the publisher is in sustained backpressure, 0 otherwise",
		}),
	}
	// Registration errors are non-fatal: losing telemetry must never stop
	// ingest. They are idempotent by key, so a restart or a duplicate instance
	// name is not an error either.
	_ = registry.RegisterCounter(subsystem, "entities_published_total", m.published)
	_ = registry.RegisterCounter(subsystem, "entities_failed_total", m.failed)
	_ = registry.RegisterCounter(subsystem, "entities_dropped_total", m.dropped)
	_ = registry.RegisterCounter(subsystem, "publish_retries_total", m.retries)
	_ = registry.RegisterGauge(subsystem, "publish_backpressure", m.backpressure)
	return m
}

func (m *pubMetrics) incPublished() {
	if m != nil {
		m.published.Inc()
	}
}

func (m *pubMetrics) incFailed() {
	if m != nil {
		m.failed.Inc()
	}
}

func (m *pubMetrics) incDropped() {
	if m != nil {
		m.dropped.Inc()
	}
}

func (m *pubMetrics) incRetries() {
	if m != nil {
		m.retries.Inc()
	}
}

func (m *pubMetrics) setBackpressure(active bool) {
	if m == nil {
		return
	}
	if active {
		m.backpressure.Set(1)
		return
	}
	m.backpressure.Set(0)
}
