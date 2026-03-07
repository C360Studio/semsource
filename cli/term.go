// Package cli provides terminal I/O helpers and interactive wizards for
// configuring semsource from the command line.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ANSI escape codes for minimal terminal styling.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
)

// Term wraps terminal I/O so prompts are testable via piped input.
type Term struct {
	scanner *bufio.Scanner
	out     io.Writer
}

// NewTerm creates a Term reading from r and writing to w.
func NewTerm(r io.Reader, w io.Writer) *Term {
	return &Term{
		scanner: bufio.NewScanner(r),
		out:     w,
	}
}

// readLine reads the next line of input, trimming whitespace.
// Returns the line and true, or "" and false on EOF/error.
func (t *Term) readLine() (string, bool) {
	if t.scanner.Scan() {
		return strings.TrimSpace(t.scanner.Text()), true
	}
	return "", false
}

// Prompt prints label and reads a line. If the user enters nothing,
// defaultVal is returned. The default is shown in brackets when non-empty.
func (t *Term) Prompt(label string, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(t.out, "%s [%s%s%s]: ", label, ansiDim, defaultVal, ansiReset)
	} else {
		fmt.Fprintf(t.out, "%s: ", label)
	}
	line, _ := t.readLine()
	if line == "" {
		return defaultVal
	}
	return line
}

// Confirm prints a Y/n or y/N prompt and returns true for yes.
// defaultYes controls which answer is the default (Enter with no input).
func (t *Term) Confirm(label string, defaultYes bool) bool {
	if defaultYes {
		fmt.Fprintf(t.out, "%s [Y/n]: ", label)
	} else {
		fmt.Fprintf(t.out, "%s [y/N]: ", label)
	}
	line, _ := t.readLine()
	if line == "" {
		return defaultYes
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

// Select prints a numbered list and returns the 0-based index of the choice.
// Re-prompts on invalid input. Returns 0 if options is empty.
func (t *Term) Select(label string, options []string) int {
	if len(options) == 0 {
		return 0
	}
	fmt.Fprintf(t.out, "%s%s%s\n", ansiBold, label, ansiReset)
	for i, opt := range options {
		fmt.Fprintf(t.out, "  %d. %s\n", i+1, opt)
	}
	for {
		fmt.Fprintf(t.out, "Choice [1-%d]: ", len(options))
		line, ok := t.readLine()
		if !ok {
			return 0
		}
		var n int
		if _, err := fmt.Sscanf(line, "%d", &n); err == nil {
			if n >= 1 && n <= len(options) {
				return n - 1
			}
		}
		fmt.Fprintf(t.out, "  Please enter a number between 1 and %d.\n", len(options))
	}
}

// MultiLine collects lines until the user submits an empty line.
// The label is shown once before input begins.
func (t *Term) MultiLine(label string) []string {
	fmt.Fprintf(t.out, "%s (one per line, empty line to finish):\n", label)
	var lines []string
	for {
		fmt.Fprintf(t.out, "  > ")
		line, ok := t.readLine()
		if !ok || line == "" {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

// Header prints a bold section header.
func (t *Term) Header(text string) {
	fmt.Fprintf(t.out, "\n%s%s%s\n", ansiBold, text, ansiReset)
}

// Success prints a green checkmark with a message.
func (t *Term) Success(msg string) {
	fmt.Fprintf(t.out, "%s%s %s%s\n", ansiGreen, "\u2713", msg, ansiReset)
}

// Info prints a plain informational line.
func (t *Term) Info(msg string) {
	fmt.Fprintln(t.out, msg)
}

// Println prints a line with a trailing newline.
func (t *Term) Println(msg string) {
	fmt.Fprintln(t.out, msg)
}
