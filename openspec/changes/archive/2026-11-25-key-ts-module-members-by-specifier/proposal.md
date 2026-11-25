# Key TypeScript module-declaration members by their raw specifier

Approved-by: Coby Leuschke (2026-11-25) sha256:1d6ea997c90c

Track: standard (one encoding change in `source/ast/ts`; identity-breaking for members of `declare module "…"` blocks
only, which rides the beta.11 reseed).

## Why

Since #337 a TS member is scoped by its namespace chain, and `NamespaceSegments` (`source/ast/ts/parser.go:499`) keeps
a string module name as one segment. IDs still pass that segment through `SanitizePathSegment`
(`source/ast/entities.go:147`), which maps `.` and `/` to `-`. So in one file `declare module "foo.bar" { export
function x() {} }` and `namespace foo.bar { export function x() {} }` both mint `<path>-foo-bar-x`, as do the string
modules `"foo.bar"`, `"foo/bar"` and `"foo-bar"` among themselves (#340). Call and type resolution already keeps them
apart (chains are keyed with `\x00`); only the IDs collide. The #333 guard keeps the first declaration and counts the
rest on `id_collisions`, so the later members are silently missing from the graph.

## What Changes

- `NamespaceSegments` returns a string module's segment as `entityid.SanitizeSegment("module:" + specifier)`: the `:`
  is never valid, so the segment always carries a hash of the raw specifier (`module-foo-bar-<hash8>`). It is already
  ID-valid, so `SanitizePathSegment` passes it through unchanged. Resolution chains use the same segment, so call and
  type resolution are unchanged.
- Members of dotted namespaces (`namespace foo.bar`) and of `declare global` keep their IDs.
- Tests: a file with `declare module "foo.bar"`, `declare module "foo/bar"` and `namespace foo.bar`, each exporting
  `x`, yields three distinct IDs; a call to `x` inside each block still resolves to its own block's `x`.

## Decisions you are making by approving

- **Members of `declare module "…"` blocks get new IDs**, which is a clean storage break with no aliasing. It rides
  the beta.11 reseed, which #348 already requires. Ambient module declarations are mostly in `.d.ts` files, so few
  entities change.
- **The segment is readable plus a hash** (`module-foo-bar-<hash8>`), not the bare specifier. That way, specifiers
  that differ only in `.`, `/` or `-` never collide.

## Out of scope

- Other `SanitizePathSegment` collisions (for example a class `foo` with a nested `bar` against `namespace foo.bar`).
  Nothing has reported them.
