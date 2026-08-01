## Context

See proposal.md — Why. Three facts about the existing code shape the approach:

1. **The ingest path selects a parser by declared language, not by extension.** Production calls
   `DefaultRegistry.CreateParser(lang, …)` (`handler/ast/handler.go:67,95`,
   `processor/ast-source/component.go:194`); `CreateParserForExtension` exists but is not on the
   ingest path. Language already comes from `watch_paths[].languages`, so the architecture is
   already "declared by the caller" — the same principle the AsciiDoc heading work established for
   document format.
2. **Extension sets are disjoint today, and the code silently depends on that.**
   `parseFileWithWatcher` picks the parser by ranging a `map[lang]FileParser` and comparing the
   file's extension against each language's registered extensions. Map iteration order is random in
   Go; it is invisible today only because no two languages claim the same extension.
3. **Both language switches fall through to Go.** `langToDomain` and `extensionsForLanguage` end in
   `default:` returning golang / `.go`.

The corpus this is for:

| repo | shape | why it matters here |
| --- | --- | --- |
| Meshtastic firmware | 775 `.h`, 515 `.cpp`, 9 `.c`; **175 of the first 200 `.h` contain `class`/`namespace`/`template`** | `.h` is C++ here |
| MAVLink | pure C, **header-only** (generated `.h`) | `.h` is C here |

So `.h` is ambiguous *across repositories*, not within one.

## Goals / Non-Goals

**Goals:**

- Two parsers following the existing `source/ast/java` shape, so review is pattern-matching.
- A file→parser rule that is a **total, deterministic function** of (declared language set, file
  extension) — reproducible across processes and runs, as entity identity requires.
- Failure modes that are loud: a language with no parser, or a new language wired only halfway,
  must fail rather than mislabel.

**Non-Goals (design-level, beyond the proposal's):**

- No change to `ParserRegistry`'s registration API. The determinism fix belongs at the selection
  site, not in how parsers announce themselves.
- No attempt to make C/C++ reference resolution as complete as Go's. Without include-path
  knowledge, the honest position is the existing "never produce a wrong edge" rule.

## Decisions

### D1 — `.h` is claimed by the declared language set, C++ winning when both are declared

The rule: within a watch path, if `languages` contains `cpp`, `.h` goes to the C++ parser; if it
contains `c` and not `cpp`, `.h` goes to the C parser.

*Why:* it resolves both real repositories correctly — Meshtastic declares `cpp` and its 775 headers
parse as C++; MAVLink declares `c` and its headers parse as C. It is a total function of declared
configuration, so it is deterministic and reproducible. And it puts the choice where the operator
already expresses intent.

*Alternatives considered:*

- **Sniff the file for `class`/`namespace`/`template`.** Rejected on precedent: the AsciiDoc work
  established that guessing a format from bytes can convert a working corpus into a broken one, and
  here a header that merely *uses* a C++ type without declaring one would sniff as C. The rule must
  not depend on content.
- **Always route `.h` to C++** (tree-sitter's C++ grammar is a superset of C, so C headers do
  parse). Rejected because it labels every MAVLink symbol `{domain}` = `cpp`, and domain is part of
  the entity ID — a permanent, wrong identity for an entire repository, in exchange for avoiding one
  config lookup.
- **A per-watch-path `header_language` setting.** Rejected as a knob that adds a way to be
  inconsistent with `languages`; the declared set already carries the information.

*Consequence to state in the spec:* a watch path declaring **both** `c` and `cpp` gets C++ for `.h`,
and C-only headers in such a repo are read by the C++ grammar. That is a deliberate, documented
resolution, not an accident — and because C++ is a superset, it is a labeling difference rather than
a parse failure.

### D2 — Selection becomes deterministic at the call site, by explicit precedence

Replace the `for lang, parser := range pw.parsers` scan with a resolution that consults languages in
a **stable, defined order** rather than map order, and that resolves an extension claimed by more
than one declared language through D1's rule.

*Why:* the fix must be at the selection site because that is where the nondeterminism is; leaving it
and relying on disjoint extensions would make the invariant "the next contributor must never
register an overlapping extension", which nothing enforces.

*Alternative considered:* keep the map and make registration reject overlapping extensions
outright. Rejected — that forbids the C/C++ case this change exists to serve.

**A test must pin this by construction**, not by observation: a watch path declaring both `c` and
`cpp` must route a `.h` file to the same parser across many repetitions. A single run passes by
luck ~50% of the time, so the test asserts over repeated resolution.

### D3 — C and C++ are separate parsers and separate domains

Two registrations (`c`, `cpp`), two `{domain}` values, even though one grammar could nominally read
both.

*Why:* `{domain}` is part of the entity ID and is what `code-context` scoping and the fusion lens
filter on. Collapsing C into C++ would make every MAVLink symbol claim to be C++, which is both
wrong and unfixable later without an ID migration.

### D4 — A C symbol's identity is qualified by its file path

C has no module system: two `.c` files may each define a `static` function of the same name, and
they are different functions. The existing `{org}.semsource.{domain}.{system}.{type}.{instance}`
shape gives no namespace to distinguish them, so `instance` must incorporate the file path — the
same approach the doc passage IDs already take with a path slug.

*Why:* without it, two distinct symbols collide onto one entity ID and silently merge — the exact
failure mode `entity-identity-safety` exists to prevent. Header-declared symbols and their
definitions are a related case the spec must address rather than leave to chance.

*Open point for implementation (not a spec question):* whether a declaration in a `.h` and its
definition in a `.cpp` should produce one entity or two related ones. Both are defensible; the spec
requires only that the choice be deterministic and collision-free.

### D5 — The two language switches become total

`langToDomain` and `extensionsForLanguage` must stop defaulting to Go. Either they return an error
for an unknown language, or a test enumerates registered parsers and fails when one is unmapped.

*Why:* this is the difference between "the C parser is not wired yet" showing up as a build/test
failure versus as thousands of C symbols published under `{domain}` = `golang` — a corrupt graph
that no existing test would notice. Cheap to fix while adding a language; nearly invisible if left.

## Risks / Trade-offs

- **A repo declaring both `c` and `cpp` gets C++ semantics for its C headers** → Documented in D1
  and in the spec; C++ is a superset, so entities are still extracted. Operators wanting C identity
  for headers declare `c` alone.
- **No preprocessor means macro-heavy C parses into less than a compiler would see** → Stated as an
  explicit limit in the spec rather than discovered during the A/B. Meshtastic and MAVLink both use
  macros heavily; MAVLink in particular is largely generated macro-dense headers, so this bounds
  what the A/B can expect and must be measured, not assumed.
- **Two new domains widen the retrieval surface; a missed enumeration site silently drops them** →
  The wiring sites are enumerated in the proposal's Impact, and D5 turns the most dangerous omission
  into a failure.
- **tree-sitter C++ is a large grammar; parse time and memory per file are higher** → Measure on
  the real Meshtastic tree rather than asserting it is fine; the AST source already serializes
  per-file parsing behind `pw.parseMu`, so throughput is the thing to watch.

## Migration Plan

Additive. New languages are opt-in through `watch_paths[].languages`; a config that does not name
`c`/`cpp` behaves exactly as before, and no existing entity ID changes. Rollback is removing the
languages from config — no graph migration, because nothing that already exists is re-identified.

## Open Questions

- Whether C++ templates should yield one entity per template or per instantiation. Deferring is
  safe: instantiations are not visible without a compiler, so the change extracts the template
  declaration, and revisiting would add entities rather than re-identify existing ones.
