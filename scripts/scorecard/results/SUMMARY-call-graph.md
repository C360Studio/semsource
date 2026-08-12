# call-graph-completeness verification — both corpora, final binary

**Date:** 2026-08-12 · **Branch:** `feat/call-graph-completeness` (final commit `6110d53`)
**Question sets:** v4 (dogfood) / questions-osh v1 — unchanged, so scores compare directly to the
scorecard-v4 baselines.

## Scores on the final binary

| corpus | before the change | after | what moved |
| --- | --- | --- | --- |
| dogfood (6,240 entities) | 26/26 | **26/26** | nothing — non-regression across four languages of resolver change |
| OSH (32,157 entities) | 9/10 (P01 miss) | **10/10** | P01: `code_impact` on `cloneAsTemplatePermission` now NAMES all three cross-module callers |

Zero UNSTABLE anywhere (repeats=3 throughout). The dogfood seed logs contain **zero
`not found` graph-query errors** — the dangling-edge class (#143's `...lifecycle-go-stat` spam
on every seed) is gone at the source.

**P01 now passes on merit, not coincidence.** The mid-change run already showed 10/10, but the
graph reviewer proved that pass depended on the OSH subclass files happening to reference
`ModulePermissions` themselves; the declaring-file binding (`typedField.declRel`) removes that
dependence, and the decoy probe — a same-named type in the subclass's package — is pinned as
`TestInheritedFieldTypeBindsInDeclaringFile`.

## Seed wall-clock (OSH, parse-cost check for the receiver tables)

1,311 s (pre-change baseline) → **1,244 s** (mid-change, clean run) → final run's timing is
EXCLUDED: a local Docker restart interrupted the seed mid-flight, so its wall-clock measures the
restart, not the parser. The clean mid-change number already answers design D1's question: the
inherited-field tables and new call passes cost nothing measurable at 32k entities.

## The review trail (wave gate, both reviews)

1. **graph-event-reviewer** (charter: entity identity, edge semantics, watch correctness):
   REQUEST-CHANGES with two blockers, both wrong-REAL-edge class — TS/Svelte scope shadowing
   (a callback param named like a module function minted a false edge) and Java inherited-field
   types resolving from the wrong file. Fixed; re-review APPROVE-WITH-NITS with all probes
   re-run; both nits (nested-callback params, package-proxy comment) closed beyond the verdict's
   requirement.
2. **High-effort code review** (29 agents): 10 confirmed findings — TS default-export and
   export-awareness holes, Python nested-class `self` leak, whitespace-fragile Java modifier
   parsing, C mixed-corpus header dangling, watch-root escape, alias-keyed external markers,
   and a quadratic seed-I/O pattern. 9 fixed (each with the probe shape as a regression test),
   1 already-documented deliberate limitation (C configured excludes — dangling-loud direction).

Every fix round ended with the full 8-package AST suite, repo build, and pinned-revive lint
green. The doctrine held throughout: ambiguity drops the edge; nothing guesses.
