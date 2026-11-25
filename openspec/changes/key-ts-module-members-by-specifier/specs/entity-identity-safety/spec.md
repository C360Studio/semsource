## ADDED Requirements

### Requirement: TypeScript module-declaration members are keyed by their raw specifier
The TypeScript and Svelte parsers SHALL scope a member of a `declare module "<specifier>"` block by a segment derived from the raw specifier, and MUST NOT give it the ID of a dotted-namespace member or of a member of another specifier that differs only in `.`, `/` or `-`.

#### Scenario: A string module and a dotted namespace declare one name
- **WHEN** one file declares `declare module "foo.bar" { export function x() {} }`, `declare module "foo/bar" { export function x() {} }` and `namespace foo.bar { export function x() {} }`
- **THEN** the parse result carries three function entities named `x` with distinct IDs
- **AND** the `namespace foo.bar` member keeps the ID it had before this change
