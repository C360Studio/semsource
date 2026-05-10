package main

import "testing"

func TestSubjectFilterMatches(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		subject string
		want    bool
	}{
		// Exact match.
		{"exact match", "graph.ingest.entity", "graph.ingest.entity", true},
		{"exact mismatch", "graph.ingest.entity", "graph.ingest.batch", false},

		// Single-token wildcard.
		{"single * matches one token", "graph.ingest.*", "graph.ingest.entity", true},
		{"single * does not cross tokens", "graph.ingest.*", "graph.ingest.add.ns", false},
		{"single * in middle", "graph.*.entity", "graph.ingest.entity", true},

		// Multi-token wildcard — the load-bearing case.
		{"trailing > captures all under graph.ingest", "graph.ingest.>", "graph.ingest.add.ns", true},
		{"trailing > captures entity too", "graph.ingest.>", "graph.ingest.entity", true},
		{"root > captures everything", ">", "graph.ingest.add.ns", true},
		{"trailing > requires at least one trailing token",
			"graph.ingest.>", "graph.ingest", false},

		// Length checks.
		{"shorter filter without > does not match", "graph.ingest", "graph.ingest.entity", false},
		{"longer filter does not match shorter subject",
			"graph.ingest.entity.detail", "graph.ingest.entity", false},

		// Control-plane probe matches.
		{"add subject under graph.ingest.>", "graph.ingest.>", "graph.ingest.add.runtimeadd", true},
		{"add subject under graph.ingest.add.*",
			"graph.ingest.add.*", "graph.ingest.add.runtimeadd", true},
		{"add subject not under graph.ingest.entity",
			"graph.ingest.entity", "graph.ingest.add.runtimeadd", false},
		{"add subject not under explicit data-plane filter set",
			"graph.ingest.manifest", "graph.ingest.add.runtimeadd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subjectFilterMatches(tt.filter, tt.subject); got != tt.want {
				t.Errorf("subjectFilterMatches(%q, %q) = %v, want %v",
					tt.filter, tt.subject, got, tt.want)
			}
		})
	}
}
