package parser

import (
	"testing"
)

func TestOpenSpecParser_Parse(t *testing.T) {
	parser := NewOpenSpecParser()

	content := `---
title: Authentication Spec
applies_to:
  - "auth/**/*.go"
  - "session/*.go"
---

# Authentication

This spec defines authentication requirements.

### Requirement: Token-Refresh

The system MUST refresh tokens before expiration.
The system SHALL NOT allow expired tokens.

#### Scenario: Valid Token Refresh

**GIVEN** a user has a valid refresh token
**WHEN** the token is about to expire
**THEN** a new access token is issued

### Requirement: Session-Timeout

Sessions SHALL timeout after 30 minutes of inactivity.
`

	doc, err := parser.Parse("auth.spec.md", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Filename != "auth.spec.md" {
		t.Errorf("Expected filename auth.spec.md, got %s", doc.Filename)
	}

	if doc.Frontmatter == nil {
		t.Fatal("Expected frontmatter to be parsed")
	}

	if doc.Frontmatter["title"] != "Authentication Spec" {
		t.Errorf("Expected title 'Authentication Spec', got %v", doc.Frontmatter["title"])
	}
}

func TestOpenSpecParser_ParseSpec(t *testing.T) {
	parser := NewOpenSpecParser()

	content := `---
title: Authentication Spec
applies_to:
  - "auth/**/*.go"
---

# Authentication

This spec defines authentication requirements.

### Requirement: Token-Refresh

The system MUST refresh tokens before expiration.
The system SHALL NOT allow expired tokens.

Applies to: ` + "`auth/token.go`" + `, ` + "`auth/refresh.go`" + `

#### Scenario: Valid Token Refresh

**GIVEN** a user has a valid refresh token
**WHEN** the token is about to expire
**THEN** a new access token is issued

#### Scenario: Expired Refresh Token

**GIVEN** a user has an expired refresh token
**WHEN** they attempt to refresh
**THEN** the request is rejected with 401

### Requirement: Session-Timeout

Sessions SHALL timeout after 30 minutes of inactivity.
`

	spec, err := parser.ParseSpec("auth.spec.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	// Check spec type
	if spec.Type != SpecTypeSourceOfTruth {
		t.Errorf("Expected type source-of-truth, got %s", spec.Type)
	}

	// Check title
	if spec.Title != "Authentication Spec" {
		t.Errorf("Expected title 'Authentication Spec', got %s", spec.Title)
	}

	// Check applies_to from frontmatter
	if len(spec.AppliesTo) != 1 {
		t.Errorf("Expected 1 applies_to pattern, got %d", len(spec.AppliesTo))
	}

	// Check requirements
	if len(spec.Requirements) != 2 {
		t.Fatalf("Expected 2 requirements, got %d", len(spec.Requirements))
	}

	// Check first requirement
	req := spec.Requirements[0]
	if req.Name != "Token-Refresh" {
		t.Errorf("Expected requirement name 'Token-Refresh', got '%s'", req.Name)
	}

	// Check normatives
	if len(req.Normatives) < 2 {
		t.Errorf("Expected at least 2 normatives, got %d", len(req.Normatives))
	}

	// Check scenarios
	if len(req.Scenarios) != 2 {
		t.Errorf("Expected 2 scenarios, got %d", len(req.Scenarios))
	}

	scenario := req.Scenarios[0]
	if scenario.Name != "Valid Token Refresh" {
		t.Errorf("Expected scenario name 'Valid Token Refresh', got '%s'", scenario.Name)
	}
	if scenario.Given == "" {
		t.Error("Expected Given clause to be extracted")
	}
	if scenario.When == "" {
		t.Error("Expected When clause to be extracted")
	}
	if scenario.Then == "" {
		t.Error("Expected Then clause to be extracted")
	}

	// Check applies_to on requirement
	if len(req.AppliesTo) != 2 {
		t.Errorf("Expected 2 applies_to patterns on requirement, got %d: %v", len(req.AppliesTo), req.AppliesTo)
	}

	// Check second requirement
	req2 := spec.Requirements[1]
	if req2.Name != "Session-Timeout" {
		t.Errorf("Expected requirement name 'Session-Timeout', got '%s'", req2.Name)
	}
}

func TestOpenSpecParser_ParseDeltaSpec(t *testing.T) {
	parser := NewOpenSpecParser()

	content := `---
title: Authentication Delta
modifies: auth.spec
---

# Authentication Changes

## ADDED Requirements

### Requirement: MFA-Support

The system MUST support multi-factor authentication.

#### Scenario: MFA Enrollment

**GIVEN** a user without MFA
**WHEN** they enable MFA
**THEN** they can authenticate with a second factor

## MODIFIED Requirements

### Requirement: Token-Refresh

The system MUST refresh tokens 5 minutes before expiration (was 1 minute).

## REMOVED Requirements

### Requirement: Legacy-Auth

This requirement has been deprecated.
`

	spec, err := parser.ParseSpec("auth-changes.delta.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseSpec failed: %v", err)
	}

	// Check spec type
	if spec.Type != SpecTypeDelta {
		t.Errorf("Expected type delta, got %s", spec.Type)
	}

	// Check delta operations
	if len(spec.DeltaOps) != 3 {
		t.Fatalf("Expected 3 delta operations, got %d", len(spec.DeltaOps))
	}

	// Check ADDED operation
	addedOp := spec.DeltaOps[0]
	if addedOp.Operation != DeltaOpAdded {
		t.Errorf("Expected operation 'added', got '%s'", addedOp.Operation)
	}
	if addedOp.Requirement.Name != "MFA-Support" {
		t.Errorf("Expected requirement 'MFA-Support', got '%s'", addedOp.Requirement.Name)
	}
	if len(addedOp.Requirement.Scenarios) != 1 {
		t.Errorf("Expected 1 scenario, got %d", len(addedOp.Requirement.Scenarios))
	}

	// Check MODIFIED operation
	modifiedOp := spec.DeltaOps[1]
	if modifiedOp.Operation != DeltaOpModified {
		t.Errorf("Expected operation 'modified', got '%s'", modifiedOp.Operation)
	}
	if modifiedOp.Requirement.Name != "Token-Refresh" {
		t.Errorf("Expected requirement 'Token-Refresh', got '%s'", modifiedOp.Requirement.Name)
	}

	// Check REMOVED operation
	removedOp := spec.DeltaOps[2]
	if removedOp.Operation != DeltaOpRemoved {
		t.Errorf("Expected operation 'removed', got '%s'", removedOp.Operation)
	}
	if removedOp.Requirement.Name != "Legacy-Auth" {
		t.Errorf("Expected requirement 'Legacy-Auth', got '%s'", removedOp.Requirement.Name)
	}
}

func TestOpenSpecParser_ExtractNormatives(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "single SHALL",
			text:     "The system SHALL validate tokens.",
			expected: 1,
		},
		{
			name:     "single MUST",
			text:     "The system MUST reject invalid requests.",
			expected: 1,
		},
		{
			name:     "multiple normatives",
			text:     "The system SHALL validate tokens. It MUST also log access. Users SHALL be authenticated.",
			expected: 3,
		},
		{
			name:     "no normatives",
			text:     "This is a description without normative language.",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normatives := extractNormatives(tt.text)
			if len(normatives) != tt.expected {
				t.Errorf("Expected %d normatives, got %d: %v", tt.expected, len(normatives), normatives)
			}
		})
	}
}

func TestOpenSpecParser_ParseScenarios(t *testing.T) {
	reqBody := `
#### Scenario: Valid Login

**GIVEN** a valid username and password
**WHEN** the user submits login
**THEN** they receive an access token

#### Scenario: Invalid Login

**GIVEN** an invalid password
**WHEN** the user submits login
**THEN** the request is rejected
`

	scenarios := parseScenarios(reqBody)

	if len(scenarios) != 2 {
		t.Fatalf("Expected 2 scenarios, got %d", len(scenarios))
	}

	// Check first scenario
	s1 := scenarios[0]
	if s1.Name != "Valid Login" {
		t.Errorf("Expected name 'Valid Login', got '%s'", s1.Name)
	}
	if s1.Given != "a valid username and password" {
		t.Errorf("Expected Given 'a valid username and password', got '%s'", s1.Given)
	}
	if s1.When != "the user submits login" {
		t.Errorf("Expected When 'the user submits login', got '%s'", s1.When)
	}
	if s1.Then != "they receive an access token" {
		t.Errorf("Expected Then 'they receive an access token', got '%s'", s1.Then)
	}
}

func TestIsOpenSpecFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"auth.spec.md", true},
		{"features/auth.spec.md", true},
		{"openspec/specs/auth.md", true},
		{"openspec/changes/proposal.md", true},
		{"docs/openspec/feature.md", true},
		{"README.md", false},
		{"docs/architecture.md", false},
		{"spec.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsOpenSpecFile(tt.path)
			if result != tt.expected {
				t.Errorf("IsOpenSpecFile(%s) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestOpenSpecParser_CanParse(t *testing.T) {
	parser := NewOpenSpecParser()

	tests := []struct {
		mimeType string
		expected bool
	}{
		{"text/x-openspec", true},
		{"text/markdown", false},
		{"text/plain", false},
		{"application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			if parser.CanParse(tt.mimeType) != tt.expected {
				t.Errorf("CanParse(%s) = %v, expected %v", tt.mimeType, !tt.expected, tt.expected)
			}
		})
	}
}

func TestOpenSpecParser_ExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "H1 heading",
			body:     "# Authentication\n\nSome content",
			expected: "Authentication",
		},
		{
			name:     "H1 with spaces",
			body:     "#   Authentication Spec  \n\nSome content",
			expected: "Authentication Spec",
		},
		{
			name:     "no H1",
			body:     "Some content\n\n## Section",
			expected: "",
		},
		{
			name:     "H1 after content",
			body:     "Intro\n# Title\nContent",
			expected: "Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := extractTitle(tt.body)
			if title != tt.expected {
				t.Errorf("extractTitle() = %q, expected %q", title, tt.expected)
			}
		})
	}
}

func TestOpenSpecParser_ExtractAppliesTo(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "single pattern",
			text:     "Applies to: `auth/*.go`",
			expected: []string{"auth/*.go"},
		},
		{
			name:     "multiple patterns",
			text:     "Applies to: `auth/*.go`, `session/*.go`",
			expected: []string{"auth/*.go", "session/*.go"},
		},
		{
			name:     "no backticks",
			text:     "Applies to: auth/*.go, session/*.go",
			expected: []string{"auth/*.go", "session/*.go"},
		},
		{
			name:     "no applies to",
			text:     "Some other content",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAppliesTo(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("extractAppliesTo() = %v, expected %v", result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("extractAppliesTo()[%d] = %q, expected %q", i, v, tt.expected[i])
				}
			}
		})
	}
}
