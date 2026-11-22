# code-call-graph

## ADDED Requirements

### Requirement: Java calls to overloaded methods resolve to one overload or none
A Java call SHALL resolve to the unique overload, collected across the receiver's in-repo class hierarchy with the nearest declaration winning for an identical signature, whose parameter count accepts the call's argument count. The arity check MUST apply even when only one candidate exists, and when no candidate or more than one qualifies, or the hierarchy reaches a type outside the repository, the call MUST emit no edge.

#### Scenario: Arity selects the overload
- **WHEN** a class declares `read(InputStream)` and `read(InputStream, int)`, and a method calls `read(in, 4)`
- **THEN** the caller has exactly one `calls` edge, to the two-parameter overload's entity ID

#### Scenario: Same-arity overloads emit no edge
- **WHEN** a class declares `visit(Text)` and `visit(Count)`, and a method calls `visit(x)`
- **THEN** the caller has no `calls` edge to either `visit` overload

#### Scenario: An overload inherited from a superclass is selected by arity
- **WHEN** `Base` declares `foo(int, int)`, its subclass `Sub` declares `foo(int)`, and a `Sub` method calls `foo(1, 2)`
- **THEN** the caller has exactly one `calls` edge, to `Base`'s `foo(int, int)` entity ID

#### Scenario: A single candidate with the wrong arity emits no edge
- **WHEN** a class declares only `add(Item)` and its in-repo code calls `add(0, item)` on an instance whose external superclass declares `add(int, Item)`
- **THEN** the caller has no `calls` edge to `add(Item)`

#### Scenario: Constructor overloads follow the same rule
- **WHEN** a class declares constructors `OWSException(String)` and `OWSException(String, Throwable)`, and code calls `new OWSException(msg, e)`
- **THEN** the caller has one `calls` edge, to the two-parameter constructor's entity ID
