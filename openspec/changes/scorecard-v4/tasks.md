# Tasks: scorecard-v4

## 1. Composition band and checker (D1, D2)

- [x] 1.1 Implement `scripts/scorecard/check-composition.py`: windowed co-occurrence scan proving
      no single passage/body carries a composition question's full `expect_all` set; same
      behavioral-gate style as `check-discrimination.py` (including a `--simulate` proof that the
      gate fires)
- [x] 1.2 Author composition questions against the dogfood corpus (shapes: impact closure,
      relation join, version composition, cross-source join; `why` names the traversal); admit
      only checker-passing questions; bump `questions.json` to version 4 with all v3 questions
      retained verbatim
- [x] 1.3 Extend `test-matcher.sh` coverage to the new band's literals and the composition checker

## 2. Latency dimension (D3)

- [x] 2.1 Record per-call wall-clock in all three arm scripts (`latency_ms` first-call +
      `latency_samples` across repeats); results header records host arch
- [x] 2.2 `compare.sh` renders median/p95 latency per band per arm; README comparability rules
      extended (same-machine rule, never cross-corpus) — README half tracked under 5.3

## 3. OSH second corpus (D4)

- [x] 3.1 `scripts/scorecard/corpus-osh.sh`: pinned-SHA OSH core corpus build (git archive,
      exclusions applied, pin recorded)
- [x] 3.2 Author `questions-osh.json` v1 (Java/Maven shapes: POM facts, symbol retrieval, impact,
      composition if admissible); run both checkers against the OSH corpus — POM/config facts
      documented unreachable (Gradle-only corpus; gradle dep entities unlabeled in graph_search)
- [ ] 3.3 Stand up an OSH stack (isolated project name, 25222+ ports — semdev owns 24222) and
      record the scale report: entities, seed wall-clock to full readiness

## 4. Arm-D readiness (D5)

- [x] 4.1 Add dormant `llm_calls`/`llm_cost_note` fields + `arm_uses_llm` header to the results
      schema and `compare.sh` (rendered only when present)

## 5. Baselines and documentation

- [x] 5.1 Run all three arms on the dogfood corpus (questions v4); commit results + SUMMARY with
      the composition band's B-vs-C recall verdict stated plainly — SEPARATION: B 4/4 vs C 0/4
      (A 0/4), B at ~9x less context on the band; v3 subset still saturated on both
- [ ] 5.2 Run all three arms on the OSH corpus (questions-osh v1); commit results + scale report
- [ ] 5.3 README: composition band rationale, checker usage, latency rules, OSH corpus recipe and
      cross-corpus rule; post the v4 dogfood verdict to #130 and the OSH scale numbers to the OSH
      scale-test tracking context
