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

- [x] 3.1 `source/ast/c` following the `source/ast/java` shape: `init()` registration, tree-sitter
      walk, entity construction via `entityid.*`. Registered for `.c` and `.h`. Node shapes were
      confirmed by probing the real grammar rather than guessed.
- [x] 3.2 Extract functions, structs, unions, enums, typedefs, macro definitions, and file-scope
      variables. Plus three things the shape of C forced:
      **(a) function prototypes are extracted, not just definitions** — a header-only C library
      (MAVLink is exactly this) declares its whole API as prototypes and defines almost nothing, so
      skipping them would index such a repo as nearly empty;
      **(b) `typedef struct Tag {…} Alias;` yields both names** — both are usable in C source and
      they cannot collide because the entity type is its own ID segment;
      **(c) Doxygen comment forms** (`///`, `//!`, `/*! */`) are recognised, not just the shared
      helper's Javadoc `/** */`. Doc comments are embedded and ranked, so leaving the dominant C
      convention unparsed would have been a retrieval-quality loss. Unions record as `struct` —
      there is no union entity type, and a named record is the right shape for retrieval.
- [x] 3.3 Qualify entity IDs by defining-file path (design — D4), and test that two files defining
      a same-named `static` function produce two distinct IDs. **No new machinery was needed** —
      `ast.BuildScopedInstanceID` already prefixes the path slug, so D4 falls out of the existing
      helper. Verified rather than assumed, and **mutation-verified**: removing the path prefix
      collapses both onto `acme.semsource.c.proj.function.helper`.
- [x] 3.4 Test that re-ingesting an unchanged tree produces byte-identical IDs. Also covered: the
      six-part ID shape and that C claims domain `c` rather than borrowing another language's;
      file-entity containment; both `#include` spellings; anonymous specifiers skipped rather than
      emitted nameless; and a detached comment is **not** attributed to the next symbol (a real bug
      this test caught — adjacency was only checked inside the line-comment run).

## 4. The C++ parser

- [x] 4.1 `source/ast/cpp`, same shape, registered as its own language and domain (`cpp`), for
      `.cpp/.cc/.cxx/.hpp/.hh/.hxx` **and `.h`** — the overlap task 1's routing table exists to
      settle. Grammar shapes probed, not guessed.
- [x] 4.2 Extract the C set plus classes, methods, constructors, destructors, namespaces, and
      templates. Scope (namespace + class chain) is carried into identity via
      `ast.NewScopedCodeEntity`, so two `send()` methods on different classes — or two `Node`
      classes in different namespaces — stay distinct. **Mutation-verified**: dropping namespace
      scope collapses both onto `…class.ns-cpp-Node`. A destructor keeps its `~`, or it would
      collide with the constructor (also mutation-verified). Out-of-line `int Radio::send(...)` is
      recorded as a method of `Radio`, not a free function. `extern "C" { … }` is unwrapped rather
      than treated as a scope, which would have put a bogus segment in every wrapped symbol's ID.
      Namespaces are recorded as `type`, **not** `package`: the package entity type deliberately
      drops the name from its instance ID, so two namespaces in one file would collide on one
      identity.
- [x] 4.3 Decide and pin the declaration-in-`.h` / definition-in-`.cpp` relationship: one entity or
      two related ones. Either is defensible; it must be deterministic and collision-free.
      **Decided: two distinct entities**, because identity is qualified by the defining file's path
      (D4) and the two live in different files. Both are typed `method` and both carry the class
      scope, so they agree on everything except the file. Deterministic and collision-free, which is
      what the spec requires. Pinned by `TestHeaderDeclarationAndDefinitionStayDistinct`, whose
      failure message says to update the spec rather than the assertion if this ever becomes
      intentional. Relating the pair is future work, not a defect.
- [x] 4.4 Test `.h` resolution both ways — a path declaring `cpp` reads headers as C++, a path
      declaring only `c` reads them as C (design — D1). Covered by the routing tests added in task 1
      (`TestRouteExtensions_DeclaredSetDecidesTheHeader`), which now exercise the real registration
      rather than fakes once task 5 links both parsers in.

## 5. Wire the languages through every surface that enumerates them

- [x] 5.1 Blank imports in `handler/ast/handler.go` and `processor/ast-source/config.go`. Added
      **first, deliberately**, to watch the task-2 guard fire on the real thing rather than a
      synthetic mutation — it named both languages and both tables.
- [x] 5.2 `handler/ast/handler.go` extensions, `handler/ast/mapper.go` domains — now the
      `languages.go` tables: domains `c` and `cpp`, extensions `.c/.h` and
      `.cpp/.cc/.cxx/.hpp/.hh/.hxx/.h`.
- [x] 5.3 `processor/code-context/component.go` `codeScopeDomains` — otherwise symbols are
      extracted and then invisible to code-scoped retrieval. Two existing tests pinned the old
      domain list verbatim (one unit, one on-the-wire integration); both expectations were updated,
      which is the correct direction since the list is meant to be *every* code-language domain.
- [x] 5.4 `source/fusion/lens/code/code.go` extension set — all eight C/C++ extensions.
- [x] 5.5 `source/ast/vocabulary.go` language description, `cli/wizard_ast.go` wizard options.
- [x] 5.6 A test that walks the registry and asserts every registered language is present at each
      enumeration site — the generalization of 2.3, and the thing that makes 5.3-5.5 impossible to
      forget next time. `TestCodeScopeCoversEveryRegisteredLanguage`, mutation-verified.

**A real defect this task uncovered, which the vacuous guard had hidden.**
`ParserRegistry.Register` is first-extension-wins, and `GetExtensionsForParser` derived its answer
from that map — so once C and C++ both claimed `.h`, the registry reported that **C++ does not
handle `.h` at all** (C registered first). Consequences: the routing table never saw an overlap, so
D1's rule never fired; and a watch path declaring only `cpp` would have parsed **no headers** —
Meshtastic's 775 `.h` files, its main content. Nothing errored. The registry now records what each
parser *declared* separately from the first-wins lookup, with a regression test. Only after that fix
did `TestEveryContestedExtensionHasAnExplicitRule` stop being vacuous — re-verified by removing the
`.h` rule and watching it fail.

## 6. Measure on the real corpus, not a fixture

- [x] 6.1 Ingest the **real** Meshtastic firmware tree (775 `.h`, 515 `.cpp`, 9 `.c`) and record
      entities extracted, parse failures, and wall-clock. Baseline to beat: **zero entities today.**
      **1,291 files → 33,092 entities, 0 parse failures, 3.1 s.** By type: 14,090 const, 7,903
      method, 6,212 var, 2,251 function, 1,291 file, plus classes/structs/enums/types.
- [x] 6.2 Ingest a real MAVLink tree — pure C, header-only, macro-dense — and record the same.
      This is the case that stresses both D1 (`.h` as C) and the no-preprocessor limit.
      `mavlink/c_library_v2`: **483 `.h`, 0 `.c`** — exactly the header-only shape D1 was designed
      for. **12,522 entities, 0 parse failures, 2.0 s**, of which **7,049 functions** — nearly all
      prototypes, confirming that extracting them (3.2a) is what makes this repo indexable at all.
- [x] 6.3 Quantify the preprocessor limit rather than asserting it: sample symbols reachable only
      through macro expansion and report roughly what share is missed, so the A/B is read with the
      right expectation. Measuring it **changed the implementation**: 893 top-level `#ifdef`/`#if`
      blocks in MAVLink had their entire contents skipped, because only direct children of the root
      were visited. Both parsers now recurse into every branch — which is the right reading when no
      preprocessor runs, since which branch compiles is unknowable. Effect: MAVLink
      **10,185 → 12,522 (+23%)**, Meshtastic **12,582 → 33,092 (2.6×)**, firmware being heavily
      per-variant `#ifdef`-guarded.

      What remains unreachable, counted rather than asserted: **483 `preproc_call`** nodes in
      MAVLink and **647** in Meshtastic — file-scope macro invocations that may expand to
      declarations — plus **308 `ERROR`** nodes in Meshtastic where tree-sitter cannot parse
      macro-heavy constructs. These are the honest bound on C/C++ coverage.
- [x] 6.4 Spot-check extraction correctness against the source by hand — a symbol count alone
      cannot distinguish 10,000 right entities from 10,000 wrong ones. Checked
      `src/mesh/RadioInterface.h` line by line: typedef at L16, `#define MAX_TX_QUEUE` at L18,
      `class RadioInterface` at L75, `void deliverToReceiver(meshtastic_MeshPacket *p)` at L122,
      `~RadioInterface` at L129, and `bool canSleep(bool deepSleep = false)` at L149 — names, kinds,
      line numbers and signatures all match the source, including the default argument.
- [x] 6.5 Record parse throughput; the AST source serializes per-file parsing behind `pw.parseMu`,
      and the C++ grammar is large. **415 files/s C++ (1,291 files in 3.1 s)** and **236 files/s C
      (483 files in 2.0 s)**. Not a bottleneck beside embedding, which dominates ingest.

## 7. Runtime acceptance

- [x] 7.1 Boot a stack over a multi-repo corpus including Meshtastic and query through MCP:
      `code_search` for a known C++ symbol, `code_context` on it, `code_impact` from it. Booted over
      osh-core + Meshtastic + MAVLink + CS API — **57,158 live entities, 0 parse failures, 0 ERROR
      log lines**. All three tools answered over a real MCP session:
      `code_search` returned `…cpp.meshtastic-firmware.*` and `…c.mavlink-c.*` nodes;
      `code_context "RadioInterface"` returned the class plus **two constructor entities** — the
      overload discriminator visible live; `code_context "mavlink_msg_heartbeat_pack"` resolved the
      C function.

      **`code_impact` finds C/C++ symbols but reports 0 dependents**, which is the documented
      non-goal rather than a wiring failure. Proved with a control in the same stack: Java
      `IModuleProvider` → **9 dependents**, C++ `RadioInterface` → 0, C
      `mavlink_msg_heartbeat_pack` → 0. Anyone reading "C/C++ works end to end" needs that caveat.

      Also observed and worth keeping: before the gates opened, `code_search` **refused** rather
      than returning an empty list — `deferred: true, defer_reason: "hard_stop"` with the index
      state attached. A premature empty answer would have been indistinguishable from a genuine
      absence.
- [x] 7.2 Confirm the `{domain}` segment is `c` / `cpp` on live entities — the silent-default bug
      this change closes would show up here and nowhere else. Read straight from ENTITY_STATES:
      **`cpp` 32,312** (all `meshtastic-firmware`), **`c` 12,104** (all `mavlink-c`), `java` 8,539,
      `web` 3,300, `python` 343. **No C or C++ entity carries `golang`.**

      The same run verified **D1 both ways in one deployment**: Meshtastic declares `cpp` and its
      775 headers parsed as C++; MAVLink declares `c` and its 483 headers parsed as C.
- [x] 7.3 Re-run the docs/testing corpus profile so `docs/testing/tier-baselines.md`'s
      "1,299 files produce zero entities" line is replaced by a measured number. Added as a dated
      note rather than by rewriting the original: the clustering numbers there were measured on the
      corpus **as it stood**, and silently restating them would misrepresent when they were taken.
      The note records the new counts and says plainly that a C/C++-heavy graph makes the
      "one blob per repo" finding unmeasured again rather than carried over.

**Scale observation, not a blocker.** At 57,158 entities the structural index took ~16 minutes to
reach `ready`, and `indexed_revision` reported stale for most of it (frozen near 18,760 while the
NAME_INDEX bucket grew past 50,000), so readiness looked stalled when it was progressing. Worth
knowing before anyone runs the A/B on a corpus this size.

## 8. Gates

- [x] 8.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`,
      `go test -tags=integration ./...` green (integration run with `-race`).
- [x] 8.2 `openspec validate c-cpp-ast-parsers --strict` green.
- [ ] 8.3 The retrieval scorecard is unchanged — it runs on a Go/markdown corpus, so a moved score
      would mean this change altered something it should not have touched.

## 9. Not this change

- [ ] 9.1 Rust, C#, and other unparsed languages.
- [ ] 9.2 A preprocessor, include-path resolution, or build-system awareness.
- [x] 9.3 ~~C/C++ call-graph completeness beyond what the existing resolvers give.~~ **Narrowed
      mid-change after a challenge to the scoping.** Call edges remain out of scope — and are out
      for Java and TypeScript too, which is the bigger finding. But **C++ inheritance edges are now
      IN scope and implemented**: `cpp.ResolveTypeRefs` resolves base classes over the fully parsed
      watch path, **350 of 393 (89%)** on Meshtastic, dropping ambiguous and out-of-corpus names
      rather than guessing. The original non-goal was a rationalisation — `Extends` was already
      extracted and only the ID construction was missing.
      Landscape recorded in `docs/design/code-edges-by-language.md`.
- [ ] 9.4 Re-running the model A/B — downstream of this landing.
