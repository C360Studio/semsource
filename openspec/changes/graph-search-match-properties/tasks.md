# Tasks: graph-search-match-properties

## 1. Rendering

- [x] 1.1 Add the value-predicate allowlist (declaration-ordered slice, not a map) and
      caps (8 props/match, 160 B/value) to `processor/mcp-gateway/graph_matches.go`;
      resolve exact predicate constants from `source/vocabulary/predicates.go` and
      `source/ast` (artifact version).
- [x] 1.2 Populate `graphMatch.Properties` on the entities path only; digests and
      ID-only paths unchanged. Deterministic property order (allowlist order).
- [x] 1.3 Unit tests: properties on entities path; absent on digest/ID paths; caps
      enforced (count + value length); non-allowlisted predicates never render;
      deterministic order across runs.
- [x] 1.4 (amendment 1 — SUPERSEDED BY AMENDMENT 3) `summarize_threshold: 0` was tried
      because the graphrag rung returned digests whose dependency labels are hash
      instances; the high review then falsified the full-entity default (no size guard,
      cache-order truncation, no scores) and the SHIPPED value is `1` (ranked compact).
      Upstream digest-label gap filed (semstreams#958 + asks-file entry) — the blocking
      dependency for #166's enablement.
- [x] 1.5 (amendment 2) `Object json.RawMessage` + scalar converter — numeric triple
      objects (code.metric.*, doc chunk-index) failed the whole unmarshal and collapsed
      matches into the disclosure fallback; regression test carries the substrate shape
      verbatim. Evidence preserved: results/166-pass-after-decodebug-osh-B.json.

## 2. Empirical gate (probe-becomes-test)

- [x] 2.1 Author `questions-osh.json` v2: keep v1 questions verbatim, add config band —
      1 name-level + 2 value-level questions against the OSH pin's real gradle deps;
      update its `config_band_absent` field/notes. Re-run `check-discrimination.py` and
      `check-composition.py` against the OSH corpus with the v2 set. (Anchors chosen
      substring-collision-safe; all checkers green.)
- [x] 2.2 Prove FAIL-before: run the v2 config band against a pre-change build (stack at
      current main) — value questions must miss. Recorded:
      results/166-fail-before-osh-B.json (G01/G02/G03 all miss — G01's miss exposed the
      digest-shape gap, see design amendment 1).
- [ ] 2.3 Prove PASS-after: same stack recipe with the change — value questions correct;
      full v2 set green; record arm B context-bytes delta on OSH and dogfood (v4 set,
      unchanged) vs the rc-beta6 baselines.

## 3. Docs + closure

- [x] 3.1 Update `scripts/scorecard/README.md`: config-band paragraph (value-level now
      authorable) + composition-band "inadmissible shapes" note + grading-scope caveat
      (the v2 config band grades graph_search's DETERMINISTIC match rendering; the
      escalating surfaces stay ungraded).
- [x] 3.2 Update the `graph_search` tool description (done; budget 7,199/8,192 B).
- [ ] 3.3 PR referencing #166 ("Closes #166"), with before/after context-bytes table and
      the FAIL-before/PASS-after evidence; wave-gate review (high /code-review; probes
      re-run by reviewer); merge on green checks JSON; `/opsx:archive` the change.
