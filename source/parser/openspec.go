package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/c360studio/semsource/source"
)

// OpenSpec delta operation types.
const (
	DeltaOpAdded    = "added"
	DeltaOpModified = "modified"
	DeltaOpRemoved  = "removed"
)

// OpenSpec spec types.
const (
	SpecTypeSourceOfTruth = "source-of-truth"
	SpecTypeDelta         = "delta"
)

// Regex patterns for OpenSpec parsing.
var (
	reqHeaderPattern     = regexp.MustCompile(`(?m)^###\s+Requirement:\s+(.+)$`)
	scenarioPattern      = regexp.MustCompile(`(?m)^####\s+Scenario:\s+(.+)$`)
	normativePattern     = regexp.MustCompile(`(?:SHALL|MUST)\s+[^.]+\.`)
	givenPattern         = regexp.MustCompile(`(?i)\*\*GIVEN\*\*\s+(.+)`)
	whenPattern          = regexp.MustCompile(`(?i)\*\*WHEN\*\*\s+(.+)`)
	thenPattern          = regexp.MustCompile(`(?i)\*\*THEN\*\*\s+(.+)`)
	deltaSectionPattern  = regexp.MustCompile(`(?m)^##\s+(ADDED|MODIFIED|REMOVED)\s+Requirements`)
	appliesToPattern     = regexp.MustCompile(`(?m)^Applies to:\s*(.+)$`)
	givenWhenThenPattern = regexp.MustCompile(`(?s)(?:\*\*GIVEN\*\*|\*Given\*|Given:)\s*(.+?)(?:\*\*WHEN\*\*|\*When\*|When:)\s*(.+?)(?:\*\*THEN\*\*|\*Then\*|Then:)\s*(.+?)(?:\n\n|$)`)
)

// ParsedSpec represents a fully parsed OpenSpec document.
type ParsedSpec struct {
	// Type is "source-of-truth" or "delta".
	Type string `json:"type"`

	// FilePath is the original file path.
	FilePath string `json:"file_path"`

	// FileHash is the content hash.
	FileHash string `json:"file_hash"`

	// Title is the spec document title.
	Title string `json:"title,omitempty"`

	// AppliesTo are file patterns this spec applies to.
	AppliesTo []string `json:"applies_to,omitempty"`

	// Requirements are the parsed requirements.
	Requirements []Requirement `json:"requirements"`

	// DeltaOps are delta operations (only for delta specs).
	DeltaOps []DeltaOperation `json:"delta_ops,omitempty"`

	// Frontmatter contains any YAML frontmatter.
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
}

// Requirement represents a normative requirement block.
type Requirement struct {
	// Name is the requirement identifier (from ### Requirement: header).
	Name string `json:"name"`

	// Description is the full requirement text.
	Description string `json:"description"`

	// Normatives are SHALL/MUST statements extracted from the requirement.
	Normatives []string `json:"normatives"`

	// Scenarios are BDD scenarios for this requirement.
	Scenarios []Scenario `json:"scenarios"`

	// AppliesTo are file patterns this requirement applies to.
	AppliesTo []string `json:"applies_to,omitempty"`
}

// Scenario represents a BDD-style scenario.
type Scenario struct {
	// Name is the scenario identifier (from #### Scenario: header).
	Name string `json:"name"`

	// Given is the precondition.
	Given string `json:"given"`

	// When is the action.
	When string `json:"when"`

	// Then is the expected result.
	Then string `json:"then"`
}

// DeltaOperation represents a change in a delta spec.
type DeltaOperation struct {
	// Operation is "added", "modified", or "removed".
	Operation string `json:"operation"`

	// Requirement is the requirement being changed.
	Requirement Requirement `json:"requirement"`
}

// OpenSpecParser parses OpenSpec-formatted markdown documents.
type OpenSpecParser struct{}

// NewOpenSpecParser creates a new OpenSpec parser.
func NewOpenSpecParser() *OpenSpecParser {
	return &OpenSpecParser{}
}

// Parse parses an OpenSpec document.
func (p *OpenSpecParser) Parse(filename string, content []byte) (*source.Document, error) {
	doc := &source.Document{
		ID:       GenerateDocID("openspec", filename, content),
		Filename: filepath.Base(filename),
		Content:  string(content),
	}

	// Parse frontmatter if present
	str := string(content)
	if strings.HasPrefix(str, "---\n") || strings.HasPrefix(str, "---\r\n") {
		frontmatter, body, err := extractFrontmatter(str)
		if err != nil {
			doc.Body = str
		} else {
			doc.Frontmatter = frontmatter
			doc.Body = body
		}
	} else {
		doc.Body = str
	}

	return doc, nil
}

// ParseSpec parses an OpenSpec document and returns structured data.
func (p *OpenSpecParser) ParseSpec(filename string, content []byte) (*ParsedSpec, error) {
	// First parse as a document
	doc, err := p.Parse(filename, content)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(content)

	spec := &ParsedSpec{
		FilePath:    filename,
		FileHash:    hex.EncodeToString(hash[:]),
		Frontmatter: doc.Frontmatter,
	}

	// Extract title from frontmatter or first heading
	if title, ok := doc.Frontmatter["title"].(string); ok {
		spec.Title = title
	} else {
		spec.Title = extractTitle(doc.Body)
	}

	// Extract applies_to from frontmatter
	if appliesTo, ok := doc.Frontmatter["applies_to"].([]any); ok {
		for _, v := range appliesTo {
			if s, ok := v.(string); ok {
				spec.AppliesTo = append(spec.AppliesTo, s)
			}
		}
	}

	// Detect if this is a delta spec
	isDelta := detectDeltaSpec(doc.Body)
	if isDelta {
		spec.Type = SpecTypeDelta
		spec.DeltaOps = parseDeltaOperations(doc.Body)
	} else {
		spec.Type = SpecTypeSourceOfTruth
		spec.Requirements = parseRequirements(doc.Body)
	}

	return spec, nil
}

// CanParse returns true if this parser can handle the given MIME type.
func (p *OpenSpecParser) CanParse(mimeType string) bool {
	return mimeType == "text/x-openspec"
}

// MimeType returns the primary MIME type for this parser.
func (p *OpenSpecParser) MimeType() string {
	return "text/x-openspec"
}

// detectDeltaSpec checks if the document contains delta sections.
func detectDeltaSpec(body string) bool {
	return deltaSectionPattern.MatchString(body)
}

// extractTitle extracts the first H1 heading from the body.
func extractTitle(body string) string {
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
	}
	return ""
}

// parseRequirements extracts all requirements from a source-of-truth spec.
func parseRequirements(body string) []Requirement {
	var requirements []Requirement

	// Find all requirement headers
	matches := reqHeaderPattern.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return requirements
	}

	for i, match := range matches {
		// Extract requirement name
		nameStart := match[2]
		nameEnd := match[3]
		name := strings.TrimSpace(body[nameStart:nameEnd])

		// Find the requirement body (until next requirement or end)
		bodyStart := match[1]
		var bodyEnd int
		if i < len(matches)-1 {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(body)
		}

		reqBody := body[bodyStart:bodyEnd]

		req := Requirement{
			Name:        name,
			Description: strings.TrimSpace(reqBody),
			Normatives:  extractNormatives(reqBody),
			Scenarios:   parseScenarios(reqBody),
			AppliesTo:   extractAppliesTo(reqBody),
		}

		requirements = append(requirements, req)
	}

	return requirements
}

// extractNormatives finds all SHALL/MUST statements in the text.
func extractNormatives(text string) []string {
	var normatives []string
	matches := normativePattern.FindAllString(text, -1)
	for _, m := range matches {
		normatives = append(normatives, strings.TrimSpace(m))
	}
	return normatives
}

// extractAppliesTo finds "Applies to:" patterns in text.
func extractAppliesTo(text string) []string {
	var patterns []string
	matches := appliesToPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			// Split by comma and clean up
			parts := strings.Split(match[1], ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				p = strings.Trim(p, "`")
				if p != "" {
					patterns = append(patterns, p)
				}
			}
		}
	}
	return patterns
}

// parseScenarios extracts all scenarios from a requirement body.
func parseScenarios(reqBody string) []Scenario {
	var scenarios []Scenario

	// Find all scenario headers
	matches := scenarioPattern.FindAllStringSubmatchIndex(reqBody, -1)
	if len(matches) == 0 {
		return scenarios
	}

	for i, match := range matches {
		// Extract scenario name
		nameStart := match[2]
		nameEnd := match[3]
		name := strings.TrimSpace(reqBody[nameStart:nameEnd])

		// Find the scenario body
		bodyStart := match[1]
		var bodyEnd int
		if i < len(matches)-1 {
			bodyEnd = matches[i+1][0]
		} else {
			bodyEnd = len(reqBody)
		}

		scenBody := reqBody[bodyStart:bodyEnd]

		scenario := Scenario{
			Name: name,
		}

		// Try the combined pattern first
		gwtMatches := givenWhenThenPattern.FindStringSubmatch(scenBody)
		if len(gwtMatches) >= 4 {
			scenario.Given = cleanGWT(gwtMatches[1])
			scenario.When = cleanGWT(gwtMatches[2])
			scenario.Then = cleanGWT(gwtMatches[3])
		} else {
			// Fall back to individual patterns
			if m := givenPattern.FindStringSubmatch(scenBody); len(m) > 1 {
				scenario.Given = cleanGWT(m[1])
			}
			if m := whenPattern.FindStringSubmatch(scenBody); len(m) > 1 {
				scenario.When = cleanGWT(m[1])
			}
			if m := thenPattern.FindStringSubmatch(scenBody); len(m) > 1 {
				scenario.Then = cleanGWT(m[1])
			}
		}

		scenarios = append(scenarios, scenario)
	}

	return scenarios
}

// cleanGWT cleans up a Given/When/Then clause.
func cleanGWT(s string) string {
	s = strings.TrimSpace(s)
	// Remove trailing bold markers
	s = strings.TrimSuffix(s, "**")
	s = strings.TrimSuffix(s, "*")
	// Clean up newlines
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// parseDeltaOperations extracts delta operations from a delta spec.
func parseDeltaOperations(body string) []DeltaOperation {
	var ops []DeltaOperation

	// Find all delta section headers
	sectionMatches := deltaSectionPattern.FindAllStringSubmatchIndex(body, -1)
	if len(sectionMatches) == 0 {
		return ops
	}

	for i, match := range sectionMatches {
		// Extract operation type
		opStart := match[2]
		opEnd := match[3]
		opType := strings.ToLower(body[opStart:opEnd])

		// Find section body
		bodyStart := match[1]
		var bodyEnd int
		if i < len(sectionMatches)-1 {
			bodyEnd = sectionMatches[i+1][0]
		} else {
			bodyEnd = len(body)
		}

		sectionBody := body[bodyStart:bodyEnd]

		// Parse requirements within this delta section
		reqs := parseRequirements(sectionBody)
		for _, req := range reqs {
			ops = append(ops, DeltaOperation{
				Operation:   opType,
				Requirement: req,
			})
		}
	}

	return ops
}

// IsOpenSpecFile checks if a file path looks like an OpenSpec file.
// Returns true for .spec.md files or files in openspec/ directories.
func IsOpenSpecFile(path string) bool {
	// Check for .spec.md extension
	if strings.HasSuffix(path, ".spec.md") {
		return true
	}

	// Check for openspec directory
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if part == "openspec" {
			return true
		}
	}

	return false
}
