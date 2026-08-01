// Command measure-communities reports the community-size distribution in a
// running stack's COMMUNITY_INDEX, and how each community maps onto the
// {system} segment of its members' entity IDs.
//
// It exists for the multi-repo edge-synthesis measurement: a single-repo
// deployment gives every entity the same system, and system-peer edge synthesis
// then collapses label propagation into one community. Whether a multi-repo
// deployment — where system varies — produces useful communities or merely one
// blob per repo is the question this answers, so the per-system breakdown
// matters as much as the sizes.
//
// Usage:
//
//	go run ./scripts/measure-communities -nats nats://localhost:28222 -label default
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// community is the subset of semstreams' clustering.Community this reads. It is
// decoded loosely on purpose: the tool must keep working across substrate
// versions that add fields.
type community struct {
	ID      string   `json:"id"`
	Level   int      `json:"level"`
	Members []string `json:"members"`
}

func main() {
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS URL")
	label := flag.String("label", "", "label for this run, printed with the results")
	top := flag.Int("top", 15, "how many of the largest communities to detail")
	level := flag.Int("level", 0, "hierarchy level to report (0 is the base partition)")
	flag.Parse()

	if err := run(*natsURL, *label, *top, *level); err != nil {
		fmt.Fprintf(os.Stderr, "measure-communities: %v\n", err)
		os.Exit(1)
	}
}

func run(natsURL, label string, top, level int) error {
	nc, err := nats.Connect(natsURL, nats.Timeout(10*time.Second))
	if err != nil {
		return fmt.Errorf("connect %s: %w", natsURL, err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}

	ctx, cancel := contextWithTimeout(60 * time.Second)
	defer cancel()

	// Ingested entities per system first: it states the corpus the clustering
	// result is a result *about*, and a community breakdown cannot be read
	// without it — 90% of one system in one community means something very
	// different when that system is 90% of the corpus.
	if err := reportCorpus(ctx, js); err != nil {
		return err
	}

	byLevel, err := loadCommunities(ctx, js)
	if err != nil {
		return err
	}

	levels := make([]int, 0, len(byLevel))
	for l := range byLevel {
		levels = append(levels, l)
	}
	sort.Ints(levels)
	fmt.Print("levels present: ")
	for _, l := range levels {
		fmt.Printf("L%d=%d communities  ", l, len(byLevel[l]))
	}
	fmt.Printf("\nreporting level %d (the base partition)\n\n", level)

	comms, ok := byLevel[level]
	if !ok {
		return fmt.Errorf("level %d not present in COMMUNITY_INDEX", level)
	}
	reportDistribution(comms, label, top, level)
	return nil
}

// loadCommunities reads COMMUNITY_INDEX and groups community records by
// hierarchy level.
//
// The bucket holds a hierarchy — community records keyed "<level>.<id>" at
// every level, plus an "entity.<level>.<id>" reverse index. Reading them
// together double-counts every entity once per level, which is how a 12,798
// entity graph appears to have 25,620 community members. Grouping by level is
// what keeps a reported distribution a partition of the graph.
func loadCommunities(ctx context.Context, js jetstream.JetStream) (map[int][]community, error) {
	kv, err := js.KeyValue(ctx, "COMMUNITY_INDEX")
	if err != nil {
		return nil, fmt.Errorf("open COMMUNITY_INDEX (clustering may not have run yet): %w", err)
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list COMMUNITY_INDEX keys: %w", err)
	}

	byLevel := map[int][]community{}
	for _, k := range keys {
		if strings.HasPrefix(k, "entity.") {
			continue
		}
		e, err := kv.Get(ctx, k)
		if err != nil {
			continue
		}
		var c community
		if err := json.Unmarshal(e.Value(), &c); err != nil {
			continue
		}
		if len(c.Members) > 0 {
			byLevel[c.Level] = append(byLevel[c.Level], c)
		}
	}
	if len(byLevel) == 0 {
		return nil, fmt.Errorf("no communities with members found in COMMUNITY_INDEX (%d keys)", len(keys))
	}
	return byLevel, nil
}

// reportDistribution prints one level's community-size distribution and how it
// maps onto systems.
func reportDistribution(comms []community, label string, top, level int) {
	sort.Slice(comms, func(i, j int) bool { return len(comms[i].Members) > len(comms[j].Members) })

	total := 0
	for _, c := range comms {
		total += len(c.Members)
	}

	fmt.Printf("=== community-size distribution (level %d) %s ===\n", level, label)
	fmt.Printf("communities: %d\n", len(comms))
	fmt.Printf("members total (sum over communities): %d\n", total)
	fmt.Printf("largest: %d (%.1f%% of members)\n", len(comms[0].Members),
		100*float64(len(comms[0].Members))/float64(total))
	fmt.Printf("median: %d\n\n", len(comms[len(comms)/2].Members))

	fmt.Printf("%-4s %-8s %-8s %s\n", "#", "size", "share", "systems (share of community)")
	for i, c := range comms {
		if i >= top {
			fmt.Printf("... %d more communities\n", len(comms)-top)
			break
		}
		fmt.Printf("%-4d %-8d %-7.1f%% %s\n", i+1, len(c.Members),
			100*float64(len(c.Members))/float64(total), systemBreakdown(c.Members))
	}

	fmt.Println("\n=== per-system totals across all communities ===")
	bySystem := map[string]int{}
	for _, c := range comms {
		for _, m := range c.Members {
			bySystem[systemOf(m)]++
		}
	}
	for _, s := range sortedByCount(bySystem) {
		fmt.Printf("  %-40s %d\n", s, bySystem[s])
	}

	// A community that draws from one system only is repo-shaped; one that mixes
	// is topical or cross-repo. This ratio is the multi-repo question in one
	// number, so print it rather than leaving it to be eyeballed.
	pure := 0
	for _, c := range comms {
		if len(distinctSystems(c.Members)) == 1 {
			pure++
		}
	}
	fmt.Printf("\nsingle-system communities: %d of %d (%.1f%%)\n",
		pure, len(comms), 100*float64(pure)/float64(len(comms)))
}

// reportCorpus prints how many ingested entities each system contributed. The
// system segment is the source root's base name, so with one source root per
// repo this is the per-repo entity count.
func reportCorpus(ctx context.Context, js jetstream.JetStream) error {
	kv, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		return fmt.Errorf("open ENTITY_STATES: %w", err)
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		return fmt.Errorf("list ENTITY_STATES keys: %w", err)
	}
	bySystem := map[string]int{}
	for _, k := range keys {
		bySystem[systemOf(k)]++
	}
	fmt.Println("=== ingested entities per system (ENTITY_STATES) ===")
	for _, s := range sortedByCount(bySystem) {
		fmt.Printf("  %-40s %6d (%.1f%%)\n", s, bySystem[s], 100*float64(bySystem[s])/float64(len(keys)))
	}
	fmt.Printf("  %-40s %6d\n\n", "TOTAL", len(keys))
	return nil
}

// systemOf returns the {system} segment of a six-part entity ID
// ({org}.{platform}.{domain}.{system}.{type}.{instance}), or "?" when the ID
// does not have that shape.
func systemOf(entityID string) string {
	parts := strings.Split(entityID, ".")
	if len(parts) < 4 {
		return "?"
	}
	return parts[3]
}

func distinctSystems(members []string) map[string]int {
	out := map[string]int{}
	for _, m := range members {
		out[systemOf(m)]++
	}
	return out
}

// systemBreakdown names the systems a community draws from, largest share
// first. Only shares that round above zero are named — a corpus with a long
// tail of one-entity systems would otherwise print twenty "0%" terms and bury
// the shape the line exists to show.
func systemBreakdown(members []string) string {
	counts := distinctSystems(members)
	var parts []string
	hidden := 0
	for _, s := range sortedByCount(counts) {
		share := 100 * float64(counts[s]) / float64(len(members))
		if share < 0.5 && len(parts) > 0 {
			hidden++
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%.0f%%", s, share))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("(+%d <0.5%%)", hidden))
	}
	return strings.Join(parts, " ")
}

func sortedByCount(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
