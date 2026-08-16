# Tasks — guard-minified-asset-ingestion

## 1. Detection

- [ ] 1.1 Minified name rule: case-insensitive `.min.js` / `-min.js` /
      `.min.css` / `-min.css` suffix check on the base name, unit-tested for
      hits, misses, and case variants (`Plotly-Latest-MIN.JS`)
- [ ] 1.2 Content probe: first-64KiB bytes-per-line heuristic (threshold 512,
      files under 4KiB exempt), unit-tested against a minified fixture string,
      a hand-written fixture, and a long-lines-but-real fixture staying under
      threshold
- [ ] 1.3 Wire the guard into `parseFileWithWatcher` after route resolution,
      before `ParseFile`: a detected file returns a synthetic file-entity-only
      ParseResult without invoking the parser; `files_parsed` still advances

## 2. Symbol cap

- [ ] 2.1 `max_symbols_per_file` on the ast-source config: schema tag, default
      5000, `0` disables, negative rejected by `Validate()` — config round-trip
      and validation unit tests
- [ ] 2.2 Cap enforcement on the ParseResult: symbol (non-file-level) entities
      beyond the cap are stripped, file-level entities kept, one WARN with
      path + withheld count; unit-tested at cap, over cap, and disabled
- [ ] 2.3 Mutation-verify every new guard (remove detection → test fails;
      remove cap strip → test fails; remove WARN → test fails) — commit BEFORE
      mutating, per the 2026-08-16 lesson

## 3. Aggregation and visibility

- [ ] 3.1 `filesWithheld` / `symbolsWithheld` atomics; one seed-end summary
      line at default level after "Initial index complete", emitted only when
      nonzero; silent on a clean corpus — both paths unit-tested
- [ ] 3.2 Watch-event guard hits log at Debug per file (no seed to aggregate
      to); verified by test

## 4. Acceptance on the measured corpus

- [ ] 4.1 OSH corpus (pin `235c0eab`) seed on a fresh stack: raw publishes drop
      ~77.8k → ~43k, zero `maximum bytes exceeded` failures, plateau gone from
      the poll trace (publish advances continuously once parse completes)
- [ ] 4.2 Seed-end summary reports the plotly family as minified_files (expect
      3–4; symbol counts are unknowable for skipped files by design); graph
      contains the file entities, zero `javascript.var` entities under the
      plotly paths
- [ ] 4.3 Scorecard `questions-osh.json` re-run stays 11/13 (G02/G03 remain
      the semstreams#823 reds; every green band stays green)
- [ ] 4.4 Dogfood guard: `go test ./...` and the CI integration set green;
      determinism gate unaffected (guard decisions are content-deterministic)
