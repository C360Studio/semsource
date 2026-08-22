// Package seedloss attributes entity-publisher loss to a single ingest pass.
//
// The publisher's loss counters are monotonic: dropped and failed only ever
// increment, and nothing resets them. So "did this pass lose anything" cannot
// be read from the cumulative figure — a test like Lost() > 0 latches on the
// first loss and never clears, which would make a loss-degraded readiness
// phase permanent by construction rather than by policy.
//
// Tracker records the cumulative count when a pass begins and freezes the
// difference when it completes, leaving the cumulative totals untouched for
// the metrics and status surfaces that report them.
package seedloss

import "sync/atomic"

// Tracker is safe for concurrent use. Its zero value is ready: no pass has
// begun and no pass has completed, so PassLost reports zero.
type Tracker struct {
	baseline atomic.Int64
	passLost atomic.Int64
}

// Begin records the publisher's cumulative loss at the start of a pass.
func (t *Tracker) Begin(cumulativeLost int64) {
	t.baseline.Store(cumulativeLost)
}

// End freezes the loss attributable to the pass that just completed. The
// previous pass's figure stands until this is called, so a re-seed in flight
// does not erase the result a consumer is currently gating on.
func (t *Tracker) End(cumulativeLost int64) {
	delta := cumulativeLost - t.baseline.Load()
	if delta < 0 {
		// Defensive: a counter that appears to move backwards means the
		// publisher was replaced under us. Report no loss rather than a
		// negative figure that would read as a repair.
		delta = 0
	}
	t.passLost.Store(delta)
}

// PassLost returns the entities lost during the most recently completed pass.
// Zero before any pass completes, which is harmless: readiness only consults
// this once every source has finished seeding.
func (t *Tracker) PassLost() int64 {
	return t.passLost.Load()
}
