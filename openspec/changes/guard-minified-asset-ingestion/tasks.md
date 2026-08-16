# Tasks — guard-minified-asset-ingestion

## 1. Detection

- [x] 1.1 Minified name rule: case-insensitive `.min.js` / `-min.js` /
      `.min.css` / `-min.css` suffix check on the base name, unit-tested for
      hits, misses, and case variants (`Plotly-Latest-MIN.JS`)
- [x] 1.2 Content probe: first-64KiB bytes-per-line heuristic (threshold 512,
      files under 4KiB exempt), unit-tested against a minified fixture string,
      a hand-written fixture, and a long-lines-but-real fixture staying under
      threshold
- [x] 1.3 Wire the guard into `parseFileWithWatcher` after route resolution,
      before `ParseFile`: a detected file returns a synthetic file-entity-only
      ParseResult without invoking the parser; `files_parsed` still advances

## 2. Symbol cap

- [x] 2.1 `max_symbols_per_file` on the ast-source config: schema tag, default
      5000, `0` disables, negative rejected by `Validate()` — config round-trip
      and validation unit tests
- [x] 2.2 Cap enforcement on the ParseResult: symbol (non-file-level) entities
      beyond the cap are stripped, file-level entities kept, one WARN with
      path + withheld count; unit-tested at cap, over cap, and disabled
- [x] 2.3 Mutation-verify every new guard (remove detection → test fails;
      remove cap strip → test fails; remove WARN → test fails) — commit BEFORE
      mutating, per the 2026-08-16 lesson

## 3. Aggregation and visibility

- [x] 3.1 `filesWithheld` / `symbolsWithheld` atomics; one seed-end summary
      line at default level after "Initial index complete", emitted only when
      nonzero; silent on a clean corpus — both paths unit-tested
- [x] 3.2 Watch-event guard hits log at Debug per file (no seed to aggregate
      to); verified by test

## 4. Acceptance on the measured corpus

- [x] 4.1 DONE 2026-08-16: raw publishes 77,802 → **32,570** (better than the
      ~43k prediction — the flood was also republishing colliding single-char
      IDs), zero publish failures (the one `maximum bytes exceeded` grep hit is
      the startup discard-posture ADVISORY quoting the phrase in its hint),
      plateau GONE — seed ~8min → **~2min**, publish advancing continuously
      (11,595 → 32,570 across consecutive polls)
- [x] 4.2 DONE: summary `minified_files:10, capped_files:0` — exactly the 10
      conventionally-named minified `.js` files in the corpus (the family was
      bigger than the 3–4 guess; `.min.css` files never route to a parser so
      the guard correctly never sees them); searchGraph probes surface zero
      plotly `javascript.var`/`function` entities
- [x] 4.3 DONE: `beta161-osh-v2-guarded` 11/13, verdicts AND byte counts
      identical to the unguarded run; G02/G03 remain the semstreams#823 reds
- [x] 4.4 DONE: full unit suite, `-race` on ast-source, lint, and the CI
      integration set (governance + mcp-gateway + code-context) all green on
      the branch; guard decisions are pure functions of name+content
