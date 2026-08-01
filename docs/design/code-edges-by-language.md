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
| **Java** | ❌ | ✅ | ✅ | — | ✅ | ✅ | `hierarchyRefID` — package ↔ path is deterministic; `external:`/`builtin:` for the rest |
| **Python** | ✅ | ✅ | — | — | ✅ | ✅ | `typeNameToEntityID` — module ↔ file is deterministic |
| **TypeScript/JS** | ❌ | ✅ | ✅ | — | ❌ | ✅ | `hierarchyRefID` via import specifiers |
| **Svelte** | ❌ | — | — | — | ❌ | — | none — components, not a symbol graph |
| **C++** | ❌ | ✅ | — | — | ❌ | ❌ | `ResolveTypeRefs` — **post-parse index**, see below |
| **C** | ❌ | n/a | n/a | n/a | ❌ | ❌ | none — no inheritance to resolve |

**Read the Calls column carefully.** Only Go and Python emit call edges. Java,
TypeScript, C, and C++ do not — so for those languages `code_impact` answers from
*type hierarchy alone*. A Java method that is called by fifty others reports the
callers it inherits from, not the callers that invoke it. That is a much narrower
question than the tool's name suggests, and it is the single largest gap in this
table.

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
| **"What derives from this?"** (C++) | ✅ **new** — 89% of inheritance resolved |
| **"Who calls this?"** (C++, C, Java, TS) | ❌ **no call edges emitted** |
| "What derives from this?" (C) | n/a — C has no inheritance |

So the honest MVP position: **structural navigation and type hierarchy work for
C++; call-graph does not, for four of seven languages.**

## Known debt, in priority order

1. **Call edges for Java, TypeScript, C++, and C.** The largest gap in the table
   and the one most likely to be mistaken for a working feature, because
   `code_impact` returns a confident empty answer rather than saying it cannot
   answer. Go and Python already show the shape.
2. **C++ has no `References`** (field/parameter/return type usage), so
   "what uses this type?" is unanswerable there even though Java and Go answer it.
3. **`ResolveTypeRefs` only resolves within one watch path.** A base class in a
   different repo of a multi-repo corpus will not resolve. Acceptable today —
   inheritance rarely crosses repos — but it is a boundary, not a guarantee.
4. **Ambiguity is silent.** Dropped references are not counted or surfaced
   anywhere at runtime; the 11%/3% figures above came from an offline probe. A
   counter would make the honesty measurable in production rather than in a
   one-off measurement.
5. **Svelte contributes no symbol edges at all** and is listed here so that is a
   recorded decision rather than an oversight.

## Keeping this page true

It is derived from code, so it goes stale silently. When adding a language or an
edge kind, update the matrix in the same change — and prefer proving a row with a
live query (`code_impact` against a known symbol, with a language that *does*
have edges as a control) over reading the parser and assuming.

The control matters: during the C/C++ acceptance run, C++ and C both reported
0 dependents, and only running the same query against Java — which returned 9 —
established that the machinery worked and the gap was the parser's.
