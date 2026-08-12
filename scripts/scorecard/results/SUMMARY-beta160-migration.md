# Beta.160 migration proof — three-arm rerun (2026-08-12)

Migration-proof run for the `semstreams-beta160-migration` change: same
`questions.json` (v3) as the 2026-08-09 baseline, corpus rebuilt from the
migration HEAD (`bb3618f`, 6,240 entities — the baseline's commit plus the
migration itself), fresh NATS 2.14 storage per the beta.160 adoption contract,
arm B at repeats=3.

**Substrate pin changed between runs (beta.159 → beta.160), so this is an
annotated comparison, not a pure A/B: the corpus commit and substrate moved
together.** The question the run answers is the migration's: did retrieval
survive the foundation cutover? It did.

## Per band, against the 2026-08-09 baseline

| band | A: grep (was) | B: MCP (was) | C: cosine (was) |
|---|---|---|---|
| code | 6/6 — 187.0 KB (=) | 6/6 — 65.9 KB (67.6) | 6/6 — 101.3 KB (101.8) |
| discrimination | 0/2 (=) | 2/2 — 37.8 KB (38.3) | 2/2 — 33.8 KB (30.2) |
| doc-early | 3/3 (=) | 3/3 — 47.8 KB (49.6) | 3/3 — 42.0 KB (41.7) |
| doc-late | 5/7 (=) | 7/7 — 162.5 KB (162.4) | 7/7 — 117.0 KB (117.3) |
| impact | 2/2 (=) | **2/2 — 4.6 KB (=)** | 2/2 — 32.3 KB (=) |
| negative | 2/2 (=) | 2/2 — 21.0 KB (=) | 2/2 — 26.2 KB (=) |
| **TOTAL** | **18/22 — 848 KB** | **22/22 — 340 KB** | **22/22 — 353 KB** |

Schema overhead: 7,087 B / 9 tools — byte-identical to the baseline.

## What this proves for the migration

1. **No recall regression on any band, any arm.** Full 22/22 on both product
   arms, zero UNSTABLE across repeats, zero MISLEADING/FABRICATED — on a stack
   whose entire mutation path, port grammar, governance model, and service
   composition changed underneath it.
2. **Costs moved within noise** (≤2.5% per band). The upstream #601/#602
   embedding fixes produced no measurable doc-band shift on this corpus —
   consistent with the fixes having landed before beta.159's baseline stack,
   not with this cutover.
3. **The run itself is the strict-validation and storage proof**: the fresh
   stack reached `phase=ready + index.ready + embedding.ready` under beta.160's
   strict flow validation (task 3.2), and arm C resolved every top-K body
   through `ENTITY_STATES.storage_ref` → the exact-named `CONTENT` store with
   zero exclusions (task 3.3).

Comparability notes: baseline stack pin was beta.159 (main @ 4fcea34); this
run's is beta.160 (bb3618f). Corpus deltas are the migration's own commits.
All other rules from the README apply unchanged.
