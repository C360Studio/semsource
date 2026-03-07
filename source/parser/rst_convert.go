package parser

import "strings"

// rstConvertState tracks RST-to-markdown conversion state.
type rstConvertState struct {
	inCodeBlock      bool
	codeBlockIndent  int
	underlineToLevel map[rune]int
	currentLevel     int
}

// newRSTConvertState initializes a fresh conversion state.
func newRSTConvertState() *rstConvertState {
	return &rstConvertState{
		underlineToLevel: make(map[rune]int),
		currentLevel:     1,
	}
}

// handleCodeBlockDirective processes a .. code-block:: directive line.
// Returns true if the line was consumed.
func (p *RSTParser) handleCodeBlockDirective(line string, result *[]string, state *rstConvertState) bool {
	if !rstCodeBlockDirective.MatchString(strings.TrimSpace(line)) {
		return false
	}
	state.inCodeBlock = true
	state.codeBlockIndent = len(line) - len(strings.TrimLeft(line, " 	"))
	*result = append(*result, "```")
	return true
}

// handleShortCodeBlock processes a line ending with :: that precedes indented content.
// Returns true if the line started a code block (caller should skip iteration).
func (p *RSTParser) handleShortCodeBlock(i int, lines []string, result *[]string, state *rstConvertState) bool {
	if state.inCodeBlock {
		return false
	}
	line := lines[i]
	if !rstCodeBlockShort.MatchString(strings.TrimSpace(line)) || i+1 >= len(lines) {
		return false
	}
	for j := i + 1; j < len(lines); j++ {
		nextLine := lines[j]
		if strings.TrimSpace(nextLine) == "" {
			continue
		}
		if len(nextLine) > 0 && (nextLine[0] == ' ' || nextLine[0] == '	') {
			stripped := strings.TrimSuffix(strings.TrimSpace(line), ":")
			stripped = strings.TrimSuffix(stripped, ":")
			*result = append(*result, stripped)
			*result = append(*result, "```")
			state.inCodeBlock = true
			state.codeBlockIndent = len(nextLine) - len(strings.TrimLeft(nextLine, " 	"))
		}
		break
	}
	return state.inCodeBlock
}

// processCodeBlockLine handles a line inside an active RST code block.
// Closes the block if indentation decreases.
func (p *RSTParser) processCodeBlockLine(line string, result *[]string, state *rstConvertState) {
	if strings.TrimSpace(line) != "" {
		currentIndent := len(line) - len(strings.TrimLeft(line, " 	"))
		if currentIndent < state.codeBlockIndent {
			*result = append(*result, "```")
			state.inCodeBlock = false
		} else if len(line) >= state.codeBlockIndent {
			line = line[state.codeBlockIndent:]
		}
	}
	*result = append(*result, line)
}

// processSectionTitle checks if line[i] is a section title followed by an underline.
// Returns the number of lines to skip (1 to skip the underline), or 0 if not matched.
func (p *RSTParser) processSectionTitle(i int, lines []string, result *[]string, state *rstConvertState) int {
	if i+1 >= len(lines) {
		return 0
	}
	underline := strings.TrimSpace(lines[i+1])
	if !rstSectionUnderline.MatchString(underline) {
		return 0
	}
	title := strings.TrimSpace(lines[i])
	if len(underline) < len(title) || title == "" {
		return 0
	}
	titleChar := rune(underline[0])
	level, exists := state.underlineToLevel[titleChar]
	if !exists {
		level = state.currentLevel
		state.underlineToLevel[titleChar] = level
		state.currentLevel++
		if state.currentLevel > 6 {
			state.currentLevel = 6
		}
	}
	prefix := strings.Repeat("#", level)
	*result = append(*result, prefix+" "+title)
	return 1
}

// convertToMarkdownStyle converts RST sections to markdown-style headings.
func (p *RSTParser) convertToMarkdownStyle(content string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	state := newRSTConvertState()

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if p.handleCodeBlockDirective(line, &result, state) {
			continue
		}

		if p.handleShortCodeBlock(i, lines, &result, state) {
			continue
		}

		if state.inCodeBlock {
			p.processCodeBlockLine(line, &result, state)
			continue
		}

		if skip := p.processSectionTitle(i, lines, &result, state); skip > 0 {
			i += skip
			continue
		}

		if match := rstFieldList.FindStringSubmatch(line); match != nil {
			result = append(result, "**"+match[1]+":**"+match[2])
			continue
		}

		if rstDirective.MatchString(strings.TrimSpace(line)) {
			continue
		}

		result = append(result, line)
	}

	if state.inCodeBlock {
		result = append(result, "```")
	}

	return strings.Join(result, "\n")
}
