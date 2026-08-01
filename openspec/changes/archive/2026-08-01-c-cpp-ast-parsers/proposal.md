## Why

SemSource parses Go, TypeScript/JavaScript, Java, Python, and Svelte. It does not parse C or C++,
so a C/C++ repository contributes **no code symbols at all** — not fewer symbols, none. Measured on
the corpus the planned agent A/B actually uses: Meshtastic firmware's **1,299 `.c` / `.cpp` / `.h`
files produce zero entities**, and the repo reaches the graph only through 40 incidental Python
files, 37 markdown files, and config
([`docs/testing/tier-baselines.md`](../../../docs/testing/tier-baselines.md)).

That is the gap directly under the A/B prompts — *"create a Meshtastic driver using OSH's CS API"*
and *"create a MAVLink driver for OSH using CS API"*. Both ask an agent to write a driver against a
codebase it cannot read symbol-level. No amount of clustering, ranking, or retrieval tuning
substitutes: `code_search`, `code_context`, and `code_impact` have nothing to return because
nothing was ever extracted. The two changes that shipped today (AsciiDoc heading identity, clustering
edge synthesis) both improved how well we retrieve what we *have*; this one is about the half of the
A/B corpus we have nothing for.

Now, because the cost turns out to be low: the tree-sitter **`c` and `cpp` grammars are already
vendored** in `github.com/smacker/go-tree-sitter`, an existing direct dependency, each exposing the
same `GetLanguage()` the Java and Python parsers already use. No new dependency, no new toolchain.

## What Changes

- **Add a C parser** (`source/ast/c`) extracting functions, structs, unions, enums, typedefs,
  macros, and file-scope variables.
- **Add a C++ parser** (`source/ast/cpp`) extracting the above plus classes, methods, constructors
  and destructors, namespaces, and templates.
- **Make file→parser resolution deterministic and configuration-driven.** Today
  `parseFileWithWatcher` (`processor/ast-source/component.go:592`) selects a parser with
  `for lang, parser := range pw.parsers` — Go map iteration order, i.e. **random**. It has never
  misbehaved only because every registered language owns a **disjoint** extension set
  (`typescript` = `.ts/.tsx/.mts/.cts`, `javascript` = `.js/.jsx/.mjs/.cjs`). C and C++ are the
  first pair that must share one — `.h` — so this becomes a correctness requirement rather than a
  latent tidiness issue: the same header could be parsed as C on one run and C++ on the next,
  producing different entities and a different `{domain}` segment for the same file. Entity IDs are
  required to be purely intrinsic, so nondeterministic parser choice is an identity defect.
- **Resolve `.h` from the watch path's declared language set**, never by sniffing file contents.
  This follows the precedent set by the AsciiDoc heading work: format comes from the caller, because
  a byte-sniffer that is wrong converts a working corpus into a broken one. The rule must be a total
  function of the declared set (design decides the exact rule).
- **Make the two silent language defaults total.** `langToDomain` (`handler/ast/mapper.go:137`) and
  `extensionsForLanguage` (`handler/ast/handler.go:214`) both `default:` to **golang / `.go`** for an
  unrecognized language. A half-wired C parser therefore does not fail — it publishes C symbols under
  **`{domain}` = `golang`**, with a green test suite. Any new language must be unable to land
  half-wired.
- **Wire the languages through the surfaces that enumerate them** so the new domains are visible to
  retrieval rather than parsed and then dropped: `processor/code-context` scope domains, the code
  fusion lens extension set, the AST vocabulary description, and the CLI wizard.

## Non-goals

- **Rust, C#, and any other unparsed language.** Same shape of work, no evidence they are on a
  critical path; adding them here would dilute the measurement that justifies this one.
- **A preprocessor.** `#include` expansion, macro expansion, and conditional compilation are not
  performed. A header is parsed as written. This bounds what call- and reference-edge resolution can
  honestly claim for C/C++ and must be stated rather than discovered later.
- **C/C++ call-graph edges.** Resolving a call site needs to know what the identifier refers to,
  which needs macro expansion and include paths — neither of which this change builds. Note this is
  not a C/C++ peculiarity: **Java and TypeScript emit no call edges either**, so `code_impact`
  answers from type hierarchy alone for four of seven languages. Tracked as the top item in
  [`docs/design/code-edges-by-language.md`](../../../docs/design/code-edges-by-language.md).

  **Corrected mid-change.** This non-goal originally covered *all* C/C++ edges, including
  inheritance, justified by the include-path argument. That was too wide: `Extends` was already
  being extracted, every other language resolves hierarchy edges, and C++ class names turn out to be
  nearly unique in practice (3% collide), so an index over the parsed set resolves 89% without
  guessing. Inheritance edges are therefore **in scope and implemented**; only call edges remain out.

- **C/C++ `References`** — field, parameter, and return type usage. Java and Go answer
  "what uses this type?"; C++ does not yet. Recorded as debt rather than silently absent.
- **Build-system awareness** (CMake, PlatformIO, Make). Not needed to extract symbols.
- **Changing the registry's public shape** beyond what determinism requires.
- **Re-running the retrieval scorecard or the model A/B.** Both are downstream of this landing.

## Capabilities

### New Capabilities

- `language-parser-resolution`: which parser reads a given file, and the guarantee that the answer
  is deterministic, derived from declared configuration rather than file contents or map order, and
  total — an unrecognized language fails loudly instead of silently becoming Go.
- `c-family-symbol-extraction`: what C and C++ sources contribute to the graph — the symbol kinds
  extracted, the identity of a symbol in a language with no module system, and the explicit limits
  that follow from not running a preprocessor.

### Modified Capabilities

- `ast-source-configuration`: `languages` accepts `c` and `cpp`, and a watch path that declares a
  language with no registered parser is rejected at configuration time rather than yielding an
  empty parse.

## Impact

- **New:** `source/ast/c/`, `source/ast/cpp/` (parser + tests), following the existing
  `source/ast/java` shape.
- **Modified:** `processor/ast-source/component.go` (deterministic parser selection),
  `handler/ast/handler.go` and `handler/ast/mapper.go` (extension and domain mapping, both made
  total), `processor/ast-source/config.go` and `handler/ast/handler.go` blank imports,
  `processor/code-context/component.go` (`codeScopeDomains`), `source/fusion/lens/code/code.go`,
  `source/ast/vocabulary.go`, `cli/wizard_ast.go`.
- **Dependencies:** none added — `github.com/smacker/go-tree-sitter` already vendors `c` and `cpp`.
- **Entity IDs:** two new `{domain}` values (`c`, `cpp`). No change to the six-part shape or to
  `entityid.*` construction.
- **No change** to the MCP surface, query contracts, GraphQL, or any tier configuration.

## Consumers

Any agent using `code_search`, `code_context`, or `code_impact` against a C/C++ repository —
today that means the planned semdev A/B, and SemSpec/SemDragon by extension, since they consume the
same MCP surface. There is no new API: the tools work exactly as they do for Java, over a corpus
they currently cannot see.
