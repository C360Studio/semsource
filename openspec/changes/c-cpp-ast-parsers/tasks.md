# Tasks

## 1. Make parser selection deterministic — before adding the languages that need it

- [x] 1.1 Replace the `for lang, parser := range pw.parsers` scan in
      `processor/ast-source/component.go:592` with resolution in a stable, defined order, applying
      D1's rule when two declared languages claim one extension. Routing moved to
      `processor/ast-source/routing.go` as a pure `routeExtensions(languages, extensionsFor)`
      function, resolved **once per watch path** at construction rather than per file, and stored on
      `pathWatcher.routes`. Taking `extensionsFor` as a parameter is what lets the rule be tested on
      language pairs the global registry does not contain yet.
- [x] 1.2 Test that a watch path declaring two languages sharing an extension routes that extension
      to the same parser **over many repetitions** — a single run passes by luck roughly half the
      time (design — D2). 500 repetitions. Also pinned that routing is a function of the language
      **set**, not its declaration order, since two configs listing the same languages differently
      must not produce different entity IDs.
- [x] 1.3 Verify the test by mutating the code: restore the map range and watch 1.2 fail. A
      determinism test that has never failed is not known to test determinism. Restored the map
      range and it **failed at iteration 2**: `routed .h to "c", first call routed it to "cpp"`.
- [x] 1.4 Confirm no existing language pair shares an extension today, so this lands as a pure
      no-op for every current configuration. Verified against the live registry — `go .go`,
      `java .java`, `python .py`, `svelte .svelte`, `typescript .ts/.tsx/.mts/.cts`,
      `javascript .js/.jsx/.mjs/.cjs`; **contested set is empty**. Consequence recorded honestly:
      the `TestEveryContestedExtensionHasAnExplicitRule` guard passes **vacuously** until C/C++
      land, so `TestContestedExtensions_DetectsOverlap` proves the detector separately rather than
      leaving a test that merely looks green.

## 2. Close the silent language defaults

- [x] 2.1 Make `langToDomain` (`handler/ast/mapper.go:137`) total — no `default:` to
      `DomainGolang`. Both lookups moved to `handler/ast/languages.go` as **data** (`languageDomains`,
      `languageExtensions`) rather than switch statements, and now return `(value, ok)`.
- [x] 2.2 Make `extensionsForLanguage` (`handler/ast/handler.go:214`) total — no `default:` to
      `.go`. (The function is named `langToExtensions`.) `Ingest` and `Watch` now call
      `validateLanguage` up front, so an unmappable language stops ingestion at the entry point
      instead of surfacing later; the two remaining `_`-discarded lookups leave the domain **empty**
      rather than `golang`, so even an unreachable path fails ID construction loudly.
- [x] 2.3 Test that a registered parser with no domain mapping is **detected**, driven off the
      registry rather than a hand-written list, so the next language added cannot land half-wired.
      `TestEveryRegisteredParserIsMapped` iterates `DefaultRegistry.ListParsers()`.
      **Mutation-verified:** removing java's entry fails it with
      *"parser \"java\" is registered but has no domain mapping"*. Also pinned the existing
      language→domain mappings, since those are baked into already-published entity IDs.
- [x] 2.4 Reject a declared language with no registered parser at configuration time, naming it.
      **Already implemented** — `WatchPathConfig.Validate` (`processor/ast-source/config.go:59`)
      rejects it and lists the registered parsers. Covered with a test rather than duplicated.

## 3. The C parser

- [ ] 3.1 `source/ast/c` following the `source/ast/java` shape: `init()` registration, tree-sitter
      walk, entity construction via `entityid.*`.
- [ ] 3.2 Extract functions, structs, unions, enums, typedefs, macro definitions, and file-scope
      variables.
- [ ] 3.3 Qualify entity IDs by defining-file path (design — D4), and test that two files defining
      a same-named `static` function produce two distinct IDs.
- [ ] 3.4 Test that re-ingesting an unchanged tree produces byte-identical IDs.

## 4. The C++ parser

- [ ] 4.1 `source/ast/cpp`, same shape, registered as its own language and domain.
- [ ] 4.2 Extract the C set plus classes, methods, constructors, destructors, namespaces, and
      templates.
- [ ] 4.3 Decide and pin the declaration-in-`.h` / definition-in-`.cpp` relationship: one entity or
      two related ones. Either is defensible; it must be deterministic and collision-free.
- [ ] 4.4 Test `.h` resolution both ways — a path declaring `cpp` reads headers as C++, a path
      declaring only `c` reads them as C (design — D1).

## 5. Wire the languages through every surface that enumerates them

- [ ] 5.1 Blank imports in `handler/ast/handler.go` and `processor/ast-source/config.go`.
- [ ] 5.2 `handler/ast/handler.go` extensions, `handler/ast/mapper.go` domains.
- [ ] 5.3 `processor/code-context/component.go` `codeScopeDomains` — otherwise symbols are
      extracted and then invisible to code-scoped retrieval.
- [ ] 5.4 `source/fusion/lens/code/code.go` extension set.
- [ ] 5.5 `source/ast/vocabulary.go` language description, `cli/wizard_ast.go` wizard options.
- [ ] 5.6 A test that walks the registry and asserts every registered language is present at each
      enumeration site — the generalization of 2.3, and the thing that makes 5.3-5.5 impossible to
      forget next time.

## 6. Measure on the real corpus, not a fixture

- [ ] 6.1 Ingest the **real** Meshtastic firmware tree (775 `.h`, 515 `.cpp`, 9 `.c`) and record
      entities extracted, parse failures, and wall-clock. Baseline to beat: **zero entities today.**
- [ ] 6.2 Ingest a real MAVLink tree — pure C, header-only, macro-dense — and record the same.
      This is the case that stresses both D1 (`.h` as C) and the no-preprocessor limit.
- [ ] 6.3 Quantify the preprocessor limit rather than asserting it: sample symbols reachable only
      through macro expansion and report roughly what share is missed, so the A/B is read with the
      right expectation.
- [ ] 6.4 Spot-check extraction correctness against the source by hand — a symbol count alone
      cannot distinguish 10,000 right entities from 10,000 wrong ones.
- [ ] 6.5 Record parse throughput; the AST source serializes per-file parsing behind `pw.parseMu`,
      and the C++ grammar is large.

## 7. Runtime acceptance

- [ ] 7.1 Boot a stack over a multi-repo corpus including Meshtastic and query through MCP:
      `code_search` for a known C++ symbol, `code_context` on it, `code_impact` from it.
- [ ] 7.2 Confirm the `{domain}` segment is `c` / `cpp` on live entities — the silent-default bug
      this change closes would show up here and nowhere else.
- [ ] 7.3 Re-run the docs/testing corpus profile so `docs/testing/tier-baselines.md`'s
      "1,299 files produce zero entities" line is replaced by a measured number.

## 8. Gates

- [ ] 8.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`,
      `go test -tags=integration ./...` green.
- [ ] 8.2 `openspec validate c-cpp-ast-parsers --strict` green.
- [ ] 8.3 The retrieval scorecard is unchanged — it runs on a Go/markdown corpus, so a moved score
      would mean this change altered something it should not have touched.

## 9. Not this change

- [ ] 9.1 Rust, C#, and other unparsed languages.
- [ ] 9.2 A preprocessor, include-path resolution, or build-system awareness.
- [ ] 9.3 C/C++ call-graph completeness beyond what the existing resolvers give.
- [ ] 9.4 Re-running the model A/B — downstream of this landing.
