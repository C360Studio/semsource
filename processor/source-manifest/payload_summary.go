package sourcemanifest

import "time"

// SummaryPayload provides a comprehensive graph overview for agent bootstrap.
// Combines status, entity type breakdown by domain, and predicate schema
// into a single response. Served at GET /source-manifest/summary and
// via NATS request/reply on graph.query.summary.
type SummaryPayload struct {
	Namespace      string                 `json:"namespace"`
	Phase          string                 `json:"phase"`
	EntityIDFormat string                 `json:"entity_id_format"`
	TotalEntities  int64                  `json:"total_entities"`
	Domains        []DomainSummary        `json:"domains"`
	Predicates     []SourcePredicateSchema `json:"predicates"`
	Timestamp      time.Time              `json:"timestamp"`
}

// DomainSummary reports entity counts by type within a single domain.
type DomainSummary struct {
	Domain      string      `json:"domain"`
	EntityCount int64       `json:"entity_count"`
	Types       []TypeCount `json:"types"`
	Sources     []string    `json:"sources"`
}

// TypeCount is an entity type and its count within a domain.
type TypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}
