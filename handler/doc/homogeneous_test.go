package doc

import (
	"os"
	"strings"
	"testing"
)

// A homogeneous list of settings dilutes its own vector: every key in the block
// contributes to one embedding, so a query for any single key competes with the
// rest. Measured on this repository, README § Configuration is 1363 bytes and the
// fact a query asked for scored 0.6569 inside it against 0.8133 alone — losing to
// a workaround value in a different section entirely.
//
// The block is UNDER the 2000 ceiling, so no size-based path could ever reach it.
// These tests pin the trigger that does.
func TestKeyValueGroupsDetection(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantGroups int
		why        string
	}{
		{
			name: "three prefixes divide",
			content: "```bash\n" +
				"NATS_URL=nats://nats:4222\n" +
				"NATS_HOST_PORT=4222\n" +
				"LOG_LEVEL=info\n" +
				"C360_PORT=3000\n" +
				"```\n",
			wantGroups: 3,
			why:        "NATS_* groups; LOG_LEVEL and C360_PORT are their own",
		},
		{
			name: "two prefixes are left whole",
			content: "```bash\n" +
				"NATS_URL=nats://nats:4222\n" +
				"LOG_LEVEL=info\n" +
				"```\n",
			wantGroups: 0,
			why:        "below the group gate: splitting would mint passages without separating facts",
		},
		{
			name: "prose with an assignment in it is not a key/value list",
			content: "```bash\n" +
				"# start the service, then check it came up\n" +
				"export NATS_URL=nats://nats:4222\n" +
				"docker compose up -d\n" +
				"curl -sf localhost:8080/health\n" +
				"```\n",
			wantGroups: 0,
			why:        "below the assignment share: this is a script, and its lines are not independent",
		},
		{
			name: "an unfenced run is not divided",
			content: "NATS_URL=nats://nats:4222\n" +
				"LOG_LEVEL=info\n" +
				"C360_PORT=3000\n",
			wantGroups: 0,
			why:        "scoped to fenced blocks deliberately; widening needs its own evidence",
		},
		{
			name: "a repeated prefix later in the block forms its own group",
			content: "```bash\n" +
				"NATS_URL=a\n" +
				"LOG_LEVEL=b\n" +
				"C360_PORT=c\n" +
				"NATS_HOST_PORT=d\n" +
				"```\n",
			wantGroups: 4,
			why:        "only CONSECUTIVE lines group, which keeps spans contiguous and tiling trivial",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := scanLines([]byte(tt.content))
			s := section{start: 0, end: len(tt.content)}
			var got int
			for _, blk := range blocksOf(lines, s) {
				got += len(keyValueGroups(lines, blk))
			}
			if got != tt.wantGroups {
				t.Errorf("groups = %d, want %d (%s)", got, tt.wantGroups, tt.why)
			}
		})
	}
}

// The floor exists to stop a run of one-line headings minting a passage each.
// Every key group is below it, so if the merge pass reaches them the block is
// reassembled and this whole change is a silent no-op. That failure would be
// invisible — the tests above would still pass — so it is pinned end to end
// through the real entry point rather than on the split in isolation.
func TestKeyGroupsSurviveTheFloorMerge(t *testing.T) {
	content := []byte("## Configuration\n\nSet these:\n\n```bash\n" +
		"SEMSOURCE_CONFIG=mvp.json\n" +
		"SEMEMBED_CPUS=2\n" +
		"NATS_URL=nats://nats:4222\n" +
		"NATS_HOST_PORT=4222\n" +
		"NATS_MONITOR_HOST_PORT=8222\n" +
		"```\n")

	var withNATS, withConfig string
	for _, p := range splitPassages(content) {
		if strings.Contains(p.Body, "NATS_MONITOR_HOST_PORT=8222") {
			withNATS = p.Body
		}
		if strings.Contains(p.Body, "SEMSOURCE_CONFIG=") {
			withConfig = p.Body
		}
	}
	if withNATS == "" {
		t.Fatal("no passage carries the NATS default — the block was not divided")
	}
	if withNATS == withConfig {
		t.Error("the NATS group and the SEMSOURCE group share a passage: the floor merge " +
			"reassembled the block, so the split is a no-op")
	}
	if strings.Contains(withNATS, "SEMEMBED_CPUS") {
		t.Errorf("the NATS group carries unrelated settings, which is the dilution being fixed:\n%s", withNATS)
	}
}

// The load-bearing case, asserted against the real document rather than a
// fixture, so the fix cannot pass while the corpus it was measured on regresses.
func TestREADMEConfigurationSeparatesTheNATSDefault(t *testing.T) {
	content, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Skipf("README not readable: %v", err)
	}

	var found string
	for _, p := range splitPassages(content) {
		if strings.Contains(p.Body, "NATS_MONITOR_HOST_PORT=8222") {
			found = p.Body
			break
		}
	}
	if found == "" {
		t.Fatal("no passage carries NATS_MONITOR_HOST_PORT=8222")
	}
	// The query that fails today asks for the default. The passage answering it
	// must not also carry the unrelated settings that diluted it.
	if strings.Contains(found, "SEMSOURCE_CONFIG=") {
		t.Errorf("the answering passage still carries the whole env block (%d bytes):\n%s",
			len(found), found)
	}
	// The workaround value lives in § Quick Start and must stay in its own passage;
	// if it ever shares one with the default, the discrimination question can no
	// longer distinguish them on any system.
	if strings.Contains(found, "NATS_MONITOR_HOST_PORT=28222") {
		t.Error("the answering passage also carries the workaround value")
	}
}

// Tiling is the invariant every split must preserve, and dividing inside a fenced
// block is the riskiest thing done to it so far: the fence delimiters and the
// section heading have to land somewhere without being duplicated or dropped.
func TestDividedBlocksStillTileTheDocument(t *testing.T) {
	content, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Skipf("README not readable: %v", err)
	}
	var b strings.Builder
	for _, p := range splitPassages(content) {
		b.WriteString(p.Body)
	}
	if b.String() != string(content) {
		t.Fatalf("passages no longer reproduce the document: got %d bytes, want %d",
			b.Len(), len(content))
	}
}
