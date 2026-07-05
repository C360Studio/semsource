# Tasks: Multi-language reference-ID parity (Java / TS / Go)

## 1. Shared: route reference construction through the definition path

- [x] 1.1 Confirm `ast.NewCodeEntity(org, lang, project, kind, name, relPath).ID`
  is the single source of truth every reference builder must call (D1). No new
  shared helper needed beyond `NewCodeEntity`; per-language resolvers supply
  `(kind, name, definingRelPath)`.

## 2. Java

- [x] 2.1 Fix superclass name extraction so the bound name is `Animal`, not
  `extends Animal` (drop the `extends` keyword / whitespace before resolution).
- [x] 2.2 Build a per-file `simpleName → FQN` map from `import a.b.C;` declarations
  (skip `import a.b.*;` wildcard). Add a `moduleToRelPath`-style resolver: FQN →
  `<root>/a/b/C.java`; unqualified name → same-package `<referrer pkg dir>/Name.java`.
- [x] 2.3 Rewrite `typeNameToEntityID` to resolve `extends`→`TypeClass`,
  `implements`→`TypeInterface`, interface-`extends`→`TypeInterface` via D2, build
  the ID against the resolved defining relPath through `NewCodeEntity` (D1); keep
  `external:`/`builtin:` for unresolved/FQN-stdlib. Pass the syntactic kind from
  the caller (extends vs implements clause).
- [x] 2.4 Same-file kind table (D5) so an unqualified field/return type that names
  a same-file class/interface/enum resolves to its real kind.
- [x] 2.5 Unit tests asserting **`ref-id == def-id`** (not substring):
  same-file `extends`/`implements`; cross-file `extends` via `import`;
  same-package (no import) `extends`; unresolved FQN stays `external:`.

## 3. TypeScript / JavaScript

- [x] 3.1 Build a per-file `localName → moduleSpecifier` map from
  `import { Base } from './base'`, `import Base from './base'`,
  `import Base as B from '...'` (skip bare/`node_modules` specifiers).
- [x] 3.2 Add a relative-specifier→file resolver: resolve `./base` / `../x` against
  the importer's dir, probing `.ts/.tsx/.js/.jsx/.mts/.cts/.mjs/.cjs` then
  `/index.*`; `ok=false` for bare specifiers.
- [x] 3.3 Rewrite `typeNameToEntityID` to build hierarchy edges via `NewCodeEntity`
  (D1) with kind from position (D2: class `extends`→class, `implements`→interface,
  interface `extends`→interface) against the resolved defining relPath. Same-file
  kind table (D5) for unknown-kind references.
- [x] 3.4 Unit tests asserting `ref-id == def-id`: same-file class `extends` +
  `implements`; interface `extends`; cross-file class `extends` via relative
  import; bare specifier (`react`) stays inert/builtin.

## 4. Go

- [x] 4.1 Add a memoized per-directory sibling scan (`go/parser`, top-level `type`
  decls only) yielding `name → (definingRelPath, kind struct|interface|type)` for
  the referrer's package directory; cache on the `Parser`, keyed by directory.
- [x] 4.2 Rewrite the local-type branch of `typeNameToEntityID` to resolve a bare
  name via the sibling scan and build through `NewCodeEntity` (D1) with the
  discovered kind + defining relPath; fix the raw-`project` bug (now via
  `NewCodeEntity`'s `SystemSlug`). Keep `pkg.Type`→`external:` and `builtin:`.
- [x] 4.3 Unit tests asserting `ref-id == def-id`: same-file embed of a local
  struct and of a local interface (correct kind segment); cross-file
  (same-package, different file) embed resolves to the sibling's ID; `pkg.Type`
  stays `external:`; raw-project (`semstreams@v1.9.0`-style) reference matches the
  `SystemSlug`'d definition.

## 5. Shared tests & race

- [x] 5.1 `-race` regression: drive one reused parser (per language that gained
  per-file/per-dir state) across two files concurrently under the ast-source lock
  pattern; assert no data race (guards D6).
- [x] 5.2 Confirm existing same-file tests still pass and that the substring-only
  assertions are upgraded to `ref-id == def-id` where they were masking the dangle.

## 6. Live validation (per language)

- [x] 6.1 Java: index a small multi-file Java repo through the compose stack;
  confirm a cross-file `extends`/`implements` resolves live — `code_context` shows
  `extended_by`/`implemented_by` and `code_impact` counts the cross-file dependent.
  Capture before/after.
- [x] 6.2 TS: index a small multi-module TS repo; confirm a cross-module class
  `extends` via relative import resolves live. Capture before/after.
- [x] 6.3 Go: index a multi-file Go package; confirm a same-package cross-file
  embed resolves live. Capture before/after.

## 7. Gates & docs

- [x] 7.1 `go build ./...`, `go vet`, `revive` (zero warnings), full `go test ./...`
  green; `go test -race ./source/ast/...`.
- [x] 7.2 Update in-code comments to reflect cross-file now resolves per language;
  note remaining non-goals (wildcard/star imports, re-exports, tsconfig paths,
  namespace packages, unknown-kind cross-file) where the resolver returns inert.
- [x] 7.3 `openspec validate --all`; then `/opsx:verify` before archiving.
