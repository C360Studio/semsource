# Key TypeScript module-declaration members by their raw specifier — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` (2026-11-25, sha256:1d6ea997c90c); line entered by claude at the owner's instruction.

## 2. Repro (fails on main)

- [x] 2.1 One file with `declare module "foo.bar"`, `declare module "foo/bar"` and `namespace foo.bar`, each exporting `x`: today the parse result carries one ID for all three `x`.

## 3. Encoding

- [x] 3.1 `NamespaceSegments` returns `entityid.SanitizeSegment("module:" + specifier)` for a string module name.
- [x] 3.2 Calls to `x` inside each block resolve to that block's `x`; dotted-namespace and module-level IDs unchanged. `TestAmbientTypeReferencesResolveToTheirDeclarations` now pins the `declare module "m"` scope as the literal `module-m-4c89ba36`.

## 4. Gates

- [x] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./source/ast/...`, `task spec:check`.
- [x] 4.2 `graph-event-reviewer` pass (approve; Svelte case added from its findings); review via `ateam review request`; archive before ready.
