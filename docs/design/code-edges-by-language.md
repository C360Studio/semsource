# Code edges by language — what `code_impact` can actually answer

`code_impact` answers *"what breaks if I change this?"* That is the highest-value
question an agent asks of a codebase it did not write, and it is a large part of
why SemSource exists. It is also the capability that varies most by language, and
the variation is invisible from the outside: a language with no edges returns a
clean, confident, **empty** answer.

This page tracks that variation so the gap is a known quantity rather than a
discovery. Derived from the parsers in `source/ast/*`, verified against a live
stack — not from intent.

## The matrix

Relationship fields each parser populates. A field only produces a graph edge when
its value is a resolvable **entity ID**; a raw name dangles and is dropped.

| Language | Calls | Extends | Implements | Embeds | References | Returns / Params / Receiver | Resolution strategy |
| --- | :-: | :-: | :-: | :-: | :-: | :-: | --- |
| **Go** | ✅ | — | — | ✅ | ✅ | ✅ | `typeNameToEntityID` — package ↔ directory is deterministic |
| **Java** | ✅ | ✅ | ✅ | — | ✅ | ✅ | declared receiver types + `resolveJavaType`; ancestor walk for inherited methods |
| **Python** | ✅ | ✅ | — | — | ✅ | ✅ | `typeNameToEntityID` — module ↔ file is deterministic |
| **TypeScript/JS** | ❌ | ✅ | ✅ | — | ❌ | ✅ | `hierarchyRefID` via import specifiers |
| **Svelte** | ❌ | — | — | — | ❌ | — | none — components, not a symbol graph |
| **C++** | ❌ | ✅ | — | — | ❌ | ❌ | `ResolveTypeRefs` — **post-parse index**, see below |
| **C** | ❌ | n/a | n/a | n/a | ❌ | ❌ | none — no inheritance to resolve |

**Read the Calls column carefully.** Go, Python, and now Java emit call edges.
**TypeScript, C, and C++ still do not** — for those three, `code_impact` answers
from *type hierarchy alone*: a method called by fifty others reports the callers
it inherits from, not the callers that invoke it. That is a much narrower question
than the tool's name suggests, and it remains the largest gap in this table.

## How Java resolves calls, and how well

Java needs no type inference: it *declares* types. A per-method table built from
class fields, parameters, and locals types the receiver of `x.foo()`; the type
resolves to a file through the same import/same-package machinery type references
use; and an edge is emitted only once that file is confirmed to declare the
method. When it does not, the `extends`/`implements` chain is walked to the
nearest ancestor that does.

Measured on OSH Core (`opensensorhub/osh-core`, 1,932 files, 313k LOC), where
71,262 call sites break down as:

| receiver shape | share |
| --- | ---: |
| typed variable (`x.foo()`) | 46.0% |
| bare / `this.` | 19.9% |
| chained (`a().b()`) | 17.2% |
| identifier that is not a variable (static call) | 10.4% |
| field access, `super.`, literals, casts | 6.5% |

Result: **23,566 resolved call edges, 0 dangling**, on 54.3% of all callables.
8,363 further callees are `external:` markers (third-party APIs), of which 157
(1.9%) still name an in-repo package. Parse wall-clock for the tree went from
1.46s to ~8.4s — the cost is parsing each referenced file to confirm the callee,
which is small next to the embedding pass that dominates ingest.

Verified on a live stack (`lib-ogc/swe-common-core`, 6,317 entities), where
`code_impact` previously returned a confident empty answer:

- `addToProcessorTree` → **11 callers across 10 files**, plus its own two callees.
- `errorLocationString`, whose four calls all target `javax.xml.stream` types →
  the `callee` relation is **absent entirely**. The fail-inert boundary holds
  end-to-end: `external:` markers dangle and are dropped rather than fabricating
  an in-tree edge.

The parser counts 61 *declarations* calling `addToProcessorTree` where the graph
shows 11 *entities*, and the gap is not an error — it is the overload collision in
debt item 2 made visible. `BinaryDataParser` alone declares six `visit` overloads
that share one entity ID, so ten files' `visit` plus one `visitRange` is exactly
11.

Everything unconfirmable emits **nothing**: chained calls, `super.`, `var`
receivers, static imports, a receiver name declared with two different types in
one method, a type name matching several files, and — importantly — enum and
record members, which have no member entities at all, so naming one would dangle.

### Multi-module repos

Gradle and Maven give each module its own source root, and resolution originally
probed only the referrer's own. On OSH Core's 15 source roots that left 4,682 call
targets marked `external:` despite naming an in-repo package. Sibling roots of the
same layout are now probed too; a name found under exactly one resolves, and a
name found under several is **ambiguous, not external**, so it emits nothing.

This repaired pre-existing hierarchy edges as well — `extends`/`implements`
resolution on the same corpus went from **53.9% to 70.0%** (+339 edges), a defect
that predated call edges and was only visible once something measured it.

## Why C++ resolves differently from everything else

Go, Java, and Python share a property that makes resolution easy: **a name implies
a path.** `org.sensorhub.api.ISensorHub` is `org/sensorhub/api/ISensorHub.java`.
A parser can therefore build a definition's entity ID while looking only at the
file in front of it.

C++ has no such convention. Which header defines `class Foo` depends on what was
`#include`d, and that depends on include paths and the preprocessor — neither of
which SemSource runs (see the `c-family-symbol-extraction` spec). Measured on
Meshtastic firmware, **85% of inheritance references name a base defined in
another file**, so per-file resolution would capture almost nothing:

| base class defined in | share of 393 references |
| --- | --- |
| the same file | 4% |
| **another file** | **85%** |
| not in the corpus | 11% |

The way out is that C++ class names are nearly unique in practice: only **3%** of
the 565 class names in that corpus are defined in more than one file. So an index
built over the whole watch path resolves the large majority without guessing.

`cpp.ResolveTypeRefs` runs at the point in `processor/ast-source` where the entire
watch path has been parsed. It is order-independent — the index is complete before
any rewriting — which matters because entity IDs must be intrinsic and
reproducible. **A name matching more than one definition is dropped, not guessed**:
a wrong inheritance edge would make `code_impact` report a dependent no compiler
agrees with, which is worse than reporting none.

Measured result on Meshtastic: **350 of 393 references (89%) resolve to real
entity IDs**; the remaining 11% are bases outside the corpus (Arduino/SDK types)
and are correctly dropped.

## Against the MVP goal

The A/B asks an agent to *"create a Meshtastic driver"* and *"create a MAVLink
driver"*. Judged against that:

| Capability | Status |
| --- | --- |
| Find a C/C++ symbol by name or description | ✅ `code_search`, `code_context` |
| Read its signature, doc comment, location | ✅ |
| Navigate file → symbols | ✅ containment |
| **"What derives from this?"** (C++) | ✅ 89% of inheritance resolved |
| **"Who calls this?"** (Java) | ✅ **new** — 22,976 edges on OSH Core, 0 dangling |
| **"Who calls this?"** (C++, C, TS) | ❌ **no call edges emitted** |
| "What derives from this?" (C) | n/a — C has no inheritance |

So the honest MVP position: **Java now answers "who calls this?"; C, C++, and
TypeScript still do not.** Call-graph coverage is three of seven languages.

## Known debt, in priority order

1. **Call edges for TypeScript, C++, and C.** Still the largest gap in the table
   and the one most likely to be mistaken for a working feature, because
   `code_impact` returns a confident empty answer rather than saying it cannot
   answer. Go, Python, and Java now show the shape; TypeScript is closest, since
   it has import specifiers and declared types.
2. **Java method entity IDs collide on overloads.** 1,164 of 16,356 method
   declarations in OSH Core (7.1%) share a `(class, name)` pair, so overloaded
   methods already share one entity ID — the same defect PR #121 fixed for C++
   with a parameter-type discriminator. It is *not* fixed here, deliberately:
   adding a discriminator would require real overload resolution at every call
   site to choose a target, which is the type inference this design excludes. A
   call currently resolves to "some overload", the honest answer available
   without inference. Fixing identity later must treat call edges as a consumer.
3. **Chained calls are 17.2% of Java call sites** and need return-type
   propagation — the single biggest remaining recall item for Java.
4. **C++ has no `References`** (field/parameter/return type usage), so
   "what uses this type?" is unanswerable there even though Java and Go answer it.
5. **`ResolveTypeRefs` only resolves within one watch path.** A base class in a
   different repo of a multi-repo corpus will not resolve. Acceptable today —
   inheritance rarely crosses repos — but it is a boundary, not a guarantee.
6. **Ambiguity is silent.** Dropped references are not counted or surfaced
   anywhere at runtime; the figures above came from offline probes. A counter
   would make the honesty measurable in production rather than in a one-off
   measurement. This now spans Java too.
7. **Svelte contributes no symbol edges at all** and is listed here so that is a
   recorded decision rather than an oversight.

## Keeping this page true

It is derived from code, so it goes stale silently. When adding a language or an
edge kind, update the matrix in the same change — and prefer proving a row with a
live query (`code_impact` against a known symbol, with a language that *does*
have edges as a control) over reading the parser and assuming.

The control matters: during the C/C++ acceptance run, C++ and C both reported
0 dependents, and only running the same query against Java — which returned 9 —
established that the machinery worked and the gap was the parser's.
