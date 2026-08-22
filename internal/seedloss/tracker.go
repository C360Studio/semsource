// Package seedloss attributes entity-publisher loss to the current ingest pass.
//
// The publisher's loss counters are monotonic: dropped and failed only ever
// increment, and nothing resets them. So "did this pass lose anything" cannot
// be read from the cumulative figure — a test like Lost() > 0 latches on the
// first loss and never clears, which would make a loss-degraded readiness
// phase permanent by construction rather than by policy.
//
// Tracker records the cumulative count when a pass begins and reports the
// difference on demand, leaving the lifetime totals untouched for the metrics
// and status surfaces that report them.
//
// The figure is computed live rather than frozen at the end of the walk.
// Publishing is asynchronous and terminal failures resolve only after their
// retry budget, well after a source has finished walking files and moved to
// watching — so a figure frozen at that transition would miss exactly the
// drain-time loss this exists to catch.
package seedloss

import "sync/atomic"

// Tracker is safe for concurrent use. Its zero value is ready: the baseline is
// zero, so LostSince reports all loss until a pass explicitly begins.
type Tracker struct {
	baseline atomic.Int64
}

// Begin records the publisher's cumulative loss at the start of a pass. A
// re-seed calls this again, which is what lets a clean pass clear a degraded
// readiness phase even though lifetime loss can never fall back.
func (t *Tracker) Begin(cumulativeLost int64) {
	t.baseline.Store(cumulativeLost)
}

// LostSince returns the entities lost since the current pass began, given the
// publisher's cumulative loss now.
func (t *Tracker) LostSince(cumulativeLost int64) int64 {
	delta := cumulativeLost - t.baseline.Load()
	if delta < 0 {
		// Defensive: a counter that appears to move backwards means the
		// publisher was replaced under us. Report no loss rather than a
		// negative figure that would read as a repair.
		return 0
	}
	return delta
}
