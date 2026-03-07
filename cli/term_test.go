package cli

import (
	"bytes"
	"strings"
	"testing"
)

func newTestTerm(input string) (*Term, *bytes.Buffer) {
	out := &bytes.Buffer{}
	term := NewTerm(strings.NewReader(input), out)
	return term, out
}

func TestPromptWithDefault(t *testing.T) {
	term, _ := newTestTerm("\n") // user presses Enter
	got := term.Prompt("Name", "alice")
	if got != "alice" {
		t.Fatalf("expected default 'alice', got %q", got)
	}
}

func TestPromptWithInput(t *testing.T) {
	term, _ := newTestTerm("bob\n")
	got := term.Prompt("Name", "alice")
	if got != "bob" {
		t.Fatalf("expected 'bob', got %q", got)
	}
}

func TestPromptNoDefault(t *testing.T) {
	term, _ := newTestTerm("myorg\n")
	got := term.Prompt("Org", "")
	if got != "myorg" {
		t.Fatalf("expected 'myorg', got %q", got)
	}
}

func TestConfirmDefaultYes(t *testing.T) {
	term, _ := newTestTerm("\n") // Enter = yes
	if !term.Confirm("Continue?", true) {
		t.Fatal("expected true for default yes")
	}
}

func TestConfirmDefaultNo(t *testing.T) {
	term, _ := newTestTerm("\n") // Enter = no
	if term.Confirm("Continue?", false) {
		t.Fatal("expected false for default no")
	}
}

func TestConfirmExplicitNo(t *testing.T) {
	term, _ := newTestTerm("n\n")
	if term.Confirm("Continue?", true) {
		t.Fatal("expected false for explicit 'n'")
	}
}

func TestConfirmExplicitYes(t *testing.T) {
	term, _ := newTestTerm("y\n")
	if !term.Confirm("Continue?", false) {
		t.Fatal("expected true for explicit 'y'")
	}
}

func TestSelectValid(t *testing.T) {
	term, _ := newTestTerm("2\n")
	idx := term.Select("Pick one", []string{"alpha", "beta", "gamma"})
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
}

func TestSelectInvalidThenValid(t *testing.T) {
	// First input is out of range, second is valid.
	term, _ := newTestTerm("99\n1\n")
	idx := term.Select("Pick one", []string{"alpha", "beta"})
	if idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}
}

func TestSelectEmpty(t *testing.T) {
	term, _ := newTestTerm("")
	idx := term.Select("Pick one", nil)
	if idx != 0 {
		t.Fatalf("expected 0 for empty options, got %d", idx)
	}
}

func TestSelectEOFReturnsZero(t *testing.T) {
	term, _ := newTestTerm("") // EOF immediately
	idx := term.Select("Pick one", []string{"alpha", "beta"})
	if idx != 0 {
		t.Fatalf("expected 0 on EOF, got %d", idx)
	}
}

func TestMultiLine(t *testing.T) {
	term, _ := newTestTerm("docs/\nREADME.md\n\n")
	lines := term.MultiLine("Paths")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "docs/" || lines[1] != "README.md" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestMultiLineEmpty(t *testing.T) {
	term, _ := newTestTerm("\n")
	lines := term.MultiLine("Paths")
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}

func TestPromptOutputContainsLabel(t *testing.T) {
	term, out := newTestTerm("x\n")
	term.Prompt("My Label", "")
	if !strings.Contains(out.String(), "My Label") {
		t.Fatalf("expected label in output, got: %q", out.String())
	}
}

func TestPromptOutputShowsDefault(t *testing.T) {
	term, out := newTestTerm("\n")
	term.Prompt("Label", "mydefault")
	if !strings.Contains(out.String(), "mydefault") {
		t.Fatalf("expected default in output, got: %q", out.String())
	}
}
