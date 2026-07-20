# Tasks — repairing the retrieval scorecard

Ordered by dependency. Groups 1–3 each change recorded outcomes, so the version-3
baseline (group 5) is recorded **once, after all three land**, against the fixed corpus
used throughout the diagnosis (`git archive d554bcc`, `scripts/scorecard/` excluded).

## 1. Make the matcher evaluate its literals

- [x] 1.1 Add `--` to all five matcher loops in `run.sh` (`expect_all`, `expect_any`,
      `expect_top_all`, `expect_top_none`, `expect_none`)
- [x] 1.2 Add a regression test that a literal beginning with `-` is matched as content, not
      parsed as an option — assert on the matcher, not on a live stack
- [x] 1.3 Extend `check-discrimination.py` with the behavioural evaluability gate: feed every
      literal through the same matcher against a known-hit and a known-miss string and require
      the two to disagree (design D2 — behavioural, not a syntactic ban on leading `-`)
- [x] 1.4 Confirm the gate fails on X02's literal under the unfixed matcher and passes under the
      fixed one, so the gate is proven to catch the bug it exists for

## 2. Name the third discrimination verdict

- [x] 2.1 Add the `MISLEADING` verdict to `run.sh`: `expect_top_none` matched **and**
      `expect_top_all` unmatched (no new question fields — derive from the existing pair)
- [x] 2.2 Order the verdict checks so `MISLEADING` cannot be shadowed by the plain `miss` that
      `expect_top_all` currently produces first
- [x] 2.3 Report `MISLEADING` separately in the per-band summary, distinct from `miss` and from
      `FABRICATED`
- [x] 2.4 Update the README's verdict section from four verdicts to five, keeping the existing
      rationale for why `IMPRECISE` is not folded into `FABRICATED` and adding the same
      reasoning for `MISLEADING` vs `miss`

## 3. Stop body-less nodes leading the answer

- [x] 3.1 Generalise the guard in `processor/code-context/component.go` from
      `Kind=="document" && Body==""` to any node with no retrievable body (design D6); decide
      filter-vs-salience here and record which, with the reason
- [x] 3.2 Preserve the existing protection that the guard never empties a result set
- [x] 3.3 Add a Go test covering a body-less non-document entity (the
      `{org}.semsource.config.…dependency.*` case actually observed), not only the parent-document
      case already covered
- [x] 3.4 Run the doc bands as the regression check — they are saturated at 10/10 and are what
      would detect over-filtering

## 4. Measure the instability rather than hide it

- [x] 4.1 Add `SCORECARD_REPEATS` (default 3) and ask each question N times
- [x] 4.2 Add the `UNSTABLE` verdict for disagreement across repeats, retaining each distinct
      outcome and the question's position in the run
- [x] 4.3 Never resolve disagreement silently to the passing or the failing result (design D3 —
      this is the decision, not an implementation detail)
- [x] 4.4 Surface the unstable count in the summary alongside fabrication, not folded into the
      score
- [x] 4.5 Document in the README that `SCORECARD_REPEATS=1` cannot detect instability, so a
      one-repeat run may not be quoted as evidence of stability

## 5. Re-baseline on the repaired instrument

- [x] 5.1 Bump `questions.json` to version 3
- [x] 5.2 Add a README comparability note naming the two reasons v2 results do not carry across
      (matcher fix, `MISLEADING` verdict), so a future reader understands why numbers moved
- [x] 5.3 Stand up the fixed corpus and record the v3 baseline. Expected: X02 `correct`, X01
      `MISLEADING`, discrimination 1/2 — **record what happens; if it differs, the difference is
      the finding, not an error to correct**
- [x] 5.4 Write `results/SUMMARY-v3-baseline.md` and cross-link it from
      `SUMMARY-instrument-diagnosis.md`

## 6. File the substrate defect

- [x] 6.1 Reduce the order-dependent drop to a minimal reproduction script that does not depend
      on the scorecard (burst of diverse `doc_context` queries, then the target query once)
- [x] 6.2 Add the entry to `docs/upstream/semstreams-asks.md`, triaged framework-shaped, naming
      the three eliminated hypotheses (embedding degradation, mixed vector population, unstable
      tie-sorting) so the reader does not repeat them
- [x] 6.3 Include the second and more valuable half of the ask: fusion exposes no score, rank or
      reason, and `fusionnats.resolveSemantic` discards the `Similarity` it receives
- [ ] 6.4 Open the GitHub issue on semstreams — issue only, never a PR (Product Boundary)
- [x] 6.5 State the blast radius accurately: reproduced on `doc_context`; `code_search` was
      stable across 4 consecutive calls after the same burst, which is not proof of immunity

## 7. Gates

- [x] 7.1 `task lint` clean — revive warnings fail CI, pinned v1.15.0
- [x] 7.2 `go test ./...` and `go test -race ./...` for the group-3 product change
- [x] 7.3 `shellcheck` or equivalent review of the `run.sh` changes, since the defect this change
      exists to fix was a shell quoting bug that no test would have caught
- [x] 7.4 `openspec validate --all` before finalising
