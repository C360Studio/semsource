# Tasks: Python call graph

## 1. Local-function pre-scan

- [x] 1.1 Add a per-`Parser` set of the file's module-level function names,
  populated at `ParseFile` start (reset each file), alongside the #44 import map.

## 2. Call-site extraction

- [x] 2.1 Add `extractCalls(body, content, filePath, scope)` that recursively walks
  a function body for `call` nodes, resolves each callee to an entity ID, dedupes
  (a `seen` set), and returns the list.
- [x] 2.2 Wire it into `extractFunction`: set `entity.Calls = p.extractCalls(body, …)`
  for both functions and methods (pass the method's class `scope`).

## 3. Callee resolution

- [x] 3.1 Bare `identifier` call (D2): local module-level function → function ID
  via `NewCodeEntity`; else import binding (`lookupBinding`) → module file
  (`moduleToRelPath`) function ID or `external:`; else inert (emit nothing).
- [x] 3.2 `attribute` call (D3/D4): `self`/`cls` receiver + method scope →
  `NewScopedCodeEntity(TypeMethod, scope, method, filePath).ID`; `module.func`
  via an import binding → function ID against the module file or `external:`;
  any other receiver → inert.

## 4. Tests

- [x] 4.1 Unit: a function calling a **same-file** module-level function — assert
  `caller.Calls` contains the callee's own entity ID (`ref-id == def-id`, not
  substring).
- [x] 4.2 Unit: cross-file — `from pkg.util import helper` then `helper()` — assert
  the call target equals `helper`'s entity ID built against `pkg/util.py`.
- [x] 4.3 Unit: `self.method()` inside a class — assert the call target equals the
  sibling method's scoped entity ID.
- [x] 4.4 Unit: `module.func()` for an out-of-tree module stays `external:`; a
  builtin (`len()`) and a bare undefined name emit **no** call edge (inert).
- [x] 4.5 Confirm existing Python parser tests still pass (no regression to
  entities/extends/params).

## 5. Live validation

- [x] 5.1 Index a small multi-file Python package through the graph stack; confirm a
  cross-file call resolves live — `code_context` shows the `callee`/`caller`
  relation and `code_impact` follows it. (Extend the in-process governance
  integration harness; assert the caller appears under the callee's `caller` role.)

## 6. Gates & docs

- [x] 6.1 `go build ./...`, `go vet`, `revive` (zero warnings), full `go test ./...`
  green; `go test -race ./source/ast/...` and the ast-source concurrency guard.
- [x] 6.2 In-code comments document what resolves and the inert non-goals
  (builtins, instantiations, `obj.method` on locals, star/dynamic).
- [x] 6.3 `openspec validate --all`; then `/opsx:verify` before archiving.
