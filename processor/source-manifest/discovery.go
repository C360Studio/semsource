package sourcemanifest

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/subwatch"
	"github.com/c360studio/semsource/workspace"
)

// DefaultDiscoveryInterval is how often the runtime discovery check runs when
// no discovering source sets branch_poll_interval.
const DefaultDiscoveryInterval = 15 * time.Second

// DiscoveryWatch is one discovering source the runtime check compares against
// what boot admitted (source-discovery spec, design D5).
type DiscoveryWatch struct {
	config.DiscoveryState
	RepoPath    string
	Patterns    []string // branch sources only
	MaxBranches int      // branch sources only; 0 = workspace.DefaultMaxBranches
	// Interval is the source's branch_poll_interval; 0 = unset.
	Interval time.Duration
	// Partial is set when boot kept this parent's previously admitted
	// components without listing them (a missing checkout or a skipped
	// branch): Admitted is then not the whole admitted set, so there is
	// nothing exact to compare and nothing is reported pending.
	Partial bool
}

// discoveryInterval is how often the runtime check runs: the shortest
// interval a watch sets, else DefaultDiscoveryInterval.
func discoveryInterval(watches []DiscoveryWatch) time.Duration {
	var interval time.Duration
	for _, w := range watches {
		if w.Interval > 0 && (interval == 0 || w.Interval < interval) {
			interval = w.Interval
		}
	}
	if interval == 0 {
		return DefaultDiscoveryInterval
	}
	return interval
}

// DiscoveryStatus is the discovery block on /source-manifest/status and
// source_status: what a restart would change, per discovering source.
type DiscoveryStatus struct {
	RestartRequired bool                    `json:"restart_required"`
	Sources         []DiscoverySourceStatus `json:"sources"`
}

// DiscoverySourceStatus is one discovering source's current state and the
// changes a restart would make to its branches or submodules.
type DiscoverySourceStatus struct {
	Source      string            `json:"source"`
	Kind        string            `json:"kind"` // "branch" or "submodule"
	State       string            `json:"state"`
	Error       string            `json:"error,omitempty"`
	NotAdmitted int               `json:"not_admitted,omitempty"`
	Pending     []DiscoveryChange `json:"pending,omitempty"`
}

// DiscoveryChange is one branch or submodule a restart would add, remove, or
// (for a moved submodule pin) change.
type DiscoveryChange struct {
	Kind   string `json:"kind"`   // "branch" or "submodule"
	Name   string `json:"name"`   // branch name or submodule path
	Change string `json:"change"` // "added", "removed", or "changed"
}

// checkDiscovery compares what each watch's boot admitted with what a restart
// would find now. It only lists refs and submodule pins: it creates no
// worktree and writes no configuration. A listing failure reports
// listing_failed without restart_required (owner ruling Q2).
func checkDiscovery(ctx context.Context, watches []DiscoveryWatch) *DiscoveryStatus {
	out := &DiscoveryStatus{Sources: []DiscoverySourceStatus{}}
	for _, w := range watches {
		src := DiscoverySourceStatus{Source: w.Source, Kind: w.Kind, State: config.DiscoveryOK}
		if w.State == config.DiscoveryListingFailed {
			// Boot kept this parent's names without knowing them, so there is
			// no boot set to compare: report the boot failure, never a pending
			// list or restart_required (owner ruling Q2).
			src.State, src.Error = w.State, w.Error
			out.Sources = append(out.Sources, src)
			continue
		}
		if w.Partial {
			src.State, src.Error = w.State, "boot kept components it could not list; a restart rediscovers this source"
			out.Sources = append(out.Sources, src)
			continue
		}
		var now map[string]string
		var err error
		if _, statErr := os.Stat(w.RepoPath); errors.Is(statErr, fs.ErrNotExist) {
			src.State = config.DiscoveryAwaitingCheckout
			out.Sources = append(out.Sources, src)
			continue
		}
		if w.Kind == "submodule" {
			now, err = subwatch.Pins(ctx, w.RepoPath)
		} else {
			now, src.NotAdmitted, err = restartBranches(ctx, w)
			if src.NotAdmitted > 0 {
				src.State = config.DiscoveryCapped
			}
		}
		if err != nil {
			src.State, src.Error, src.NotAdmitted = config.DiscoveryListingFailed, err.Error(), 0
			out.Sources = append(out.Sources, src)
			continue
		}
		src.Pending = diffAdmitted(w.Kind, w.Admitted, now)
		if len(w.Skipped) > 0 {
			src.Error = w.Error // boot could not create these worktrees
		}
		out.RestartRequired = out.RestartRequired || len(src.Pending) > 0
		out.Sources = append(out.Sources, src)
	}
	return out
}

// restartBranches is the branch set the next boot would admit: the matching
// local branches up to the cap, in listing order, as boot's expansion takes
// them, and how many matches the cap leaves out.
//
// shortcut: externally created worktrees boot also admits are not counted,
// so they can show as pending removals, and a skipped branch whose cause was
// fixed is not reported; upgrade if an operator relies on either.
func restartBranches(ctx context.Context, w DiscoveryWatch) (map[string]string, int, error) {
	branches, err := workspace.ListBranches(ctx, w.RepoPath)
	if err != nil {
		return nil, 0, err
	}
	// A branch boot could not create a worktree for takes no slot, as at
	// boot, and is not pending: a restart does not admit it either.
	matched := slices.DeleteFunc(workspace.MatchBranches(branches, w.Patterns), func(b string) bool {
		return slices.Contains(w.Skipped, b)
	})
	limit := w.MaxBranches
	if limit <= 0 {
		limit = workspace.DefaultMaxBranches
	}
	notAdmitted := 0
	if len(matched) > limit {
		notAdmitted = len(matched) - limit
		matched = matched[:limit]
	}
	out := make(map[string]string, len(matched))
	for _, b := range matched {
		out[b] = ""
	}
	return out, notAdmitted, nil
}

// diffAdmitted lists, sorted by name, what differs between the boot set and
// the current one: names only now are added, names only at boot are removed,
// and names whose value (a submodule's gitlink SHA) moved are changed.
func diffAdmitted(kind string, boot, now map[string]string) []DiscoveryChange {
	var out []DiscoveryChange
	for name, v := range now {
		was, ok := boot[name]
		switch {
		case !ok:
			out = append(out, DiscoveryChange{Kind: kind, Name: name, Change: "added"})
		case was != v:
			out = append(out, DiscoveryChange{Kind: kind, Name: name, Change: "changed"})
		}
	}
	for name := range boot {
		if _, ok := now[name]; !ok {
			out = append(out, DiscoveryChange{Kind: kind, Name: name, Change: "removed"})
		}
	}
	slices.SortFunc(out, func(a, b DiscoveryChange) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}

// startDiscoveryCheck runs the read-only check now and then on interval,
// storing each result in the served status.
func (c *Component) startDiscoveryCheck(ctx context.Context, watches []DiscoveryWatch, interval time.Duration) context.CancelFunc {
	if interval <= 0 {
		interval = DefaultDiscoveryInterval
	}
	checkCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.workers.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			status := checkDiscovery(checkCtx, watches)
			c.statusMu.Lock()
			c.discovery = status
			c.statusMu.Unlock()
			c.refreshStatus()
			select {
			case <-checkCtx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	return cancel
}
