# Tasks: scorecard-token-cost-arms

## 1. Arm B cost accounting (run.sh)

- [x] 1.1 Record per-question context cost in `run.sh`: bytes of the first call's decoded tool
      result (charged whether it succeeds or returns `isError`; repeat calls never charged — D5),
      as additive `context_bytes` field in the per-question results JSON
- [x] 1.2 Capture `tools/list` response byte length and tool count once after `initialize`; store
      as session-level `schema_overhead` in the results JSON header, never folded into
      per-question figures (D4)
- [x] 1.3 Extend the end-of-run summary with per-band context-bytes figures (bytes primary,
      `bytes/4` token estimate labeled as an estimate — D2), leaving all existing output lines
      unchanged
- [x] 1.4 Verify a fresh run's results JSON is backward-compatible: existing fields byte-identical
      in shape, new fields additive; re-run an existing results file through the summary jq paths

## 2. Arm A — deterministic grep-and-read baseline

- [x] 2.1 Commit `scripts/scorecard/stopwords.txt` (fixed list; README comparability note — changing
      it invalidates cross-run cost comparison)
- [x] 2.2 Implement `scripts/scorecard/arm-a-grep.sh <corpus-dir> <label>`: term extraction →
      `grep -ril` file ranking (distinct-terms, total matches, lexicographic tie-break) →
      term-coverage reading with file cap 5 → whole-file charging, per-file bytes recorded (D1);
      consumes only `args.query`, never `expect_*`
- [x] 2.3 Grade the read set with the existing matcher semantics (top-ranked file = top node for
      discrimination bands); emit the shared results JSON shape with `arm: "A"`
- [x] 2.4 Determinism check: two consecutive runs over the same corpus produce byte-identical
      results JSON (timestamps excluded)

## 3. Arm C — semembed-only cosine ranking

- [x] 3.1 Probe `EMBEDDING_INDEX` payload parseability from a script; if impractical, switch to the
      documented fallback (enumerate passage entities via graph query, re-embed bodies via
      `/v1/embeddings`) and record which path was used in the results JSON (D3)
- [x] 3.2 Implement `scripts/scorecard/arm-c-cosine.sh <label>`: embed `args.query` with
      `query_prefix` (query side only), cosine-rank product vectors, fetch top-K bodies via graph
      query for grading only; K defaults to arm B's node cap, recorded per question; gate on
      `embedding.ready` like `run.sh`
- [ ] 3.3 Grade top-K bodies with the existing matcher semantics (rank-1 body = top node); charge
      body bytes + recorded query-embedding request/response bytes; emit shared results shape with
      `arm: "C"`
- [x] 3.4 Spot-verify isolation: for one doc-band and one code-band question, confirm the ranking
      path issues no graph query before ranking (body fetch only after — D3's isolation claim)

## 4. Comparison + documentation

- [x] 4.1 Implement `scripts/scorecard/compare.sh <results.json>...`: join any set of same-version
      result files into the per-band × per-arm table (fact recall + context bytes/tokens),
      refusing files with mismatched `questions.json` versions
- [x] 4.2 Update `scripts/scorecard/README.md`: arm procedures, blindness rule, whole-file-charging
      bias direction, doc-band C-vs-B interpretation caveat, cost comparability rules (same corpus,
      same stack for B/C, stopword list versioning), schema-overhead framing (fixed F + marginal)
- [ ] 4.3 Run all three arms against a live stack on the dogfood corpus (isolated
      `COMPOSE_PROJECT_NAME`, high ports); commit results under `scripts/scorecard/results/` with a
      `SUMMARY-token-cost-baseline.md` recording the first per-band × per-arm table
- [ ] 4.4 Post the measured baseline (schema overhead + per-band table) to #130, and the
      `tools/list` measurement to #126

## 5. Follow-ups (file, do not implement here)

- [x] 5.1 File the staleness-probe issue (provisioning-controlled variant; out of scope per
      proposal) and the error-amplification-probe issue, both referencing #130's methodology
