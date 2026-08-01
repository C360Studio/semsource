## ADDED Requirements

### Requirement: Declared languages are validated against registered parsers

A watch path's `languages` SHALL be validated at configuration time against the set of registered
parsers. A declared language with no parser MUST fail configuration rather than produce a watch path
that walks files and extracts nothing.

`c` and `cpp` are valid declared languages.

#### Scenario: A watch path declares an unregistered language

- **WHEN** component configuration declares a language with no registered parser
- **THEN** configuration fails and names the language

#### Scenario: A watch path declares C or C++

- **WHEN** a watch path declares `c` or `cpp`
- **THEN** configuration succeeds and the matching source files are parsed
