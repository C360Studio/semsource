# Key TypeScript module-declaration members by their raw specifier — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval.

## 2. Repro (fails on main)

- [ ] 2.1 One file with `declare module "foo.bar"`, `declare module "foo/bar"` and `namespace foo.bar`, each exporting `x`: today the parse result carries one ID for all three `x`.

## 3. Encoding

- [ ] 3.1 `NamespaceSegments` returns `entityid.SanitizeSegment("module:" + specifier)` for a string module name.
- [ ] 3.2 Calls to `x` inside each block resolve to that block's `x`; dotted-namespace and module-level IDs unchanged.

## 4. Gates

- [ ] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./source/ast/...`, `task spec:check`.
- [ ] 4.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
