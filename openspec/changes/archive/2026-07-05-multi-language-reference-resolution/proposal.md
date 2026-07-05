# Proposal: Multi-language reference-ID parity (Java / TS / Go)

## Why

Task #44 fixed cross-file `extends` / `references` resolution for **Python**.
The identical failure mode is still live in **Java, TypeScript/JavaScript, and Go**:
every `extends` / `implements` / `references` / `embeds` edge is built against a
target ID that does **not** byte-match the definition, so it dangles —
`code_impact` and `code_context` show no subclasses, implementers, embedders, or
cross-type referrers for these three languages.

Confirmed empirically (Java, two classes in one file):

```
Animal definition:  acme.semsource.java.test.class.Zoo-java-Animal
Dog.Extends[0]:     acme.semsource.java.test.type.Zoo-java-extends Animal   ← dangles
```

Three distinct bugs stack up, and existing tests miss all of them because they
assert `strings.Contains(edge, "Animal")` (a substring) rather than
`edge == definition.ID`:

1. **Wrong entity-type segment** — the reference builder hard-codes `type`, but
   the definition uses `class` / `interface` / `struct` / `enum`. `.type.` ≠
   `.class.` → dangles. (Same `.type.`-vs-`.class.` bug #43 fixed for Python.)
2. **Raw `project`** — the reference builder uses `p.project` verbatim, but the
   definition runs it through `entityid.SystemSlug(project)`. Any project needing
   a slug (module-cache path, `@version`) → dangles. (The Go "raw-project bug".)
3. **Cross-file targets built against the referrer** — a bare `Animal` /
   imported `Base` builds its ID against the *referrer's* file path, not the file
   where the type is **defined** → dangles across files. (What #44 fixed for
   Python.) Java additionally mis-extracts the name as `"extends Animal"`
   (keyword + space), an invalid segment.

## What Changes

Bring Java, TypeScript/JavaScript, and Go to the parity #44 established for
Python — every resolved type-dependency edge targets the definition's own ID.

- **Route all reference-ID construction through the definition's own path.**
  Replace the ad-hoc `entityid.Build(..., p.project, "type", BuildInstanceID(...))`
  in each language's `typeNameToEntityID` with the same call the definition makes
  — `ast.NewCodeEntity(org, lang, project, <kind>, name, <definingRelPath>).ID` —
  which fixes bugs 1 and 2 for free (correct `SystemSlug`, caller-supplied kind).
- **Infer the target's entity-kind from syntactic position** where the language
  makes it unambiguous: Java/TS class `extends` → class, `implements` → interface,
  interface `extends` → interface. This is the clean, correct kind for the
  hierarchy edges that carry the most `code_impact` value.
- **Add per-language cross-file resolution**, mirroring #44's *pattern* (imports →
  local-name map → module/package → file → definition ID), each with its own
  resolver because import/package semantics differ:
  - **Java** — `import a.b.C;` binds simple name `C` → FQN → `<root>/a/b/C.java`;
    same-package (unimported) names resolve to a sibling file in the referrer's
    package directory.
  - **TS/JS** — `import { Base } from './base'` binds `Base` → module specifier
    → `<dir>/base.ts` (`.tsx`/`.js`/`.jsx`/`.mts`/… and `/index.*`).
  - **Go** — a bare local type resolves within the **same package = same
    directory**; a memoized per-directory sibling scan finds the defining file
    and the real kind (struct/interface/type) so embeds/references connect.
- **Fix the Java superclass name extraction** so the bound name is `Animal`, not
  `extends Animal`.
- No change to entity-ID **format**, payloads, or predicates. Strictly additive
  correctness: edges that dangled now resolve; nothing that resolved before
  changes.

## Capabilities

- **Modified Capabilities**: `code-reference-resolution` — generalize the
  existing capability (seeded by #44, Python-only) to Java, TypeScript/JavaScript,
  and Go: kind-from-syntactic-position, and per-language cross-file resolution.

## Impact

- **Code**: `source/ast/java/parser.go`, `source/ast/ts/parser.go`,
  `source/ast/golang/parser.go` (each `typeNameToEntityID` + a per-language
  import/package→file resolver); superclass extraction fix in Java. Unit tests
  asserting `ref-id == def-id` (not substring) + live validation per language.
- **APIs / payloads**: none. 6-part IDs, predicates, envelopes unchanged.
- **Downstream consumers** (SemSpec, SemDragon, agents over MCP): strictly better —
  cross-type / cross-module reverse-dependency becomes populated for Java, TS, Go.
  No breaking change.
- **Dependencies**: none new.

## Non-goals

- **Field / parameter / return / alias references of unknown kind** — a Java field
  type, a Go struct field, a TS parameter type can be a class *or* interface *or*
  enum; its kind is not knowable from position. Same-file such references resolve
  via a same-file name→kind table; cross-file ones of unknown kind stay inert
  (never a wrong edge) unless a resolver already discovers the kind (Go's sibling
  scan does; Java/TS import→file for these is deferred). Documented, not silently
  wrong.
- **Call-graph edges** — Python call graph is task #45; multi-language call graphs
  are a later task. This change is type-dependency edges only.
- **Star / wildcard imports** (`import a.b.*;`, `export *`), **re-exports** through
  a barrel/`__init__`/package, **dynamic** imports, and **namespace packages** —
  out of scope; such references stay inert (documented), never wrong.
- **Whole-project two-pass indexing** — rejected (as in #44): resolution stays
  per-file / per-directory and filesystem-driven, preserving incremental watch.
- **TS path aliases / `tsconfig` `paths`, `baseUrl`, `node_modules` resolution** —
  only relative (`./`, `../`) and in-tree specifiers resolve; bare-module
  specifiers stay `external:`/inert.
