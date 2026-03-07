package ast

import (
	"fmt"
	"sync"
)

// ParserFactory creates a FileParser for a specific language.
// The factory receives org, project, and repoRoot to configure entity ID generation.
type ParserFactory func(org, project, repoRoot string) FileParser

// ParserRegistry maintains a registry of language parsers.
// Parsers are registered by name with their supported file extensions.
// Thread-safe for concurrent access.
type ParserRegistry struct {
	mu      sync.RWMutex
	parsers map[string]ParserFactory // name → factory
	extMap  map[string]string        // extension → parser name
}

// NewParserRegistry creates a new empty parser registry.
func NewParserRegistry() *ParserRegistry {
	return &ParserRegistry{
		parsers: make(map[string]ParserFactory),
		extMap:  make(map[string]string),
	}
}

// Register adds a parser factory for the given extensions.
// The first registration wins if there's an extension conflict.
// Extensions should include the leading dot (e.g., ".go", ".ts").
func (r *ParserRegistry) Register(name string, extensions []string, factory ParserFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Register the factory
	r.parsers[name] = factory

	// Map extensions to this parser (first registration wins)
	for _, ext := range extensions {
		if _, exists := r.extMap[ext]; !exists {
			r.extMap[ext] = name
		}
	}
}

// GetParserName returns the parser name registered for a file extension.
// Returns empty string and false if no parser is registered for the extension.
func (r *ParserRegistry) GetParserName(ext string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name, ok := r.extMap[ext]
	return name, ok
}

// CreateParser instantiates a parser by name with the given configuration.
// Returns an error if the parser name is not registered.
func (r *ParserRegistry) CreateParser(name, org, project, repoRoot string) (FileParser, error) {
	r.mu.RLock()
	factory, ok := r.parsers[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("parser not registered: %s", name)
	}

	return factory(org, project, repoRoot), nil
}

// CreateParserForExtension creates a parser for the given file extension.
// Returns an error if no parser is registered for the extension.
func (r *ParserRegistry) CreateParserForExtension(ext, org, project, repoRoot string) (FileParser, error) {
	name, ok := r.GetParserName(ext)
	if !ok {
		return nil, fmt.Errorf("no parser registered for extension: %s", ext)
	}
	return r.CreateParser(name, org, project, repoRoot)
}

// ListParsers returns all registered parser names.
func (r *ParserRegistry) ListParsers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.parsers))
	for name := range r.parsers {
		names = append(names, name)
	}
	return names
}

// ListExtensions returns all registered file extensions.
func (r *ParserRegistry) ListExtensions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extensions := make([]string, 0, len(r.extMap))
	for ext := range r.extMap {
		extensions = append(extensions, ext)
	}
	return extensions
}

// GetExtensionsForParser returns all extensions mapped to a parser name.
func (r *ParserRegistry) GetExtensionsForParser(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var extensions []string
	for ext, parserName := range r.extMap {
		if parserName == name {
			extensions = append(extensions, ext)
		}
	}
	return extensions
}

// HasParser returns true if a parser with the given name is registered.
func (r *ParserRegistry) HasParser(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.parsers[name]
	return ok
}

// DefaultRegistry is the global parser registry.
// Language parsers register themselves via init() functions.
var DefaultRegistry = NewParserRegistry()
