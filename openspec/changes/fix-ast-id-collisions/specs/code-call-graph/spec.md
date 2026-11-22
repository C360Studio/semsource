# code-call-graph

## ADDED Requirements

### Requirement: Java calls to overloaded methods resolve to one overload or none
A Java call whose callee name has several overloads in the declaring class SHALL resolve to the unique overload whose parameter count accepts the call's argument count. When no overload or more than one qualifies, the call MUST emit no edge.

#### Scenario: Arity selects the overload
- **WHEN** a class declares `read(InputStream)` and `read(InputStream, int)`, and a method calls `read(in, 4)`
- **THEN** the caller has exactly one `calls` edge, to the two-parameter overload's entity ID

#### Scenario: Same-arity overloads emit no edge
- **WHEN** a class declares `visit(Text)` and `visit(Count)`, and a method calls `visit(x)`
- **THEN** the caller has no `calls` edge to either `visit` overload

#### Scenario: Constructor overloads follow the same rule
- **WHEN** a class declares constructors `OWSException(String)` and `OWSException(String, Throwable)`, and code calls `new OWSException(msg, e)`
- **THEN** the caller has one `calls` edge, to the two-parameter constructor's entity ID
