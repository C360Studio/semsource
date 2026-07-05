# Tasks: Python cross-file reference resolution

## 1. Import binding extraction

- [ ] 1.1 Add an import parser in `source/ast/python/` that walks a file's
  `import_statement` / `import_from_statement` nodes into a
  `map[localName]importBinding{module, originName, relativeLevel}`, covering
  `import m`, `import m as a`, `from m import N`, `from m import N as A`, and
  relative imports (`from . import x`, `from ..p import y`).
- [ ] 1.2 Unit-test the import parser against each binding form (including aliases
  and multi-level relative imports); assert the produced bindings.

## 2. Module → file resolver

- [ ] 2.1 Add a `moduleToRelPath(module string, fromFile string, level int) (relPath string, ok bool)`
  resolver rooted at the parser's `repoRoot`: probe `<root>/a/b/c.py` then
  `<root>/a/b/c/__init__.py`; resolve relative imports against `fromFile`'s package
  dir walking up `level` parents. Return `ok=false` for out-of-tree modules.
- [ ] 2.2 Unit-test the resolver: absolute in-tree module → `.py` and `__init__.py`
  layouts, relative imports at levels 1 and 2, and an out-of-tree module → `ok=false`.

## 3. Wire resolution into reference-id construction

- [ ] 3.1 Thread the file's import bindings into `typeNameToEntityID` (or a small
  resolver struct built per file). When the bare `typeName` is an imported name that
  resolves in-tree, build the target ID via
  `ast.NewCodeEntity(org,"python",project,ast.TypeClass,originName,<defining relPath>).ID`
  (D4 — same construction path as the definition). Otherwise keep today's same-file
  behavior; leave third-party/unresolved as `external:`/inert.
- [ ] 3.2 Ensure the defining-file `relPath` is normalized exactly as `ParseFile`
  normalizes a parsed file's path (relative to `repoRoot`), so ref-id == def-id.

## 4. Tests

- [ ] 4.1 Parser regression: parse two files together (`pkg/base.py` defines
  `BaseClient`, `pkg/client.py` does `from pkg.base import BaseClient` +
  `class AsyncClient(BaseClient)`) and assert `AsyncClient.Extends[0] ==
  BaseClient.ID` (real cross-file parity, not substring).
- [ ] 4.2 Negative tests: third-party import stays `external:`/inert; a star-import
  reference is not resolved to a wrong entity; an unresolvable relative import is inert.
- [ ] 4.3 Confirm same-file resolution (task #43) is unchanged (existing
  `TestParseFile_ClassInheritance` still passes).

## 5. Live validation

- [ ] 5.1 Index a real multi-module Python repo (httpx) via the compose stack;
  confirm a cross-module `extends` (a subclass importing its base from another file)
  resolves live — `code_context` shows `extended_by` and `code_impact` counts the
  cross-file dependent. Capture before/after.

## 6. Gates & docs

- [ ] 6.1 `go build ./...`, `go vet`, `revive` (zero warnings), full `go test ./...` green.
- [ ] 6.2 Update in-code comments to reflect cross-file now resolves; note remaining
  non-goals (star-import, re-export, namespace packages) where the resolver returns inert.
- [ ] 6.3 `openspec validate --all`; then `/opsx:verify` before archiving.
