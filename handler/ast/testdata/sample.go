// Package testpkg is a sample Go package for ASTHandler testing.
package testpkg

import "fmt"

// Greeter greets things.
type Greeter interface {
	Greet(name string) string
}

// ConsoleGreeter implements Greeter using standard output.
type ConsoleGreeter struct {
	Prefix string
}

// Greet returns a greeting string.
func (g *ConsoleGreeter) Greet(name string) string {
	return fmt.Sprintf("%s %s", g.Prefix, name)
}

// NewConsoleGreeter constructs a ConsoleGreeter with the given prefix.
func NewConsoleGreeter(prefix string) *ConsoleGreeter {
	return &ConsoleGreeter{Prefix: prefix}
}
