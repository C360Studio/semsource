package parser

import (
	"path/filepath"
	"strings"
)

// codeBlockState tracks code block parsing state.
type codeBlockState struct {
	active bool
	delim  string
}

// handleBlockDelimiter processes a block delimiter and updates state.
// Returns true if the line was consumed.
func (p *ASCIIDocParser) handleBlockDelimiter(trimmed string, result *[]string, state *codeBlockState) bool {
	switch {
	case adocListingBlock.MatchString(trimmed):
		if state.active && state.delim == "----" {
			*result = append(*result, "```")
			state.active = false
			state.delim = ""
		} else if !state.active {
			*result = append(*result, "```")
			state.active = true
			state.delim = "----"
		}
		return true
	case adocLiteralBlock.MatchString(trimmed):
		if state.active && state.delim == "...." {
			*result = append(*result, "```")
			state.active = false
			state.delim = ""
		} else if !state.active {
			*result = append(*result, "```")
			state.active = true
			state.delim = "...."
		}
		return true
	case adocPassthroughBlock.MatchString(trimmed):
		if state.active && state.delim == "++++" {
			state.active = false
			state.delim = ""
		} else if !state.active {
			state.active = true
			state.delim = "++++"
		}
		return true
	case adocSidebarBlock.MatchString(trimmed) || adocExampleBlock.MatchString(trimmed):
		return true
	}
	return false
}

// convertBlockMacro converts an AsciiDoc block macro to markdown-style.
func (p *ASCIIDocParser) convertBlockMacro(macroType, target, attrs string, result *[]string) {
	switch macroType {
	case "image":
		alt := attrs
		if alt == "" {
			alt = filepath.Base(target)
		}
		*result = append(*result, "!["+alt+"]("+target+")")
	case "include":
		*result = append(*result, "_[Include: "+target+"]_")
	default:
		*result = append(*result, "_["+macroType+": "+target+"]_")
	}
}

// convertInlineElement converts a non-delimiter AsciiDoc line to markdown-style.
func (p *ASCIIDocParser) convertInlineElement(trimmed, line string, result *[]string, state *codeBlockState) {
	if match := adocSourceBlock.FindStringSubmatch(trimmed); match != nil {
		if match[1] != "" {
			*result = append(*result, "```"+match[1])
		} else {
			*result = append(*result, "```")
		}
		state.active = true
		state.delim = "source"
		return
	}
	if match := adocSectionTitle.FindStringSubmatch(trimmed); match != nil {
		prefix := strings.Repeat("#", len(match[1]))
		*result = append(*result, prefix+" "+match[2])
		return
	}
	if match := adocAdmonition.FindStringSubmatch(trimmed); match != nil {
		*result = append(*result, "**"+match[1]+":** "+match[2])
		return
	}
	if match := adocBlockMacro.FindStringSubmatch(trimmed); match != nil {
		p.convertBlockMacro(match[1], match[2], match[3], result)
		return
	}
	*result = append(*result, line)
}

// convertToMarkdownStyle converts AsciiDoc to markdown-style format.
func (p *ASCIIDocParser) convertToMarkdownStyle(content string) string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	state := &codeBlockState{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if p.handleBlockDelimiter(trimmed, &result, state) {
			continue
		}

		if state.active {
			result = append(result, line)
			continue
		}

		p.convertInlineElement(trimmed, line, &result, state)
	}

	if state.active {
		result = append(result, "```")
	}

	return strings.Join(result, "\n")
}
