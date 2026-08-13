# v5 loose-band baseline — the #170 gate run

**Date:** 2026-08-13 · **Corpus:** dogfood (`git archive HEAD` at `076ac39`, minus
`scripts/scorecard/`) · **Stack:** isolated `scorecard-loose` project
(28080/25222/29222/28081) · **Machine:** M3 Pro arm64, **not fully quiet** (a
sibling `semboids-demo-nats` stack was active on 24222/28222 — latency figures
are measured-under-load; recall and bytes are unaffected) · **Repeats:** 3, zero
UNSTABLE · Checkers green against this corpus before scoring (X01/X02 ok,
P01–P04 clean).

## Verdict: measured-no-gap — #170 closes, T1 feeding is not justified

Arm B (the product surface — the same `code_search`/`doc_context` engines the
UI's fusion route rides) went **33/33 with the loose band 7/7**, at a loose-band
median of 63 ms — indistinguishable from its precise-phrasing code band. Every
looseness dimension the band encodes (vocabulary-free, effect-described,
colloquial-where, behavior-described symbol hunt, maximally colloquial, typo,
effect-phrased twin-less) was absorbed without a classifier front-end.

Under the gate semantics (README, loose band): **both-pass = saturation**. The
dormant T1 embedding classifier (`domain_examples_path`) stays unfed.

## The one phrasing gap found — and where it lives

Arm C (raw cosine, top-20 over 6,495 product vectors): loose **6/7**. The miss
is **L05** ("what kinds of stuff can I point this thing at", maximally
colloquial) while its twin **D01 passed** — the exact twin-pass/loose-fail
signature the band was built to detect. So a phrasing gap exists, but **below
the product surface**: raw cosine misses the most vocabulary-free phrasing, and
arm B's query layer bridges it (B answered L05 correct at 218 B top-node).

This is the second measured separator between B and the cosine floor —
composition (v4) was the first; phrasing robustness on the colloquial extreme is
the second. Cosine-only consumers inherit the gap; MCP consumers do not.

## Arm A: a cost collapse, not a verdict collapse

Arm A went loose **7/7 on verdicts** — the band did not defeat grep's recall —
but at **652,375 B (~163k tok) for seven questions, 93 KB mean, 5.4× arm B's
bytes on the same band**, its most expensive band by far (own code-band mean:
31.9 KB). Loose phrasing multiplied the read set on 4 of 7 questions (L01
153.8 KB/5 files vs twin C04's 13.7 KB/1 file — 11.2×; L02 185.4 KB; L07
174.5 KB). Whole-file fact-presence lets generous expect terms ride along —
the same weakness that put A at 0/2 on discrimination (both IMPRECISE). The
honest claim: **under human phrasing, grep still finds the fact somewhere in a
five-file read; what it loses is the byte budget** — an agent pays ~163k tokens
for what B returns in ~30k, and a human UI cannot page through five whole files
at all.

## Per-band table

| band | A (grep) | B (MCP) | C (cosine) |
|---|---|---|---|
| code | 6/6 · 191 KB | 6/6 · 67 KB | 6/6 · 102 KB |
| composition | 0/4 · 71 KB | 4/4 · 8 KB | 0/4 · 71 KB |
| discrimination | 0/2 · 52 KB | 2/2 · 38 KB | 2/2 · 34 KB |
| doc-early | 3/3 · 101 KB | 3/3 · 48 KB | 3/3 · 43 KB |
| doc-late | 6/7 · 446 KB | 7/7 · 165 KB | 7/7 · 122 KB |
| impact | 2/2 · 42 KB | 2/2 · 5 KB | 2/2 · 32 KB |
| **loose** | **7/7 · 652 KB** | **7/7 · 121 KB** | **6/7 · 118 KB** |
| negative | 2/2 · 45 KB | 2/2 · 21 KB | 2/2 · 26 KB |
| **TOTAL** | **26/33 · 1,600 KB** | **33/33 · 473 KB** | **28/33 · 548 KB** |

Latency (same-machine, under load): B ALL 75/154 ms median/p95 — loose 63/92 ms;
A ALL 601/1030 ms; C ALL ~1.9 s (embed round-trip dominated).

## Bounds and caveats

- **One corpus.** The loose band exists only in the dogfood set (v5); OSH is its
  own comparability domain and has no loose band. Author one there only if a
  real consumer surfaces Java-shaped loose phrasings worth pinning.
- **Latency is under-load** (sibling stack active); arm-to-arm comparison within
  this run is same-machine-same-load and stands; cross-run latency comparisons
  should prefer the quiet-machine RC numbers.
- Arm A's loose 7/7 rides on `expect_any` terms that are generous against
  whole-file evidence (`exclude`, `store`, `Build`) — deliberate, since expects
  are twin-identical by design. The cost column carries the discrimination.
- The gate decision consumed recall + bytes only, which are load-independent.
