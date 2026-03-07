package source

// DocCategoryType represents document classification categories.
type DocCategoryType string

const (
	// DocCategorySOP is a Standard Operating Procedure document.
	// SOPs define validation rules and review requirements.
	DocCategorySOP DocCategoryType = "sop"

	// DocCategorySpec is a technical specification document.
	// Specs define system behavior and requirements.
	DocCategorySpec DocCategoryType = "spec"

	// DocCategoryDatasheet is a data format documentation.
	// Datasheets describe data structures and formats.
	DocCategoryDatasheet DocCategoryType = "datasheet"

	// DocCategoryReference is general reference material.
	// Reference docs provide background and context.
	DocCategoryReference DocCategoryType = "reference"

	// DocCategoryAPI is API documentation.
	// API docs describe endpoints, schemas, and usage.
	DocCategoryAPI DocCategoryType = "api"
)

// DocSeverityType represents SOP violation severity levels.
type DocSeverityType string

const (
	// DocSeverityError indicates a violation that blocks approval.
	// Reviewer cannot approve if this SOP is violated.
	DocSeverityError DocSeverityType = "error"

	// DocSeverityWarning indicates a violation that should be addressed.
	// Reviewer has discretion on whether to approve.
	DocSeverityWarning DocSeverityType = "warning"

	// DocSeverityInfo indicates informational guidance.
	// No enforcement; purely advisory.
	DocSeverityInfo DocSeverityType = "info"
)

// DocScopeType represents when a document applies in the workflow.
type DocScopeType string

const (
	// DocScopePlan indicates the document applies during plan approval.
	// Used for architectural decisions, scope validation, migration planning.
	DocScopePlan DocScopeType = "plan"

	// DocScopeCode indicates the document applies during implementation/review.
	// Used for coding standards, error handling, naming conventions.
	DocScopeCode DocScopeType = "code"

	// DocScopeAll indicates the document applies to both planning and implementation.
	// Used for security policies, compliance requirements, universal standards.
	DocScopeAll DocScopeType = "all"
)

// StatusType represents the status of a source.
type StatusType string

const (
	// SourceStatusPending indicates the source is queued for processing.
	SourceStatusPending StatusType = "pending"

	// SourceStatusIndexing indicates the source is currently being processed.
	SourceStatusIndexing StatusType = "indexing"

	// SourceStatusReady indicates the source is fully processed and available.
	SourceStatusReady StatusType = "ready"

	// SourceStatusError indicates processing failed.
	SourceStatusError StatusType = "error"

	// SourceStatusStale indicates the source may be out of date.
	// For repos: new commits available. For docs: file modified.
	SourceStatusStale StatusType = "stale"
)

// TypeValue represents the type discriminator for sources.
type TypeValue string

const (
	// SourceTypeRepository indicates an external git repository.
	SourceTypeRepository TypeValue = "repository"

	// SourceTypeDocument indicates an ingested document.
	SourceTypeDocument TypeValue = "document"

	// SourceTypeWeb indicates a web URL source.
	SourceTypeWeb TypeValue = "web"
)

// DomainType represents semantic domains for domain-aware SOP matching.
// Used to classify documents by subject matter, enabling smart context
// gathering during code review - when touching auth code, find all
// auth-domain SOPs regardless of file path patterns.
type DomainType string

const (
	// DomainAuth covers authentication, authorization, sessions, tokens.
	DomainAuth DomainType = "auth"

	// DomainDatabase covers database operations, migrations, queries, transactions.
	DomainDatabase DomainType = "database"

	// DomainAPI covers API design, endpoints, request/response handling.
	DomainAPI DomainType = "api"

	// DomainSecurity covers security practices, cryptography, secrets management.
	DomainSecurity DomainType = "security"

	// DomainTesting covers testing practices, test organization, coverage.
	DomainTesting DomainType = "testing"

	// DomainLogging covers logging, observability, metrics, tracing.
	DomainLogging DomainType = "logging"

	// DomainMessaging covers message queues, event systems, pub/sub patterns.
	DomainMessaging DomainType = "messaging"

	// DomainDeployment covers CI/CD, infrastructure, containerization.
	DomainDeployment DomainType = "deployment"

	// DomainPerformance covers optimization, caching, benchmarking.
	DomainPerformance DomainType = "performance"

	// DomainErrorHandling covers error handling, recovery, resilience patterns.
	DomainErrorHandling DomainType = "error-handling"

	// DomainValidation covers input validation, data sanitization.
	DomainValidation DomainType = "validation"

	// DomainConfig covers configuration management, environment handling.
	DomainConfig DomainType = "config"
)

// RelatedDomains maps domains to conceptually related domains.
// Used for cross-domain SOP inclusion during code review.
var RelatedDomains = map[DomainType][]DomainType{
	DomainAuth:          {DomainSecurity, DomainValidation},
	DomainDatabase:      {DomainErrorHandling, DomainPerformance},
	DomainAPI:           {DomainSecurity, DomainValidation, DomainErrorHandling},
	DomainSecurity:      {DomainValidation, DomainErrorHandling},
	DomainMessaging:     {DomainErrorHandling, DomainLogging},
	DomainDeployment:    {DomainConfig, DomainLogging},
	DomainPerformance:   {DomainLogging},
	DomainErrorHandling: {DomainLogging},
}
