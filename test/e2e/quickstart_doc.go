// Package e2e runs black-box tests against the compiled semsource binary.
//
// This file is quickstart doc-block extraction: the marker grammar that makes
// docs/QUICKSTART.md executable truth. Deliberately NOT behind the e2e build
// tag — the parser's unit tests run in the ordinary `go test ./...` gate so a
// grammar regression fails fast, without Docker or the network tier.
package e2e

import (
	"fmt"
	"strings"
)

// quickstartTracks is the closed set of track names the grammar accepts.
// A marker naming any other track is malformed — this is what catches a typo
// like `quickstart:signle` at parse time instead of silently dropping the
// block from the executed track.
var quickstartTracks = map[string]bool{
	"single": true,
	"multi":  true,
}

// QuickstartBlock is one marked fenced command block extracted from the
// quickstart document, in document order.
type QuickstartBlock struct {
	// Heading is the nearest preceding markdown heading, for failure
	// messages that point a reader at the right doc section.
	Heading string
	// Line is the 1-based line number of the opening fence.
	Line int
	// Tracks the block belongs to (e.g. ["single"] or ["single","multi"]).
	Tracks []string
	// Script is the verbatim block body, ready for a shell.
	Script string
}

// InTrack reports whether the block belongs to the given track.
func (b QuickstartBlock) InTrack(track string) bool {
	for _, t := range b.Tracks {
		if t == track {
			return true
		}
	}
	return false
}

// ExtractQuickstartBlocks scans raw markdown for fenced code blocks whose
// info string carries a `quickstart:<track>[,<track>]` marker and returns
// them in document order, filtered to the given track. Unmarked fences are
// prose examples and are ignored. A marker that does not parse exactly is a
// loud error, never a silently skipped block.
func ExtractQuickstartBlocks(doc string, track string) ([]QuickstartBlock, error) {
	if !quickstartTracks[track] {
		return nil, fmt.Errorf("unknown quickstart track %q", track)
	}
	all, err := parseQuickstartBlocks(doc)
	if err != nil {
		return nil, err
	}
	var out []QuickstartBlock
	for _, b := range all {
		if b.InTrack(track) {
			out = append(out, b)
		}
	}
	return out, nil
}

// parseQuickstartBlocks extracts every marked block regardless of track.
func parseQuickstartBlocks(doc string) ([]QuickstartBlock, error) {
	var (
		blocks    []QuickstartBlock
		heading   string
		inFence   bool
		fenceMark string // the exact opening fence run, e.g. "```" or "````"
		current   *QuickstartBlock
		body      []string
	)

	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)

		if inFence {
			// Only a closing fence at least as long as the opener ends the
			// block; anything else (including shorter backtick runs) is body.
			if isClosingFence(trimmed, fenceMark) {
				if current != nil {
					current.Script = strings.Join(body, "\n")
					blocks = append(blocks, *current)
					current = nil
				}
				inFence = false
				body = nil
				continue
			}
			if current != nil {
				body = append(body, line)
			}
			continue
		}

		if isHeading(trimmed) {
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			continue
		}

		if mark := fenceRun(trimmed); mark != "" {
			info := strings.TrimSpace(strings.TrimPrefix(trimmed, mark))
			tracks, err := parseMarker(info)
			if err != nil {
				return nil, fmt.Errorf("line %d (%s): %w", lineNo, headingOr(heading), err)
			}
			inFence = true
			fenceMark = mark
			body = nil
			if tracks != nil {
				current = &QuickstartBlock{
					Heading: headingOr(heading),
					Line:    lineNo,
					Tracks:  tracks,
				}
			}
		}
	}
	if inFence {
		return nil, fmt.Errorf("unterminated fence (last heading: %s)", headingOr(heading))
	}
	return blocks, nil
}

// parseMarker inspects a fence info string. It returns nil tracks for an
// unmarked fence, the track list for a well-formed marker, and an error for
// anything that mentions "quickstart" but does not parse exactly.
func parseMarker(info string) ([]string, error) {
	fields := strings.Fields(info)
	var marker string
	for _, f := range fields {
		if strings.HasPrefix(f, "quickstart") {
			if marker != "" {
				return nil, fmt.Errorf("multiple quickstart markers in info string %q", info)
			}
			marker = f
		}
	}
	if marker == "" {
		return nil, nil
	}
	rest, ok := strings.CutPrefix(marker, "quickstart:")
	if !ok || rest == "" {
		return nil, fmt.Errorf("malformed quickstart marker %q (want quickstart:<track>[,<track>])", marker)
	}
	var tracks []string
	seen := map[string]bool{}
	for _, tr := range strings.Split(rest, ",") {
		if !quickstartTracks[tr] {
			return nil, fmt.Errorf("unknown track %q in marker %q (valid: single, multi)", tr, marker)
		}
		if seen[tr] {
			return nil, fmt.Errorf("duplicate track %q in marker %q", tr, marker)
		}
		seen[tr] = true
		tracks = append(tracks, tr)
	}
	return tracks, nil
}

// fenceRun returns the leading backtick run ("```" or longer) if the line
// opens a fence, else "".
func fenceRun(trimmed string) string {
	if !strings.HasPrefix(trimmed, "```") {
		return ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '`' {
		n++
	}
	return trimmed[:n]
}

// isClosingFence reports whether the line closes a fence opened with mark.
func isClosingFence(trimmed, mark string) bool {
	if !strings.HasPrefix(trimmed, mark) {
		return false
	}
	return strings.Trim(trimmed, "`") == ""
}

// isHeading reports whether the line is a markdown ATX heading.
func isHeading(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	rest := strings.TrimLeft(trimmed, "#")
	return strings.HasPrefix(rest, " ") && strings.TrimSpace(rest) != ""
}

// headingOr labels blocks that precede the first heading.
func headingOr(h string) string {
	if h == "" {
		return "(before first heading)"
	}
	return h
}
