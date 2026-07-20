# Tasks — making the default outrank the workaround

The A/B baseline side is **already recorded**: `results/v3-baseline.json`, questions.json v3,
corpus `git archive d554bcc` with `scripts/scorecard/` excluded. Hold that corpus fixed and vary
only the binary.

## 1. Detect a homogeneous key/value block

- [x] 1.1 Add detection in `handler/doc/splitter.go`: a fenced block whose non-blank lines are
      predominantly `^[A-Za-z_][A-Za-z0-9_]*=` (design D2)
- [x] 1.2 Group consecutive qualifying lines by the key's leading token up to the first underscore
      (design D3) — purely lexical, no dictionary and no model
- [x] 1.3 Gate on at least three distinct groups, so small or uniform blocks are left whole
      (design D6)
- [x] 1.4 Unit-test detection against the measured case (README `§ Configuration`: five groups,
      `NATS_*` is three keys) and against a two-group block that must NOT split

## 2. Split on homogeneity, independently of size

- [x] 2.1 Introduce the split as a trigger that runs regardless of section size — § Configuration
      is 1363 B, under the 2000 ceiling, so a size-gated path can never reach it (design D1)
- [x] 2.2 Narrow the fenced-block-is-atomic rule to exclude homogeneous key/value lists only;
      ordinary code fences keep today's behaviour unchanged
- [x] 2.3 Do NOT repeat the section heading in each group's body (design D4): it breaks tiling and
      measured 0.8127 vs 0.8133 without, so it buys nothing
- [x] 2.4 Verify the tiling invariant still holds — the existing tests
      (`splitter_test.go:301`, `passage_test.go:514`) must pass **unmodified**. If an
      implementation needs those tests edited, the implementation is wrong

## 3. Survive the floor merge

- [x] 3.1 ~~Mark homogeneity groups as not merge-eligible~~ **NOT NEEDED** — `mergeSmallSections`
      operates on `[]section` and runs *before* `subdivide`, so it never sees these spans. Design D5
      corrected in place rather than implementing a no-op exemption
- [x] 3.2 ~~Confine the marker~~ **NOT NEEDED** — no marker exists; the floor is untouched
- [x] 3.3 Test the failure mode directly: split, run the merge pass, assert the groups survive.
      This is the decision most likely to be got wrong — without it the whole change is a silent
      no-op

## 4. Offline confirmation before spending a stack

- [x] 4.1 Extend the bounds/sweep test to assert the README `§ Configuration` block yields a
      passage containing `NATS_MONITOR_HOST_PORT=8222` and NOT `SEMSOURCE_CONFIG=`
- [x] 4.2 Confirm the emitted group matches the measured 236-byte shape closely enough that the
      0.7783 cosine result is expected to carry — if the emitted text differs materially, re-measure
      offline before running the stack

## 5. The A/B

- [ ] 5.1 Rebuild the stack on the candidate binary with `docker compose down -v` (rebuild, not
      reindex — design D7), same corpus, wait for `phase` + `index.ready` + `embedding.ready`
- [ ] 5.2 Run `scripts/scorecard/run.sh v3-candidate` at the default `SCORECARD_REPEATS=3`
- [ ] 5.3 **The signal:** X01 moves `MISLEADING` → `correct`
- [ ] 5.4 **The regression detector:** doc bands stay 10/10. A drop there kills the change
      regardless of X01
- [ ] 5.5 Record entity count and time-to-ready against the v3 baseline — the existing
      `retrieval-ranking` requirement obliges corpus growth to be measured, not assumed
- [ ] 5.6 Do NOT read X02 either way: it may move as a side effect of re-chunking and may stay
      `UNSTABLE` while semstreams#597 is open

## 6. The spec correction (independent of the above)

- [x] 6.1 Apply the `runtime-configuration` delta: org is rejected, `project` is normalized by
      design
- [x] 6.2 Confirm against code that the narrowed requirement is now true — `config.ValidateNamespace`
      covers namespace only, and `entityid.SystemSlug` normalizes `project`
- [x] 6.3 Add a test pinning that a `project` outside the alphabet loads successfully and is
      slugified at ID construction, so the spec and the code cannot drift apart again

## 7. Record and gate

- [ ] 7.1 Write `results/SUMMARY-v3-candidate.md`: the A/B table, the entity-count delta, and what
      the result does and does not establish
- [ ] 7.2 If X01 does not move, record why rather than tuning until it does — the offline cosine
      predicts it will, and a disagreement between prediction and stack is itself the finding
- [ ] 7.3 `task lint` clean (revive warnings fail CI, pinned v1.15.0)
- [ ] 7.4 `go test ./...` and `go test -race ./...`
- [ ] 7.5 `openspec validate --all` before finalising
