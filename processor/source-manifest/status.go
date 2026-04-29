package sourcemanifest

import (
	"strings"
	"time"

	"github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/vocabulary"
)

const (
	// statusReportSubject is the internal NATS subject for source status reports.
	statusReportSubject = "semsource.internal.status"

	// statusSubject is the NATS subject for publishing status to consumers.
	statusSubject = "graph.ingest.status"

	// statusQuerySubject is the NATS subject for on-demand status queries.
	statusQuerySubject = "graph.query.status"

	// predicatesSubject is the NATS subject for publishing predicate schema.
	predicatesSubject = "graph.ingest.predicates"

	// predicatesQuerySubject is the NATS subject for on-demand predicate schema queries.
	predicatesQuerySubject = "graph.query.predicates"
)

// Phase constants for aggregate ingestion status.
const (
	PhaseSeeding  = "seeding"
	PhaseReady    = "ready"
	PhaseDegraded = "degraded"
)

// Source phase constants.
const (
	SourcePhaseIngesting = "ingesting"
	SourcePhaseWatching  = "watching"
	SourcePhaseIdle      = "idle"
	SourcePhaseErrored   = "errored"
)

// Predicate role constants.
const (
	RoleIdentity     = "identity"
	RoleContent      = "content"
	RoleLocation     = "location"
	RoleRelationship = "relationship"
	RoleMetric       = "metric"
	RoleMetadata     = "metadata"
)

// SourceStatusReport is the internal message published by source components
// to semsource.internal.status after initial ingest and periodically.
type SourceStatusReport struct {
	InstanceName string           `json:"instance_name"`
	SourceType   string           `json:"source_type"`
	Phase        string           `json:"phase"`
	EntityCount  int64            `json:"entity_count"`
	ErrorCount   int64            `json:"error_count"`
	TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
	LastError    *SourceError     `json:"last_error,omitempty"`
	Timestamp    time.Time        `json:"timestamp"`
}

// statusAggregator tracks per-source status reports and determines the
// aggregate ingestion phase.
type statusAggregator struct {
	reports       map[string]*SourceStatusReport // instance_name → latest report
	expectedCount int
	degraded      bool // set by markDegraded, sticky until allReported
}

func newStatusAggregator(expectedCount int) *statusAggregator {
	return &statusAggregator{
		reports:       make(map[string]*SourceStatusReport),
		expectedCount: expectedCount,
	}
}

// update records a status report and returns the current aggregate status.
func (a *statusAggregator) update(report *SourceStatusReport) {
	a.reports[report.InstanceName] = report
}

// buildStatus constructs a StatusPayload from the current aggregated state.
func (a *statusAggregator) buildStatus(namespace string) *StatusPayload {
	sources := make([]SourceStatus, 0, len(a.reports))
	var totalEntities int64

	for _, r := range a.reports {
		sources = append(sources, SourceStatus{
			InstanceName: r.InstanceName,
			SourceType:   r.SourceType,
			Phase:        r.Phase,
			EntityCount:  r.EntityCount,
			ErrorCount:   r.ErrorCount,
			TypeCounts:   r.TypeCounts,
			LastError:    r.LastError,
		})
		totalEntities += r.EntityCount
	}

	phase := PhaseSeeding
	if a.allReported() {
		phase = PhaseReady
	} else if a.degraded {
		phase = PhaseDegraded
	}

	return &StatusPayload{
		Namespace:     namespace,
		Phase:         phase,
		Sources:       sources,
		TotalEntities: totalEntities,
		Timestamp:     time.Now(),
	}
}

// allReported returns true when all expected sources have reported.
func (a *statusAggregator) allReported() bool {
	if a.expectedCount <= 0 {
		return len(a.reports) > 0
	}
	return len(a.reports) >= a.expectedCount
}

// markDegraded forces the aggregate to degraded phase (used on seed timeout).
func (a *statusAggregator) markDegraded(namespace string) *StatusPayload {
	a.degraded = true
	return a.buildStatus(namespace)
}

// sourcePredicates maps source types to predicate prefixes.
var sourcePredicatePrefixes = map[string][]string{
	"ast":    {"code.", "dc.", "agentic."},
	"git":    {"source.git."},
	"docs":   {"source.doc."},
	"config": {"source.config."},
	"url":    {"source.web."},
	"image":  {"source.media."},
	"video":  {"source.media."},
	"audio":  {"source.media."},
}

// buildPredicateSchema constructs a PredicateSchemaPayload from the vocabulary registry.
func buildPredicateSchema(sourceTypes []string) *PredicateSchemaPayload {
	allPredicates := vocabulary.ListRegisteredPredicates()

	schemas := make([]SourcePredicateSchema, 0, len(sourceTypes))
	for _, srcType := range sourceTypes {
		prefixes := sourcePredicatePrefixes[srcType]
		if len(prefixes) == 0 {
			continue
		}

		var descriptors []PredicateDescriptor
		for _, pred := range allPredicates {
			if !matchesPrefixes(pred, prefixes) {
				continue
			}
			meta := vocabulary.GetPredicateMetadata(pred)
			desc := ""
			dataType := "string"
			if meta != nil {
				desc = meta.Description
				if meta.DataType != "" {
					dataType = meta.DataType
				}
			}
			descriptors = append(descriptors, PredicateDescriptor{
				Name:        pred,
				Description: desc,
				DataType:    dataType,
				Role:        predicateRole(pred),
			})
		}

		schemas = append(schemas, SourcePredicateSchema{
			SourceType: srcType,
			Predicates: descriptors,
		})
	}

	return &PredicateSchemaPayload{
		Sources:   schemas,
		Timestamp: time.Now(),
	}
}

func matchesPrefixes(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// predicateRole classifies a predicate into a semantic role for consumers.
func predicateRole(name string) string {
	// Explicit overrides for predicates where pattern matching is ambiguous.
	if role, ok := explicitRoles[name]; ok {
		return role
	}
	// Pattern-based classification.
	return patternRole(name)
}

// explicitRoles maps specific predicates to their semantic roles.
var explicitRoles = map[string]string{
	// Location predicates
	ast.CodePath:          RoleLocation,
	source.DocFilePath:    RoleLocation,
	source.ConfigFilePath: RoleLocation,
	source.MediaFilePath:  RoleLocation,
	source.WebURL:         RoleLocation,
	ast.CodeStartLine:     RoleLocation,
	ast.CodeEndLine:       RoleLocation,

	// Identity predicates
	ast.DcTitle:             RoleIdentity,
	ast.CodeType:            RoleIdentity,
	ast.CodePackage:         RoleIdentity,
	ast.CodeVisibility:      RoleIdentity,
	ast.CodeLanguage:        RoleIdentity,
	ast.CodeFramework:       RoleIdentity,
	source.DocType:          RoleIdentity,
	source.WebType:          RoleIdentity,
	source.RepoType:         RoleIdentity,
	source.MediaType:        RoleIdentity,
	source.GitCommitSHA:     RoleIdentity,
	source.GitAuthorName:    RoleIdentity,
	source.GitAuthorEmail:   RoleIdentity,
	source.GitBranchName:    RoleIdentity,
	source.GitCommitSubject: RoleIdentity,
	source.ConfigModulePath: RoleIdentity,
	source.ConfigPkgName:    RoleIdentity,

	// Content predicates
	source.DocContent:             RoleContent,
	source.WebContent:             RoleContent,
	ast.CodeDocComment:            RoleContent,
	source.DocSummary:             RoleContent,
	source.WebSummary:             RoleContent,
	source.DocRequirements:        RoleContent,
	source.WebRequirements:        RoleContent,
	source.MediaVisionDescription: RoleContent,
	source.MediaVisionText:        RoleContent,

	// Relationship predicates that don't match the pattern
	source.ConfigRequires:      RoleRelationship,
	source.ConfigDepends:       RoleRelationship,
	source.ConfigContains:      RoleRelationship,
	source.GitCommitAuthoredBy: RoleRelationship,
	source.GitCommitTouches:    RoleRelationship,
	source.MediaKeyframeOf:     RoleRelationship,
}

// patternRole classifies predicates by their dotted-notation category segment.
func patternRole(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) < 3 {
		return RoleMetadata
	}

	category := parts[len(parts)-2]
	switch category {
	case "relationship", "structure", "dependency":
		return RoleRelationship
	case "metric":
		return RoleMetric
	}

	// Check by second segment (domain.category.property pattern)
	if len(parts) >= 3 {
		switch parts[1] {
		case "relationship", "structure", "dependency":
			return RoleRelationship
		case "metric":
			return RoleMetric
		}
	}

	return RoleMetadata
}
