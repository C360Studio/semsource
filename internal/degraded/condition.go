// Package degraded provides edge-triggered degradation signals.
//
// It exists because of ADR-0011. A condition that silently breaks a
// user-visible guarantee must be visible at the default log level — but logging
// it on every occurrence floods, and flooding is exactly what drove these
// signals down to Debug, where they became invisible during the incident that
// motivated the ADR.
//
// A Condition resolves that tension: the transition INTO a degraded state logs
// once at Warn, and the return to healthy logs once at Info. A condition that
// persists across thousands of events costs two lines, not thousands, so it can
// sit at a level an operator actually reads.
package degraded

import (
	"log/slog"
	"sync/atomic"
)

// Condition is an edge-triggered degradation signal. The zero value is a
// healthy condition and is ready to use.
type Condition struct {
	active atomic.Bool
}

// Enter marks the condition degraded and logs at Warn, but only on the
// transition — repeated calls while already degraded are silent. Safe for
// concurrent use.
func (c *Condition) Enter(logger *slog.Logger, msg string, args ...any) {
	if c.active.Swap(true) {
		return // already reported
	}
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

// Clear marks the condition healthy and logs recovery at Info, but only on the
// transition. Calling it while already healthy is silent, so it is safe to call
// on every success without producing noise.
func (c *Condition) Clear(logger *slog.Logger, msg string, args ...any) {
	if !c.active.Swap(false) {
		return // was not degraded
	}
	if logger != nil {
		logger.Info(msg, args...)
	}
}

// Active reports whether the condition is currently degraded, so it can be
// surfaced on a status payload rather than inferred from logs.
func (c *Condition) Active() bool { return c.active.Load() }
