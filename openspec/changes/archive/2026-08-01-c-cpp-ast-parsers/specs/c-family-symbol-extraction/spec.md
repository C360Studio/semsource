## ADDED Requirements

### Requirement: C and C++ sources contribute code symbols

C and C++ sources SHALL be parsed into code entities so that symbol-level retrieval covers them.
C SHALL yield at least functions, structs, unions, enums, typedefs, macro definitions, and
file-scope variables. C++ SHALL additionally yield classes, methods, constructors, destructors,
namespaces, and templates.

C and C++ SHALL be distinct languages with distinct `{domain}` segments, because domain is part of
entity identity and is what code-scoped retrieval filters on.

#### Scenario: A C translation unit is ingested

- **WHEN** a `.c` file defining functions and structs is ingested
- **THEN** an entity is produced for each, carrying the C domain

#### Scenario: A C++ translation unit is ingested

- **WHEN** a `.cpp` file defining a class with methods is ingested
- **THEN** the class and each method are produced as entities, carrying the C++ domain

#### Scenario: A C++ repository is retrievable by symbol

- **WHEN** a symbol defined in a C++ repository is queried through the code retrieval surface
- **THEN** the defining entity is returned, as it would be for any other supported language

### Requirement: A C-family symbol's identity survives the absence of a module system

C has no module or package namespace, so two files may each define a different symbol of the same
name. A symbol's entity ID SHALL therefore be qualified by the path of the file that defines it, so
that two distinct symbols never collide onto one entity ID.

Identity SHALL remain purely intrinsic — derived from path and symbol alone, never from parse
order, timestamps, or the order files are walked.

#### Scenario: Two files define a same-named static function

- **GIVEN** two source files in one repository each defining a file-local function of the same name
- **WHEN** both are ingested
- **THEN** they produce two distinct entity IDs, and neither overwrites the other

#### Scenario: The same repository is ingested twice

- **WHEN** an unchanged C or C++ repository is ingested twice
- **THEN** both passes produce byte-identical entity IDs

### Requirement: Extraction limits that follow from not preprocessing are stated

C and C++ sources SHALL be parsed as written, without `#include` expansion, macro expansion, or
conditional-compilation evaluation. Consequently a symbol reachable only through macro expansion or
an inactive preprocessor branch is not extracted.

This limit SHALL be documented rather than left to be discovered, because a macro-dense corpus can
otherwise appear to be fully indexed while a substantial share of its symbols is absent.

#### Scenario: A symbol is produced only by macro expansion

- **WHEN** a source file defines a symbol through a macro that would expand to a declaration
- **THEN** the symbol is not extracted, and this is a known limit rather than a defect

#### Scenario: A reference cannot be resolved without include knowledge

- **WHEN** a C or C++ reference cannot be resolved to a definition
- **THEN** no edge is emitted rather than an edge to a guessed target

### Requirement: C++ inheritance is resolved across the parsed set, never guessed

A C++ class's base classes SHALL be emitted as edges to the base's entity ID, so that
"what derives from this?" is answerable. Because C++ has no convention tying a type name to a
file — which header defines a class depends on what was included — resolution SHALL happen once
the whole watch path is parsed, over the complete set of definitions, rather than per file.

Resolution SHALL be a function of the definition set alone: the same tree MUST produce the same
edges regardless of the order files were parsed, because entity IDs are intrinsic and reproducible.

A base-class name matching more than one definition, or no definition, SHALL yield **no edge**. A
guessed edge would make impact analysis report a dependent that does not exist, which is worse than
reporting none: a missing edge is a known gap, a wrong one is a confident falsehood.

#### Scenario: A base class defined in another file

- **GIVEN** a class deriving from a base declared in a different file of the same watch path
- **WHEN** the watch path is parsed
- **THEN** the derived class carries an inheritance edge to the base's entity ID

#### Scenario: An ambiguous base-class name

- **GIVEN** two classes of the same name defined in different files
- **WHEN** a third class derives from that name
- **THEN** no inheritance edge is emitted for it

#### Scenario: A base class outside the corpus

- **GIVEN** a class deriving from a type no ingested file defines
- **WHEN** the watch path is parsed
- **THEN** no inheritance edge is emitted

#### Scenario: Resolution does not depend on parse order

- **WHEN** the same tree is parsed twice with files visited in different orders
- **THEN** the resulting inheritance edges are identical
